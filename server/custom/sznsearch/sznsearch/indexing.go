package sznsearch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

// IndexPost indexes a post in ElasticSearch
func (s *SznSearchImpl) IndexPost(post *model.Post, teamId string) *model.AppError {
	if !s.IsActive() {
		return nil // Not an error if not started
	}

	// Fast O(1) check: ignore if channel is in ignore list
	if s.ignoreChannels[post.ChannelId] {
		return nil
	}

	// Fast O(1) check: ignore if team is in ignore list
	if teamId != "" && s.ignoreTeams[teamId] {
		return nil
	}

	s.Platform.Log().Debug("SznSearch: IndexPost called",
		mlog.String("post_id", post.Id),
		mlog.String("channel_id", post.ChannelId),
		mlog.String("user_id", post.UserId),
	)

	msg, err := s.formatPostForIndex(post)
	if err != nil {
		s.Platform.Log().Error("SznSearch: Failed to format post for indexing",
			mlog.String("post_id", post.Id),
			mlog.Err(err),
		)
		return err
	}

	// Check if the message queue size exceeds the limit
	if len(s.messageQueue) >= *s.Platform.Config().SznSearchSettings.MessageQueueSize {
		s.Platform.Log().Error("SznSearch: Message queue is full, dropping message",
			mlog.String("post_id", post.Id),
			mlog.String("channel_id", post.ChannelId),
			mlog.String("user_id", post.UserId),
		)
		return model.NewAppError("SznSearch.IndexPost", "sznsearch.index_post.queue_full", nil, "Message queue is full", http.StatusTooManyRequests)
	}

	// Add to message queue for async indexing (always async)
	s.mutex.WLock(common.MutexMessageQueue)
	s.messageQueue[post.Id] = msg
	s.mutex.WUnlock(common.MutexMessageQueue)
	return nil
}

// DeletePost removes a post from the ElasticSearch index
func (s *SznSearchImpl) DeletePost(post *model.Post) *model.AppError {
	if !s.IsActive() {
		return nil
	}

	// Check circuit breaker first
	if !s.circuitBreaker.AllowRequest() {
		s.Platform.Log().Warn("SznSearch: Circuit breaker open, skipping delete",
			mlog.String("post_id", post.Id))
		return model.NewAppError("SznSearch.DeletePost", "sznsearch.circuit_breaker_open", nil, "Circuit breaker is open", http.StatusServiceUnavailable)
	}

	s.Platform.Log().Debug("SznSearch: DeletePost called",
		mlog.String("post_id", post.Id),
		mlog.String("channel_id", post.ChannelId),
	)

	// Execute delete with retry + backoff
	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		res, delErr := s.client.Delete(common.MessageIndex, post.Id)
		if delErr != nil {
			return delErr
		}
		defer res.Body.Close()

		if res.IsError() && res.StatusCode != http.StatusNotFound {
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("DeletePost", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("DeletePost", "es_error", nil, res.String(), res.StatusCode)
			}
		}
		return nil
	})

	if err != nil {
		s.Platform.Log().Error("SznSearch: Failed to delete post",
			mlog.String("post_id", post.Id),
			mlog.Err(err),
		)
		// Record circuit breaker failure for 5xx and network errors, not 4xx
		// Note: 4xx errors are wrapped in NonRetryableError by retry wrapper
		var appErr *model.AppError
		if errors.As(err, &appErr) && appErr.StatusCode >= 500 {
			s.circuitBreaker.RecordFailure()
		} else if !errors.As(err, &appErr) {
			s.circuitBreaker.RecordFailure()
		}
		return model.NewAppError("SznSearch.DeletePost", "sznsearch.delete_post.error", nil, err.Error(), http.StatusInternalServerError)
	}

	s.circuitBreaker.RecordSuccess()
	s.Platform.Log().Debug("SznSearch: Post deleted successfully", mlog.String("post_id", post.Id))
	return nil
}

// DeleteChannelPosts removes all posts from a channel
func (s *SznSearchImpl) DeleteChannelPosts(rctx request.CTX, channelID string) *model.AppError {
	if !s.IsActive() {
		return nil // Not an error if not started
	}

	// Check circuit breaker first
	if !s.circuitBreaker.AllowRequest() {
		rctx.Logger().Warn("Circuit breaker open, skipping channel posts delete", mlog.String("channel_id", channelID))
		return model.NewAppError("SznSearch.DeleteChannelPosts", "sznsearch.circuit_breaker_open", nil, "Circuit breaker is open", http.StatusServiceUnavailable)
	}

	query := map[string]any{
		"query": map[string]any{
			"term": map[string]any{
				"ChannelId": channelID,
			},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return model.NewAppError("SznSearch.DeleteChannelPosts", "sznsearch.delete_channel_posts.encode", nil, err.Error(), http.StatusInternalServerError)
	}

	// Execute delete by query with retry + backoff
	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, rctx.Logger(), func() error {
		bufCopy := bytes.NewBuffer(buf.Bytes())
		res, delErr := s.client.DeleteByQuery([]string{common.MessageIndex}, bufCopy)
		if delErr != nil {
			return delErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("DeleteChannelPosts", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("DeleteChannelPosts", "es_error", nil, res.String(), res.StatusCode)
			}
		}
		return nil
	})

	if err != nil {
		rctx.Logger().Warn("Failed to delete channel posts from index", mlog.String("channel_id", channelID), mlog.Err(err))
		// Record circuit breaker failure for 5xx and network errors, not 4xx
		// Note: 4xx errors are wrapped in NonRetryableError by retry wrapper
		var appErr *model.AppError
		if errors.As(err, &appErr) && appErr.StatusCode >= 500 {
			s.circuitBreaker.RecordFailure()
		} else if !errors.As(err, &appErr) {
			s.circuitBreaker.RecordFailure()
		}
	} else {
		s.circuitBreaker.RecordSuccess()
	}

	return nil
}

// DeleteUserPosts removes all posts from a user
func (s *SznSearchImpl) DeleteUserPosts(rctx request.CTX, userID string) *model.AppError {
	if !s.IsActive() {
		return nil
	}

	// Check circuit breaker first
	if !s.circuitBreaker.AllowRequest() {
		rctx.Logger().Warn("Circuit breaker open, skipping user posts delete", mlog.String("user_id", userID))
		return model.NewAppError("SznSearch.DeleteUserPosts", "sznsearch.circuit_breaker_open", nil, "Circuit breaker is open", http.StatusServiceUnavailable)
	}

	query := map[string]any{
		"query": map[string]any{
			"term": map[string]any{
				"UserId": userID,
			},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return model.NewAppError("SznSearch.DeleteUserPosts", "sznsearch.delete_user_posts.encode", nil, err.Error(), http.StatusInternalServerError)
	}

	// Execute delete by query with retry + backoff
	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, rctx.Logger(), func() error {
		bufCopy := bytes.NewBuffer(buf.Bytes())
		res, delErr := s.client.DeleteByQuery([]string{common.MessageIndex}, bufCopy)
		if delErr != nil {
			return delErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("DeleteUserPosts", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("DeleteUserPosts", "es_error", nil, res.String(), res.StatusCode)
			}
		}
		return nil
	})

	if err != nil {
		rctx.Logger().Warn("Failed to delete user posts from index", mlog.String("user_id", userID), mlog.Err(err))
		// Record circuit breaker failure for 5xx and network errors, not 4xx
		// Note: 4xx errors are wrapped in NonRetryableError by retry wrapper
		var appErr *model.AppError
		if errors.As(err, &appErr) && appErr.StatusCode >= 500 {
			s.circuitBreaker.RecordFailure()
		} else if !errors.As(err, &appErr) {
			s.circuitBreaker.RecordFailure()
		}
	} else {
		s.circuitBreaker.RecordSuccess()
	}

	return nil
}

// formatPostForIndex converts a post to IndexedMessage format
func (s *SznSearchImpl) formatPostForIndex(post *model.Post) (*common.IndexedMessage, *model.AppError) {
	// Get channel to determine type and members
	channel, err := s.Platform.Store.Channel().Get(post.ChannelId, true)
	if err != nil {
		s.Platform.Log().Error("SznSearch: Failed to get channel for post",
			mlog.String("post_id", post.Id),
			mlog.String("channel_id", post.ChannelId),
			mlog.Err(err),
		)
		return nil, model.NewAppError("SznSearch.formatPostForIndex", "sznsearch.format_post.get_channel", nil, err.Error(), http.StatusInternalServerError)
	}

	channelType := common.GetChannelTypeInt(string(channel.Type))
	teamID := channel.TeamId
	if teamID == "" {
		teamID = common.NoTeamID
	}

	members := make([]string, 0)
	// For direct and group channels, get member list
	if channel.Type == model.ChannelTypeDirect || channel.Type == model.ChannelTypeGroup {
		memberList, err := s.Platform.Store.Channel().GetMembers(model.ChannelMembersGetOptions{
			ChannelID: channel.Id,
		})
		if err == nil {
			for _, m := range memberList {
				members = append(members, m.UserId)
			}
		}
	}

	payload := ""
	for _, att := range post.Attachments() {
		payload += fmt.Sprintf("%s %s ", att.Title, att.Text)
	}

	// Extract hashtags from post
	var hashtags []string
	if post.Hashtags != "" {
		// Post.Hashtags is a space-separated string like "#tag1 #tag2"
		// Split and clean them
		hashtagList := strings.Fields(post.Hashtags)
		for _, tag := range hashtagList {
			// Remove the # prefix if present and normalize to lowercase for case-insensitive search
			tag = strings.TrimPrefix(tag, "#")
			tag = strings.ToLower(tag)
			if tag != "" {
				hashtags = append(hashtags, tag)
			}
		}
	}

	indexedMsg := &common.IndexedMessage{
		ID:          post.Id,
		Message:     post.Message,
		Payload:     payload,
		Hashtags:    hashtags,
		CreatedAt:   post.CreateAt,
		ChannelID:   post.ChannelId,
		ChannelType: channelType,
		TeamID:      teamID,
		UserID:      post.UserId,
		Members:     members,
	}

	return indexedMsg, nil
}
