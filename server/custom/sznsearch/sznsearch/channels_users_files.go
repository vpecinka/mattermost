package sznsearch

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

// IndexChannel indexes a channel (metadata, not used for post search)
func (s *SznSearchImpl) IndexChannel(rctx request.CTX, channel *model.Channel, userIDs, teamMemberIDs []string) *model.AppError {
	// Channels are not indexed separately in this implementation
	// Channel information is embedded in post documents
	return nil
}

// SyncBulkIndexChannels bulk indexes channels
func (s *SznSearchImpl) SyncBulkIndexChannels(rctx request.CTX, channels []*model.Channel, getUserIDsForChannel func(channel *model.Channel) ([]string, error), teamMemberIDs []string) *model.AppError {
	// Not implemented - channels are not indexed separately
	return nil
}

// DeleteChannel removes a channel from the index
func (s *SznSearchImpl) DeleteChannel(channel *model.Channel) *model.AppError {
	// Not implemented - channels are not indexed separately
	return nil
}

// IndexUser indexes a user
func (s *SznSearchImpl) IndexUser(rctx request.CTX, user *model.User, teamsIds, channelsIds []string) *model.AppError {
	// Users are not indexed in this implementation
	// User search is handled by the database
	return nil
}

// DeleteUser removes a user from the index
func (s *SznSearchImpl) DeleteUser(user *model.User) *model.AppError {
	// Users are not indexed in this implementation
	return nil
}

// IndexFile indexes a file
func (s *SznSearchImpl) IndexFile(file *model.FileInfo, channelId string) *model.AppError {
	// Files are not indexed in this implementation yet
	return nil
}

// DeleteFile removes a file from the index
func (s *SznSearchImpl) DeleteFile(fileID string) *model.AppError {
	// Files are not indexed in this implementation yet
	return nil
}

// DeletePostFiles removes all files associated with a post
func (s *SznSearchImpl) DeletePostFiles(rctx request.CTX, postID string) *model.AppError {
	// Files are not indexed in this implementation yet
	return nil
}

// DeleteUserFiles removes all files associated with a user
func (s *SznSearchImpl) DeleteUserFiles(rctx request.CTX, userID string) *model.AppError {
	// Files are not indexed in this implementation yet
	return nil
}

// DeleteFilesBatch removes files in batch
func (s *SznSearchImpl) DeleteFilesBatch(rctx request.CTX, endTime, limit int64) *model.AppError {
	// Files are not indexed in this implementation yet
	return nil
}

// TestConfig tests the ElasticSearch configuration
func (s *SznSearchImpl) TestConfig(rctx request.CTX, cfg *model.Config) *model.AppError {
	// Create a test client with the provided config
	testEngine := &SznSearchImpl{
		Platform: s.Platform,
		mutex:    common.NewKeyedMutex(),
	}

	client, err := testEngine.createClient()
	if err != nil {
		if appErr, ok := err.(*model.AppError); ok {
			return appErr
		}
		return model.NewAppError("SznSearch.TestConfig", "sznsearch.test_config.create_client", nil, err.Error(), http.StatusInternalServerError)
	}

	// Test connection
	info, esErr := client.Info()
	if esErr != nil {
		return model.NewAppError("SznSearch.TestConfig", "sznsearch.test_config.connection_failed", nil, esErr.Error(), http.StatusInternalServerError)
	}
	defer info.Body.Close()

	if info.IsError() {
		return model.NewAppError("SznSearch.TestConfig", "sznsearch.test_config.connection_error", nil, info.String(), http.StatusInternalServerError)
	}

	return nil
}

// PurgeIndexes purges all indexes
func (s *SznSearchImpl) PurgeIndexes(rctx request.CTX) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return model.NewAppError("SznSearch.PurgeIndexes", "sznsearch.not_started", nil, "", http.StatusInternalServerError)
	}

	// Delete message index with retry
	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, rctx.Logger(), func() error {
		res, delErr := s.client.Indices.Delete([]string{common.MessageIndex})
		if delErr != nil {
			return delErr
		}
		defer res.Body.Close()

		if res.IsError() {
			return model.NewAppError("PurgeIndexes", "sznsearch.purge_indexes.error", nil, res.String(), res.StatusCode)
		}
		return nil
	})

	if err != nil {
		if appErr, ok := err.(*model.AppError); ok {
			return appErr
		}
		return model.NewAppError("SznSearch.PurgeIndexes", "sznsearch.purge_indexes.delete_error", nil, err.Error(), http.StatusInternalServerError)
	}

	// Recreate indexes
	if err := s.ensureIndices(); err != nil {
		if appErr, ok := err.(*model.AppError); ok {
			return appErr
		}
		return model.NewAppError("SznSearch.PurgeIndexes", "sznsearch.purge_indexes.ensure_indices", nil, err.Error(), http.StatusInternalServerError)
	}
	return nil
}

// PurgeIndexList purges specific indexes
func (s *SznSearchImpl) PurgeIndexList(rctx request.CTX, indexes []string) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return model.NewAppError("SznSearch.PurgeIndexList", "sznsearch.not_started", nil, "", http.StatusInternalServerError)
	}

	res, err := s.client.Indices.Delete(indexes)
	if err != nil {
		return model.NewAppError("SznSearch.PurgeIndexList", "sznsearch.purge_index_list.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	return nil
}

// RefreshIndexes refreshes the ElasticSearch indexes
func (s *SznSearchImpl) RefreshIndexes(rctx request.CTX) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return nil
	}

	res, err := s.client.Indices.Refresh(
		s.client.Indices.Refresh.WithIndex(common.MessageIndex),
	)
	if err != nil {
		rctx.Logger().Warn("Failed to refresh ElasticSearch indexes", mlog.Err(err))
		return nil // Don't fail on refresh errors
	}
	defer res.Body.Close()

	return nil
}

// DataRetentionDeleteIndexes deletes indexes older than cutoff time
func (s *SznSearchImpl) DataRetentionDeleteIndexes(rctx request.CTX, cutoff time.Time) *model.AppError {
	// Not implemented - data retention is handled differently
	return nil
}
