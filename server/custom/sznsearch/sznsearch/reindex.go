package sznsearch

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/channels/store"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

const (
	reindexChannelLimit = 1000
)

// channelCacheItem represents minimal channel info for reindexing
type channelCacheItem struct {
	ID     string
	TeamID string
	Type   model.ChannelType
}

// channelsCache holds all channels grouped by TeamID for efficient filtering
type channelsCache struct {
	byTeam map[string][]channelCacheItem // key: TeamID (or "" for DM/GM)
}

// FullReindexFromDatabase performs a full reindex of all posts from the database
func (s *SznSearchImpl) FullReindexFromDatabase(rctx request.CTX) *model.AppError {
	if !s.IsActive() {
		return model.NewAppError("SznSearch.FullReindexFromDatabase", "sznsearch.reindex.not_active", nil, "", 500)
	}

	rctx.Logger().Info("SznSearch: Starting full database reindex")

	// Build channels cache once for all teams
	rctx.Logger().Info("SznSearch: Building channels cache")
	cache, err := s.buildChannelsCache(rctx)
	if err != nil {
		return err
	}
	rctx.Logger().Info("SznSearch: Channels cache built",
		mlog.Int("total_teams", len(cache.byTeam)),
	)

	// Reindex all teams (iterate over cache keys to get all TeamIDs including "")
	for teamID := range cache.byTeam {
		// Fast O(1) check: skip ignored teams
		if teamID != "" && s.ignoreTeams[teamID] {
			rctx.Logger().Info("SznSearch: Skipping ignored team", mlog.String("team_id", teamID))
			continue
		}

		if teamID == "" {
			rctx.Logger().Info("SznSearch: Reindexing direct messages and group messages")
		} else {
			rctx.Logger().Info("SznSearch: Reindexing team", mlog.String("team_id", teamID))
		}

		if err := s.reindexTeamWithCache(rctx, teamID, cache); err != nil {
			rctx.Logger().Error("SznSearch: Failed to reindex team",
				mlog.String("team_id", teamID),
				mlog.Err(err),
			)
			// Continue with other teams
		}
	}

	rctx.Logger().Info("SznSearch: Full reindex completed")

	return nil
}

// buildChannelsCache loads all active (non-deleted) channels into memory for efficient filtering
func (s *SznSearchImpl) buildChannelsCache(rctx request.CTX) (*channelsCache, *model.AppError) {
	cache := &channelsCache{
		byTeam: make(map[string][]channelCacheItem),
	}

	offset := 0
	totalChannels := 0

	for {
		// Get all channels in batches (only active channels, no deleted/archived)
		channels, err := s.Platform.Store.Channel().GetAllChannels(offset, reindexChannelLimit, store.ChannelSearchOpts{
			IncludeDeleted: false, // Skip deleted/archived channels
		})
		if err != nil {
			return nil, model.NewAppError("SznSearch.buildChannelsCache", "sznsearch.reindex.get_channels", nil, err.Error(), 500)
		}

		if len(channels) == 0 {
			break
		}

		// Group channels by TeamID
		for _, channel := range channels {
			teamID := channel.TeamId // Empty string for DM/GM
			cache.byTeam[teamID] = append(cache.byTeam[teamID], channelCacheItem{
				ID:     channel.Id,
				TeamID: teamID,
				Type:   channel.Type,
			})
			totalChannels++
		}

		offset += reindexChannelLimit

		// Break if we got less than limit (no more channels)
		if len(channels) < reindexChannelLimit {
			break
		}
	}

	rctx.Logger().Debug("SznSearch: Channels cache built",
		mlog.Int("total_channels", totalChannels),
		mlog.Int("teams_count", len(cache.byTeam)),
	)

	return cache, nil
}

// ReindexTeam reindexes all channels in a team
// Pass empty teamID ("") to reindex all DM/GM channels
func (s *SznSearchImpl) ReindexTeam(rctx request.CTX, teamID string) *model.AppError {
	// Build cache for single team reindex (slash command usage)
	cache, err := s.buildChannelsCache(rctx)
	if err != nil {
		return err
	}

	return s.reindexTeamWithCache(rctx, teamID, cache)
}

// reindexTeamWithCache reindexes all channels in a team using provided cache
func (s *SznSearchImpl) reindexTeamWithCache(rctx request.CTX, teamID string, cache *channelsCache) *model.AppError {
	if !s.IsActive() {
		return model.NewAppError("SznSearch.reindexTeamWithCache", "sznsearch.reindex.not_active", nil, "", 500)
	}

	if teamID == "" {
		rctx.Logger().Info("SznSearch: Starting direct/group messages reindex")
	} else {
		rctx.Logger().Info("SznSearch: Starting team reindex", mlog.String("team_id", teamID))
	}

	// Get channels for this team from cache
	channels, exists := cache.byTeam[teamID]
	if !exists || len(channels) == 0 {
		rctx.Logger().Info("SznSearch: No channels found for team", mlog.String("team_id", teamID))
		return nil
	}

	totalPosts := 0
	rctx.Logger().Info("SznSearch: Processing channels",
		mlog.String("team_id", teamID),
		mlog.Int("channel_count", len(channels)),
	)

	for _, channel := range channels {
		// Fast O(1) check: skip ignored channels
		if s.ignoreChannels[channel.ID] {
			rctx.Logger().Debug("SznSearch: Skipping ignored channel",
				mlog.String("channel_id", channel.ID),
			)
			continue
		}

		rctx.Logger().Debug("SznSearch: Reindexing channel",
			mlog.String("channel_id", channel.ID),
			mlog.String("channel_type", string(channel.Type)),
		)

		if err := s.ReindexChannel(rctx, channel.ID); err != nil {
			rctx.Logger().Error("SznSearch: Failed to reindex channel",
				mlog.String("channel_id", channel.ID),
				mlog.Err(err),
			)
			// Continue with other channels
		}
	}

	if teamID == "" {
		rctx.Logger().Info("SznSearch: Direct/group messages reindex completed",
			mlog.Int("total_posts", totalPosts),
		)
	} else {
		rctx.Logger().Info("SznSearch: Team reindex completed",
			mlog.String("team_id", teamID),
			mlog.Int("total_posts", totalPosts),
		)
	}

	return nil
}

// ReindexChannel reindexes all posts in a channel
func (s *SznSearchImpl) ReindexChannel(rctx request.CTX, channelID string) *model.AppError {
	if !s.IsActive() {
		return model.NewAppError("SznSearch.ReindexChannel", "sznsearch.reindex.not_active", nil, "", 500)
	}

	rctx.Logger().Debug("SznSearch: Starting channel reindex", mlog.String("channel_id", channelID))

	offset := 0
	totalPosts := 0

	for {
		// Get posts in batches
		postList, err := s.Platform.Store.Post().GetPosts(model.GetPostsOptions{
			ChannelId: channelID,
			Page:      offset / s.batchSize,
			PerPage:   s.batchSize,
		}, false, map[string]bool{})
		if err != nil {
			return model.NewAppError("SznSearch.ReindexChannel", "sznsearch.reindex.get_posts", nil, err.Error(), 500)
		}

		if len(postList.Posts) == 0 {
			break
		}

		// Prepare batch for indexing
		batch := make([]common.IndexedMessage, 0, len(postList.Posts))
		for _, post := range postList.Posts {
			msg, appErr := s.formatPostForIndex(post)
			if appErr != nil {
				rctx.Logger().Error("SznSearch: Failed to format post for reindex",
					mlog.String("post_id", post.Id),
					mlog.Err(appErr),
				)
				continue
			}
			batch = append(batch, *msg)
		}

		// Index the batch
		if len(batch) > 0 {
			if err := s.indexMessageBatch(batch); err != nil {
				rctx.Logger().Error("SznSearch: Failed to index batch during reindex",
					mlog.String("channel_id", channelID),
					mlog.Int("batch_size", len(batch)),
					mlog.Err(err),
				)
				return err
			}
			totalPosts += len(batch)
		}

		offset += s.batchSize

		// Break if we got less than batch size (no more posts)
		if len(postList.Posts) < s.batchSize {
			break
		}
	}

	rctx.Logger().Debug("SznSearch: Channel reindex completed",
		mlog.String("channel_id", channelID),
		mlog.Int("total_posts", totalPosts),
	)

	return nil
}
