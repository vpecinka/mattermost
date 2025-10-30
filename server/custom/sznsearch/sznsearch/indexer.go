package sznsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

const (
	indexerTickInterval = 5 * time.Second
	batchSize           = 100
)

// startIndexer starts the background indexer worker
func (s *SznSearchImpl) startIndexer() {
	s.Platform.Log().Info("Starting SznSearch indexer worker")

	ticker := time.NewTicker(indexerTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			s.Platform.Log().Info("Stopping SznSearch indexer worker")
			return
		case <-ticker.C:
			s.processMessageQueue()
		}
	}
}

// processMessageQueue processes messages from the queue and indexes them
func (s *SznSearchImpl) processMessageQueue() {
	if !s.IsActive() {
		return
	}

	s.mutex.WLock(common.MutexMessageQueue)
	if len(s.messageQueue) == 0 {
		s.mutex.WUnlock(common.MutexMessageQueue)
		return
	}

	// Collect batch of messages
	batch := make([]common.IndexedMessage, 0, batchSize)
	for channelID, messages := range s.messageQueue {
		for _, msg := range messages {
			batch = append(batch, *msg)
			if len(batch) >= batchSize {
				break
			}
		}
		// Clear processed messages for this channel
		delete(s.messageQueue, channelID)
		if len(batch) >= batchSize {
			break
		}
	}
	s.mutex.WUnlock(common.MutexMessageQueue)

	if len(batch) == 0 {
		return
	}

	// Index batch
	s.Platform.Log().Debug("SznSearch indexing batch", mlog.Int("size", len(batch)))
	if err := s.indexMessageBatch(batch); err != nil {
		s.Platform.Log().Error("Failed to index message batch", mlog.Err(err))
	}
}

// indexMessageBatch indexes a batch of messages using ElasticSearch bulk API
func (s *SznSearchImpl) indexMessageBatch(messages []common.IndexedMessage) *model.AppError {
	if len(messages) == 0 {
		return nil
	}

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
			return model.NewAppError("SznSearch.indexMessageBatch", "sznsearch.indexer.encode_meta", nil, err.Error(), 500)
		}

		// Document
		doc := map[string]any{
			"Message":     msg.Message,
			"Payload":     msg.Payload,
			"CreatedAt":   msg.CreatedAt,
			"ChannelId":   msg.ChannelID,
			"ChannelType": msg.ChannelType,
			"TeamId":      msg.TeamID,
			"UserId":      msg.UserID,
			"Members":     msg.Members,
		}
		if err := json.NewEncoder(&buf).Encode(doc); err != nil {
			return model.NewAppError("SznSearch.indexMessageBatch", "sznsearch.indexer.encode_doc", nil, err.Error(), 500)
		}
	}

	// Execute bulk request
	res, err := s.client.Bulk(
		&buf,
		s.client.Bulk.WithContext(context.Background()),
	)
	if err != nil {
		return model.NewAppError("SznSearch.indexMessageBatch", "sznsearch.indexer.bulk_error", nil, err.Error(), 500)
	}
	defer res.Body.Close()

	if res.IsError() {
		return model.NewAppError("SznSearch.indexMessageBatch", "sznsearch.indexer.bulk_es_error", nil, res.String(), 500)
	}

	return nil
}

// MakeWorker creates a simple indexer worker (for job interface compatibility)
func (s *SznSearchImpl) MakeWorker() model.Worker {
	// Return nil - bulk indexing through jobs is not implemented yet
	return nil
}
