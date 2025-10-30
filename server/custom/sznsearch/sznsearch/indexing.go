package sznsearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

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

	msg, err := s.formatPostForIndex(post)
	if err != nil {
		return err
	}

	// Add to message queue for async indexing
	if !s.IsIndexingSync() {
		s.mutex.WLock(common.MutexMessageQueue)
		if _, exists := s.messageQueue[post.ChannelId]; !exists {
			s.messageQueue[post.ChannelId] = make([]*common.IndexedMessage, 0)
		}
		s.messageQueue[post.ChannelId] = append(s.messageQueue[post.ChannelId], msg)
		s.mutex.WUnlock(common.MutexMessageQueue)
		return nil
	}

	// Synchronous indexing
	return s.indexMessageBatch([]common.IndexedMessage{*msg})
}

// DeletePost removes a post from the ElasticSearch index
func (s *SznSearchImpl) DeletePost(post *model.Post) *model.AppError {
	if !s.IsActive() {
		return nil
	}

	res, err := s.client.Delete(common.MessageIndex, post.Id)
	if err != nil {
		return model.NewAppError("SznSearch.DeletePost", "sznsearch.delete_post.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() && res.StatusCode != http.StatusNotFound {
		return model.NewAppError("SznSearch.DeletePost", "sznsearch.delete_post.es_error", nil, res.String(), http.StatusInternalServerError)
	}

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
		return model.NewAppError("SznSearch.DeleteChannelPosts", "sznsearch.delete_channel_posts.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() {
		rctx.Logger().Warn("Failed to delete channel posts from index", mlog.String("channel_id", channelID), mlog.String("response", res.String()))
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
		return model.NewAppError("SznSearch.DeleteUserPosts", "sznsearch.delete_user_posts.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() {
		rctx.Logger().Warn("Failed to delete user posts from index", mlog.String("user_id", userID), mlog.String("response", res.String()))
	}

	return nil
}

// formatPostForIndex converts a post to IndexedMessage format
func (s *SznSearchImpl) formatPostForIndex(post *model.Post) (*common.IndexedMessage, *model.AppError) {
	// Get channel to determine type and members
	channel, err := s.Platform.Store.Channel().Get(post.ChannelId, true)
	if err != nil {
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

	return &common.IndexedMessage{
		ID:          post.Id,
		Message:     post.Message,
		Payload:     payload,
		CreatedAt:   post.CreateAt,
		ChannelID:   post.ChannelId,
		ChannelType: channelType,
		TeamID:      teamID,
		UserID:      post.UserId,
		Members:     members,
	}, nil
}
