package sznsearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

	s.Platform.Log().Debug("SznSearch: DeletePost called",
		mlog.String("post_id", post.Id),
		mlog.String("channel_id", post.ChannelId),
	)

	res, err := s.client.Delete(common.MessageIndex, post.Id)
	if err != nil {
		s.Platform.Log().Error("SznSearch: Failed to delete post",
			mlog.String("post_id", post.Id),
			mlog.Err(err),
		)
		s.markUnhealthy()
		return model.NewAppError("SznSearch.DeletePost", "sznsearch.delete_post.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() && res.StatusCode != http.StatusNotFound {
		s.Platform.Log().Error("SznSearch: ElasticSearch error deleting post",
			mlog.String("post_id", post.Id),
			mlog.Int("status_code", res.StatusCode),
			mlog.String("response", res.String()),
		)
		// Mark ES as unhealthy on 5xx errors
		if res.StatusCode >= 500 {
			s.markUnhealthy()
		}
		return model.NewAppError("SznSearch.DeletePost", "sznsearch.delete_post.es_error", nil, res.String(), http.StatusInternalServerError)
	}

	s.Platform.Log().Debug("SznSearch: Post deleted successfully", mlog.String("post_id", post.Id))
	return nil
}

// DeleteChannelPosts removes all posts from a channel
func (s *SznSearchImpl) DeleteChannelPosts(rctx request.CTX, channelID string) *model.AppError {
	if !s.IsActive() {
		return nil // Not an error if not started
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

	res, err := s.client.DeleteByQuery(
		[]string{common.MessageIndex},
		&buf,
	)
	if err != nil {
		s.markUnhealthy()
		return model.NewAppError("SznSearch.DeleteChannelPosts", "sznsearch.delete_channel_posts.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() {
		rctx.Logger().Warn("Failed to delete channel posts from index", mlog.String("channel_id", channelID), mlog.String("response", res.String()))
		// Mark ES as unhealthy on 5xx errors
		if res.StatusCode >= 500 {
			s.markUnhealthy()
		}
	}

	return nil
}

// DeleteUserPosts removes all posts from a user
func (s *SznSearchImpl) DeleteUserPosts(rctx request.CTX, userID string) *model.AppError {
	if !s.IsActive() {
		return nil
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

	res, err := s.client.DeleteByQuery(
		[]string{common.MessageIndex},
		&buf,
	)
	if err != nil {
		s.markUnhealthy()
		return model.NewAppError("SznSearch.DeleteUserPosts", "sznsearch.delete_user_posts.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() {
		rctx.Logger().Warn("Failed to delete user posts from index", mlog.String("user_id", userID), mlog.String("response", res.String()))
		// Mark ES as unhealthy on 5xx errors
		if res.StatusCode >= 500 {
			s.markUnhealthy()
		}
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
