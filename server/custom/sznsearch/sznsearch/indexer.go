package sznsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

const (
	indexerTickInterval = 5 * time.Second
)

// startIndexer starts the background indexer worker
func (s *SznSearchImpl) startIndexer() {
	s.Platform.Log().Info("SznSearch: Background indexer worker started",
		mlog.Duration("tick_interval", indexerTickInterval),
		mlog.Int("batch_size", s.batchSize),
	)

	ticker := time.NewTicker(indexerTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			s.Platform.Log().Info("SznSearch: Background indexer worker stopped")
			return
		case <-ticker.C:
			s.processMessageQueue()
		}
	}
}

// processMessageQueue processes messages from the queue and indexes them
func (s *SznSearchImpl) processMessageQueue() {
	if !s.IsIndexingEnabled() {
		return
	}

	// Check backend health before processing - retry with backoff if unhealthy
	if !s.isBackendHealthy() {
		s.Platform.Log().Debug("SznSearch: Backend unhealthy, skipping queue processing")
		return
	}

	s.mutex.WLock(common.MutexMessageQueue)
	if len(s.messageQueue) == 0 {
		s.mutex.WUnlock(common.MutexMessageQueue)
		return
	}

	s.Platform.Log().Debug("SznSearch: Processing message queue",
		mlog.Int("queue_size", len(s.messageQueue)),
	)

	// Collect batch of messages (up to batchSize)
	batch := make([]common.IndexedMessage, 0, s.batchSize)
	postIDs := make([]string, 0, s.batchSize)

	for postID, msg := range s.messageQueue {
		batch = append(batch, *msg)
		postIDs = append(postIDs, postID)
		if len(batch) >= s.batchSize {
			break
		}
	}

	// Remove collected messages from queue
	for _, postID := range postIDs {
		delete(s.messageQueue, postID)
	}
	// Release lock BEFORE network I/O to minimize lock contention
	s.mutex.WUnlock(common.MutexMessageQueue)

	if len(batch) == 0 {
		return
	}

	// Index batch (ES client is thread-safe, no locking needed)
	s.Platform.Log().Debug("SznSearch: Indexing batch", mlog.Int("batch_size", len(batch)))
	if err := s.indexMessageBatch(batch); err != nil {
		s.Platform.Log().Error("SznSearch: Failed to index message batch",
			mlog.Err(err),
			mlog.Int("batch_size", len(batch)),
		)

		// Return failed messages back to queue
		s.mutex.WLock(common.MutexMessageQueue)
		for _, msg := range batch {
			msgCopy := msg // Copy to avoid pointer issues
			s.messageQueue[msg.ID] = &msgCopy
		}
		s.mutex.WUnlock(common.MutexMessageQueue)

		s.Platform.Log().Info("SznSearch: Returned failed messages to queue",
			mlog.Int("num_messages", len(batch)),
		)
	} else {
		s.Platform.Log().Debug("SznSearch: Successfully indexed batch", mlog.Int("batch_size", len(batch)))
	}
}

// indexMessageBatch indexes a batch of messages using ElasticSearch bulk API
func (s *SznSearchImpl) indexMessageBatch(messages []common.IndexedMessage) *model.AppError {
	if len(messages) == 0 {
		return nil
	}

	// Check circuit breaker first
	if !s.circuitBreaker.AllowRequest() {
		s.Platform.Log().Warn("SznSearch: Circuit breaker open, skipping batch indexing",
			mlog.Int("batch_size", len(messages)))
		return model.NewAppError("SznSearch.indexMessageBatch", "sznsearch.circuit_breaker_open", nil, "Circuit breaker is open", 503)
	}

	s.Platform.Log().Debug("SznSearch: Building bulk request", mlog.Int("num_messages", len(messages)))

	// Build bulk request body (newline-delimited JSON)
	var buf bytes.Buffer

	for _, msg := range messages {
		// Index operation
		meta := map[string]any{
			"index": map[string]any{
				"_index": common.MessageIndex,
				"_id":    msg.ID,
			},
		}
		if err := json.NewEncoder(&buf).Encode(meta); err != nil {
			s.Platform.Log().Error("SznSearch: Failed to encode bulk meta",
				mlog.String("post_id", msg.ID),
				mlog.Err(err),
			)
			return model.NewAppError("SznSearch.indexMessageBatch", "sznsearch.indexer.encode_meta", nil, err.Error(), 500)
		}

		// Document
		doc := map[string]any{
			"Message":     msg.Message,
			"Payload":     msg.Payload,
			"Hashtags":    msg.Hashtags,
			"CreatedAt":   msg.CreatedAt,
			"ChannelId":   msg.ChannelID,
			"ChannelType": msg.ChannelType,
			"TeamId":      msg.TeamID,
			"UserId":      msg.UserID,
			"Members":     msg.Members,
		}
		if err := json.NewEncoder(&buf).Encode(doc); err != nil {
			s.Platform.Log().Error("SznSearch: Failed to encode bulk document",
				mlog.String("post_id", msg.ID),
				mlog.Err(err),
			)
			return model.NewAppError("SznSearch.indexMessageBatch", "sznsearch.indexer.encode_doc", nil, err.Error(), 500)
		}
	}

	// Execute bulk request with retry + backoff
	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		bufCopy := bytes.NewBuffer(buf.Bytes()) // Create copy for retry
		res, bulkErr := s.client.Bulk(bufCopy, s.client.Bulk.WithContext(context.Background()))
		if bulkErr != nil {
			return bulkErr
		}
		defer res.Body.Close()

		// Check for HTTP errors inside retry loop
		if res.IsError() {
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("indexMessageBatch", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("indexMessageBatch", "es_server_error", nil, res.String(), res.StatusCode)
			}
		}

		return nil
	})

	if err != nil {
		s.Platform.Log().Error("SznSearch: Bulk request failed", mlog.Err(err))
		// Record circuit breaker failure for:
		// - 5xx errors (server errors)
		// - Network errors (timeouts, connection failures)
		// Note: 4xx errors are wrapped in NonRetryableError by retry wrapper, so they stop retry immediately
		var appErr *model.AppError
		if errors.As(err, &appErr) && appErr.StatusCode >= 500 {
			// 5xx server errors trigger circuit breaker
			s.circuitBreaker.RecordFailure()
		} else if !errors.As(err, &appErr) {
			// Non-AppError means network/timeout error - trigger circuit breaker
			s.circuitBreaker.RecordFailure()
		}
		return model.NewAppError("SznSearch.indexMessageBatch", "sznsearch.indexer.bulk_error", nil, err.Error(), 500)
	}

	// Success
	s.circuitBreaker.RecordSuccess()
	return nil
}

// MakeWorker creates a simple indexer worker (for job interface compatibility)
func (s *SznSearchImpl) MakeWorker() model.Worker {
	// Return nil - bulk indexing through jobs is not implemented yet
	return nil
}
