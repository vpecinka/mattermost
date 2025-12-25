// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.
//
// This code is a derivative work based on Mattermost server
// (https://github.com/mattermost/mattermost-server)
// Original Mattermost code is licensed under AGPL v3.0.
//
// This derivative work is used exclusively for internal purposes
// within Seznam.cz, a.s. and is not distributed to third parties.
// Therefore, the copyleft provisions of AGPL v3.0 do not apply
// as per the license's distribution requirements.

package sznsearch

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

// IndexChannel indexes a channel metadata for autocomplete
func (s *SznSearchImpl) IndexChannel(rctx request.CTX, channel *model.Channel, userIDs, teamMemberIDs []string) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return model.NewAppError("SznSearch.IndexChannel", "sznsearch.not_started", nil, "", http.StatusInternalServerError)
	}

	if !s.isBackendHealthy() {
		rctx.Logger().Warn("SznSearch.IndexChannel: ES backend unhealthy, skipping channel indexing",
			mlog.String("channel_id", channel.Id),
		)
		return nil // Don't fail hard on indexing when backend is down
	}

	// Convert channel to ES document
	searchChannel := common.ESChannelFromChannel(channel, userIDs, teamMemberIDs)

	// Serialize to JSON
	channelJSON, err := json.Marshal(searchChannel)
	if err != nil {
		return model.NewAppError("SznSearch.IndexChannel", "sznsearch.index_channel.marshal", nil, err.Error(), http.StatusInternalServerError)
	}

	// Index with retry
	indexErr := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, rctx.Logger(), func() error {
		res, esErr := s.client.Index(
			common.ChannelIndex,
			bytes.NewReader(channelJSON),
			s.client.Index.WithDocumentID(searchChannel.Id),
			s.client.Index.WithRefresh("false"), // async refresh for better performance
		)
		if esErr != nil {
			return esErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("IndexChannel", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("IndexChannel", "es_server_error", nil, res.String(), res.StatusCode)
			}
		}
		return nil
	})

	if indexErr != nil {
		var appErr *model.AppError
		if errors.As(indexErr, &appErr) {
			return appErr
		}
		return model.NewAppError("SznSearch.IndexChannel", "sznsearch.index_channel.error", nil, indexErr.Error(), http.StatusInternalServerError)
	}

	rctx.Logger().Debug("SznSearch: Channel indexed successfully",
		mlog.String("channel_id", channel.Id),
		mlog.String("channel_name", channel.Name),
	)

	// Record metrics
	if metrics := s.Platform.Metrics(); metrics != nil {
		metrics.IncrementChannelIndexCounter()
	}

	return nil
}

// SyncBulkIndexChannels bulk indexes channels
func (s *SznSearchImpl) SyncBulkIndexChannels(rctx request.CTX, channels []*model.Channel, getUserIDsForChannel func(channel *model.Channel) ([]string, error), teamMemberIDs []string) *model.AppError {
	// Not implemented - channels are not indexed separately
	return nil
}

// DeleteChannel removes a channel from the index
func (s *SznSearchImpl) DeleteChannel(channel *model.Channel) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return model.NewAppError("SznSearch.DeleteChannel", "sznsearch.not_started", nil, "", http.StatusInternalServerError)
	}

	if !s.isBackendHealthy() {
		s.Platform.Log().Warn("SznSearch.DeleteChannel: ES backend unhealthy, skipping channel deletion",
			mlog.String("channel_id", channel.Id),
		)
		return nil // Don't fail hard on deletion when backend is down
	}

	// Delete with retry
	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		res, delErr := s.client.Delete(
			common.ChannelIndex,
			channel.Id,
		)
		if delErr != nil {
			return delErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 404 is OK - channel doesn't exist
			if res.StatusCode == http.StatusNotFound {
				return nil
			}
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("DeleteChannel", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("DeleteChannel", "es_server_error", nil, res.String(), res.StatusCode)
			}
		}
		return nil
	})

	if err != nil {
		var appErr *model.AppError
		if errors.As(err, &appErr) {
			return appErr
		}
		return model.NewAppError("SznSearch.DeleteChannel", "sznsearch.delete_channel.error", nil, err.Error(), http.StatusInternalServerError)
	}

	s.Platform.Log().Debug("SznSearch: Channel deleted from index",
		mlog.String("channel_id", channel.Id),
		mlog.String("channel_name", channel.Name),
	)

	return nil
}

// IndexUser indexes a user
func (s *SznSearchImpl) IndexUser(rctx request.CTX, user *model.User, teamsIds, channelsIds []string) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return model.NewAppError("SznSearch.IndexUser", "sznsearch.not_started", nil, "", http.StatusInternalServerError)
	}

	if !s.isBackendHealthy() {
		rctx.Logger().Warn("SznSearch.IndexUser: ES backend unhealthy, skipping user indexing",
			mlog.String("user_id", user.Id),
		)
		return nil // Don't fail hard on indexing when backend is down
	}

	// Convert user to ES document
	searchUser := common.ESUserFromUserAndTeams(user, teamsIds, channelsIds)

	// Serialize to JSON
	userJSON, err := json.Marshal(searchUser)
	if err != nil {
		return model.NewAppError("SznSearch.IndexUser", "sznsearch.index_user.marshal", nil, err.Error(), http.StatusInternalServerError)
	}

	// Index with retry
	indexErr := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, rctx.Logger(), func() error {
		res, esErr := s.client.Index(
			common.UserIndex,
			bytes.NewReader(userJSON),
			s.client.Index.WithDocumentID(searchUser.Id),
			s.client.Index.WithRefresh("false"), // async refresh for better performance
		)
		if esErr != nil {
			return esErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("IndexUser", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("IndexUser", "es_server_error", nil, res.String(), res.StatusCode)
			}
		}
		return nil
	})

	if indexErr != nil {
		var appErr *model.AppError
		if errors.As(indexErr, &appErr) {
			return appErr
		}
		return model.NewAppError("SznSearch.IndexUser", "sznsearch.index_user.error", nil, indexErr.Error(), http.StatusInternalServerError)
	}

	rctx.Logger().Debug("SznSearch: User indexed successfully",
		mlog.String("user_id", user.Id),
		mlog.String("username", user.Username),
	)

	// Record metrics
	if metrics := s.Platform.Metrics(); metrics != nil {
		metrics.IncrementUserIndexCounter()
	}

	return nil
}

// DeleteUser removes a user from the index
func (s *SznSearchImpl) DeleteUser(user *model.User) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return model.NewAppError("SznSearch.DeleteUser", "sznsearch.not_started", nil, "", http.StatusInternalServerError)
	}

	if !s.isBackendHealthy() {
		s.Platform.Log().Warn("SznSearch.DeleteUser: ES backend unhealthy, skipping user deletion",
			mlog.String("user_id", user.Id),
		)
		return nil // Don't fail hard on deletion when backend is down
	}

	// Delete with retry
	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		res, delErr := s.client.Delete(
			common.UserIndex,
			user.Id,
		)
		if delErr != nil {
			return delErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 404 is OK - user doesn't exist
			if res.StatusCode == http.StatusNotFound {
				return nil
			}
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("DeleteUser", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("DeleteUser", "es_server_error", nil, res.String(), res.StatusCode)
			}
		}
		return nil
	})

	if err != nil {
		var appErr *model.AppError
		if errors.As(err, &appErr) {
			return appErr
		}
		return model.NewAppError("SznSearch.DeleteUser", "sznsearch.delete_user.error", nil, err.Error(), http.StatusInternalServerError)
	}

	s.Platform.Log().Debug("SznSearch: User deleted from index",
		mlog.String("user_id", user.Id),
		mlog.String("username", user.Username),
	)

	return nil
}

// IndexFile indexes a file for search
func (s *SznSearchImpl) IndexFile(file *model.FileInfo, channelId string) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return model.NewAppError("SznSearch.IndexFile", "sznsearch.not_started", nil, "", http.StatusInternalServerError)
	}

	if !s.isBackendHealthy() {
		s.Platform.Log().Warn("SznSearch.IndexFile: ES backend unhealthy, skipping file indexing",
			mlog.String("file_id", file.Id),
		)
		return nil // Don't fail hard on indexing when backend is down
	}

	// Convert to ES document (content extraction and truncation happens inside)
	searchFile := common.ESFileFromFileInfo(file, channelId)

	// Serialize to JSON
	fileJSON, err := json.Marshal(searchFile)
	if err != nil {
		return model.NewAppError("SznSearch.IndexFile", "sznsearch.index_file.marshal", nil, err.Error(), http.StatusInternalServerError)
	}

	// Index with retry and circuit breaker
	indexErr := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		res, esErr := s.client.Index(
			common.FileIndex,
			bytes.NewReader(fileJSON),
			s.client.Index.WithDocumentID(searchFile.ID),
			s.client.Index.WithRefresh("false"), // async refresh for better performance
		)
		if esErr != nil {
			return esErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("IndexFile", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("IndexFile", "es_server_error", nil, res.String(), res.StatusCode)
			}
		}
		return nil
	})

	if indexErr != nil {
		var appErr *model.AppError
		if errors.As(indexErr, &appErr) && appErr.StatusCode >= 500 {
			s.circuitBreaker.RecordFailure()
		} else if !errors.As(indexErr, &appErr) {
			s.circuitBreaker.RecordFailure()
		}

		if errors.As(indexErr, &appErr) {
			return appErr
		}
		return model.NewAppError("SznSearch.IndexFile", "sznsearch.index_file.error", nil, indexErr.Error(), http.StatusInternalServerError)
	}

	s.circuitBreaker.RecordSuccess()

	s.Platform.Log().Debug("SznSearch: File indexed successfully",
		mlog.String("file_id", file.Id),
		mlog.String("file_name", file.Name),
	)

	return nil
}

// DeleteFile removes a file from the index
func (s *SznSearchImpl) DeleteFile(fileID string) *model.AppError {
	return s.deleteDocumentByID(common.FileIndex, fileID, "DeleteFile")
}

// DeletePostFiles removes all files associated with a post
func (s *SznSearchImpl) DeletePostFiles(rctx request.CTX, postID string) *model.AppError {
	return s.deleteDocumentsByQuery(rctx, common.FileIndex, "PostId", postID, "DeletePostFiles")
}

// DeleteUserFiles removes all files associated with a user
func (s *SznSearchImpl) DeleteUserFiles(rctx request.CTX, userID string) *model.AppError {
	return s.deleteDocumentsByQuery(rctx, common.FileIndex, "CreatorId", userID, "DeleteUserFiles")
}

// DeleteFilesBatch removes files created before endTime (for data retention)
func (s *SznSearchImpl) DeleteFilesBatch(rctx request.CTX, endTime, limit int64) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return nil
	}

	if !s.isBackendHealthy() {
		return nil
	}

	// Delete by query: files with CreateAt <= endTime
	query := map[string]any{
		"query": map[string]any{
			"range": map[string]any{
				"CreateAt": map[string]any{
					"lte": endTime,
				},
			},
		},
	}

	if limit > 0 {
		query["size"] = limit
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return model.NewAppError("SznSearch.DeleteFilesBatch", "sznsearch.delete_files_batch.encode", nil, err.Error(), http.StatusInternalServerError)
	}

	res, err := s.client.DeleteByQuery(
		[]string{common.FileIndex},
		&buf,
	)
	if err != nil {
		rctx.Logger().Error("SznSearch: Failed to delete files batch", mlog.Err(err))
		return model.NewAppError("SznSearch.DeleteFilesBatch", "sznsearch.delete_files_batch.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() {
		rctx.Logger().Error("SznSearch: ES error deleting files batch", mlog.String("response", res.String()))
		return model.NewAppError("SznSearch.DeleteFilesBatch", "sznsearch.delete_files_batch.es_error", nil, res.String(), res.StatusCode)
	}

	return nil
}

// deleteDocumentByID is a helper function to delete a single document by ID from an index
func (s *SznSearchImpl) deleteDocumentByID(indexName, documentID, operation string) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return model.NewAppError("SznSearch."+operation, "sznsearch.not_started", nil, "", http.StatusInternalServerError)
	}

	if !s.isBackendHealthy() {
		s.Platform.Log().Warn("SznSearch."+operation+": ES backend unhealthy, skipping deletion",
			mlog.String("document_id", documentID),
			mlog.String("index", indexName),
		)
		return nil // Don't fail hard on deletion when backend is down
	}

	// Delete with retry
	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		res, delErr := s.client.Delete(indexName, documentID)
		if delErr != nil {
			return delErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 404 is OK - document doesn't exist
			if res.StatusCode == http.StatusNotFound {
				return nil
			}
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError(operation, "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError(operation, "es_server_error", nil, res.String(), res.StatusCode)
			}
		}
		return nil
	})

	if err != nil {
		var appErr *model.AppError
		if errors.As(err, &appErr) && appErr.StatusCode >= 500 {
			s.circuitBreaker.RecordFailure()
		} else if !errors.As(err, &appErr) {
			s.circuitBreaker.RecordFailure()
		}

		if errors.As(err, &appErr) {
			return appErr
		}
		return model.NewAppError("SznSearch."+operation, "sznsearch.delete_document.error", nil, err.Error(), http.StatusInternalServerError)
	}

	s.circuitBreaker.RecordSuccess()

	s.Platform.Log().Debug("SznSearch: Document deleted from index",
		mlog.String("document_id", documentID),
		mlog.String("index", indexName),
	)

	return nil
}

// deleteDocumentsByQuery is a helper function to delete documents matching a query
func (s *SznSearchImpl) deleteDocumentsByQuery(rctx request.CTX, indexName, fieldName, fieldValue, operation string) *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return nil
	}

	if !s.isBackendHealthy() {
		return nil
	}

	// Delete by query
	query := map[string]any{
		"query": map[string]any{
			"term": map[string]any{
				fieldName: fieldValue,
			},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return model.NewAppError("SznSearch."+operation, "sznsearch.delete_by_query.encode", nil, err.Error(), http.StatusInternalServerError)
	}

	res, err := s.client.DeleteByQuery(
		[]string{indexName},
		&buf,
	)
	if err != nil {
		rctx.Logger().Error("SznSearch: Failed to delete documents by query",
			mlog.String("operation", operation),
			mlog.String("index", indexName),
			mlog.String("field", fieldName),
			mlog.Err(err),
		)
		return model.NewAppError("SznSearch."+operation, "sznsearch.delete_by_query.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() {
		rctx.Logger().Error("SznSearch: ES error deleting documents by query",
			mlog.String("operation", operation),
			mlog.String("response", res.String()),
		)
		return model.NewAppError("SznSearch."+operation, "sznsearch.delete_by_query.es_error", nil, res.String(), res.StatusCode)
	}

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

	// Delete all indices with retry
	indicesToDelete := []string{
		common.MessageIndex,
		common.UserIndex,
		common.ChannelIndex,
		common.FileIndex,
	}

	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, rctx.Logger(), func() error {
		res, delErr := s.client.Indices.Delete(indicesToDelete)
		if delErr != nil {
			return delErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 404 is OK - indices don't exist
			if res.StatusCode == http.StatusNotFound {
				return nil
			}
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("PurgeIndexes", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("PurgeIndexes", "es_server_error", nil, res.String(), res.StatusCode)
			}
		}
		return nil
	})

	if err != nil {
		// Log but don't fail on circuit breaker for manual operations
		var appErr *model.AppError
		if errors.As(err, &appErr) {
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

	indicesToRefresh := []string{
		common.MessageIndex,
		common.UserIndex,
		common.ChannelIndex,
		common.FileIndex,
	}

	res, err := s.client.Indices.Refresh(
		s.client.Indices.Refresh.WithIndex(indicesToRefresh...),
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
