package sznsearch

import (
	"fmt"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/channels/store"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

const (
	reindexChannelLimit = 1000 // Number of channels to load per database query
)

// channelCacheItem represents minimal channel info for reindexing
type channelCacheItem struct {
	ID     string // channel ID
	TeamID string
	Type   model.ChannelType
}

// channelsCache holds all channels for efficient access and filtering
// Only contains non-ignored channels from non-ignored teams
type channelsCache struct {
	byTeam  map[string][]channelCacheItem // key: TeamID (or "" for DM/GM)
	allList []channelCacheItem            // flat list of all channels
}

// ReindexTeam reindexes all channels in a team
// Pass empty teamID ("") to reindex all DM/GM channels
func (s *SznSearchImpl) ReindexTeam(rctx request.CTX, teamID, userID string) *model.AppError {
	if !s.IsActive() {
		return model.NewAppError("SznSearch.ReindexTeam", "sznsearch.reindex.not_active", nil, "", 500)
	}

	// Try to start reindex - this will fail if another reindex is already running
	reindexInfo := &common.ReindexInfo{
		Type:      common.ReindexTypeTeam,
		TargetID:  teamID,
		UserID:    userID,
		StartedAt: time.Now().Unix(),
	}

	if err := s.startReindex(reindexInfo); err != nil {
		return err
	}
	defer s.stopReindex()

	// Build cache for single team reindex (slash command usage)
	cache, err := s.buildChannelsCache(rctx)
	if err != nil {
		return err
	}

	return s.reindexTeamWithCache(rctx, teamID, cache)
}

// ReindexChannel reindexes all posts in a channel
func (s *SznSearchImpl) ReindexChannel(rctx request.CTX, channelID, userID string) *model.AppError {
	if !s.IsActive() {
		return model.NewAppError("SznSearch.ReindexChannel", "sznsearch.reindex.not_active", nil, "", 500)
	}

	// Try to start reindex - this will fail if another reindex is already running
	reindexInfo := &common.ReindexInfo{
		Type:      common.ReindexTypeChannel,
		TargetID:  channelID,
		UserID:    userID,
		StartedAt: time.Now().Unix(),
	}

	if err := s.startReindex(reindexInfo); err != nil {
		return err
	}
	defer s.stopReindex()

	return s.reindexChannelInternal(rctx, channelID, 0)
}

// FullReindexFromDatabase performs a full reindex of all posts from the database
func (s *SznSearchImpl) FullReindexFromDatabase(rctx request.CTX, userID string) *model.AppError {
	if !s.IsActive() {
		return model.NewAppError("SznSearch.FullReindexFromDatabase", "sznsearch.reindex.not_active", nil, "", 500)
	}

	// Try to start reindex - this will fail if another reindex is already running
	reindexInfo := &common.ReindexInfo{
		Type:      common.ReindexTypeFull,
		TargetID:  "",
		UserID:    userID,
		StartedAt: time.Now().Unix(),
	}

	if err := s.startReindex(reindexInfo); err != nil {
		return err
	}
	defer s.stopReindex()

	rctx.Logger().Info("SznSearch: Starting full database reindex")

	// Build channels cache once for all teams (already filtered for ignored teams/channels)
	cache, err := s.buildChannelsCache(rctx)
	if err != nil {
		return err
	}

	// Reindex all channels using global worker pool (sinceTime = 0 means full reindex)
	errorCount := s.reindexChannelsParallel(rctx, cache.allList, 0)

	rctx.Logger().Info("SznSearch: Full reindex completed",
		mlog.Int("total_channels", len(cache.allList)),
		mlog.Int("errors", errorCount),
	)

	return nil
}

// DeltaReindexFromDatabase performs a delta reindex of posts created/updated within the last N days
func (s *SznSearchImpl) DeltaReindexFromDatabase(rctx request.CTX, userID string, days int) *model.AppError {
	if !s.IsActive() {
		return model.NewAppError("SznSearch.DeltaReindexFromDatabase", "sznsearch.reindex.not_active", nil, "", 500)
	}

	// Try to start reindex - this will fail if another reindex is already running
	reindexInfo := &common.ReindexInfo{
		Type:      common.ReindexTypeDelta,
		TargetID:  fmt.Sprintf("%d", days), // Store days as target ID for tracking
		UserID:    userID,
		StartedAt: time.Now().Unix(),
	}

	if err := s.startReindex(reindexInfo); err != nil {
		return err
	}
	defer s.stopReindex()

	rctx.Logger().Info("SznSearch: Starting delta database reindex", mlog.Int("days", days))

	// Calculate timestamp (days ago in milliseconds)
	sinceTime := model.GetMillis() - int64(days*24*60*60*1000)

	// Build channels cache once (already filtered for ignored teams/channels)
	cache, err := s.buildChannelsCache(rctx)
	if err != nil {
		return err
	}

	rctx.Logger().Info("SznSearch: Delta reindexing channels",
		mlog.Int("total_channels", len(cache.allList)),
		mlog.String("since_timestamp", fmt.Sprintf("%d", sinceTime)),
	)

	// Reindex channels with delta logic using parallel workers
	errorCount := s.reindexChannelsParallel(rctx, cache.allList, sinceTime)

	rctx.Logger().Info("SznSearch: Delta reindex completed",
		mlog.Int("total_channels", len(cache.allList)),
		mlog.Int("days", days),
		mlog.Int("errors", errorCount),
	)

	return nil
}

// reindexChannelsParallel reindexes multiple channels in parallel using a worker pool
// This is a shared function used by both full reindex and delta reindex
// If sinceTime is 0, performs full reindex. Otherwise, reindexes only posts since that timestamp.
func (s *SznSearchImpl) reindexChannelsParallel(rctx request.CTX, channels []channelCacheItem, sinceTime int64) int {

	channelCount := len(channels)
	if channelCount == 0 {
		rctx.Logger().Info("SznSearch: No channels to reindex")
		return 0
	}

	logMsg := "SznSearch: Processing channels with worker pool"
	logFields := []mlog.Field{
		mlog.Int("channel_count", channelCount),
		mlog.Int("pool_size", s.reindexPoolSize),
	}
	if sinceTime > 0 {
		logMsg = "SznSearch: Processing delta reindex with worker pool"
		logFields = append(logFields, mlog.String("since_time", fmt.Sprintf("%d", sinceTime)))
	}
	rctx.Logger().Info(logMsg, logFields...)

	// Create worker pool for parallel channel reindexing
	channelJobs := make(chan channelCacheItem, len(channels))
	errors := make(chan error, len(channels))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < s.reindexPoolSize; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for channel := range channelJobs {
				if err := s.reindexChannelInternal(rctx, channel.ID, sinceTime); err != nil {
					rctx.Logger().Error("SznSearch: Failed to reindex channel",
						mlog.String("channel_id", channel.ID),
						mlog.String("team_id", channel.TeamID),
						mlog.Int("worker_id", workerID),
						mlog.Err(err),
					)
					errors <- err
				}
			}
		}(i)
	}

	// Send channel jobs to workers
	for _, channel := range channels {
		channelJobs <- channel
	}
	close(channelJobs)

	// Wait for all workers to finish
	wg.Wait()
	close(errors)

	// Count errors
	errorCount := 0
	for range errors {
		errorCount++
	}

	return errorCount
}

// reindexTeamWithCache reindexes all channels in a team using provided cache
// Cache is already filtered for ignored teams/channels
func (s *SznSearchImpl) reindexTeamWithCache(rctx request.CTX, teamID string, cache *channelsCache) *model.AppError {
	if !s.IsActive() {
		return model.NewAppError("SznSearch.reindexTeamWithCache", "sznsearch.reindex.not_active", nil, "", 500)
	}

	if teamID == "" {
		rctx.Logger().Info("SznSearch: Starting direct/group messages reindex")
	} else {
		rctx.Logger().Info("SznSearch: Starting team reindex", mlog.String("team_id", teamID))
	}

	// Get channels for this team from cache (already filtered)
	channels, exists := cache.byTeam[teamID]
	if !exists || len(channels) == 0 {
		rctx.Logger().Info("SznSearch: No channels found for team", mlog.String("team_id", teamID))
		return nil
	}

	// Use shared parallel reindex function with pre-filtered channels (sinceTime = 0 means full reindex)
	errorCount := s.reindexChannelsParallel(rctx, channels, 0)

	if teamID == "" {
		rctx.Logger().Info("SznSearch: Direct/group messages reindex completed",
			mlog.Int("total_channels", len(channels)),
			mlog.Int("errors", errorCount),
		)
	} else {
		rctx.Logger().Info("SznSearch: Team reindex completed",
			mlog.String("team_id", teamID),
			mlog.Int("total_channels", len(channels)),
			mlog.Int("errors", errorCount),
		)
	}

	return nil
}

// reindexChannelInternal performs the actual channel reindex without state checking
// This is used internally by full/team/delta reindex operations
// If sinceTime is 0, reindexes all posts. Otherwise, reindexes only posts since that timestamp.
func (s *SznSearchImpl) reindexChannelInternal(rctx request.CTX, channelID string, sinceTime int64) *model.AppError {
	if sinceTime > 0 {
		rctx.Logger().Debug("SznSearch: Starting delta channel reindex",
			mlog.String("channel_id", channelID),
			mlog.String("since_time", fmt.Sprintf("%d", sinceTime)),
		)
	} else {
		rctx.Logger().Info("SznSearch: Reindexing channel", mlog.String("channel_id", channelID))
	}

	totalPosts := 0
	offset := 0

	// Unified pagination loop for both full and delta reindex
	for {
		var postList *model.PostList
		var err error

		// Choose method based on reindex type
		if sinceTime > 0 {
			// Delta reindex: use GetPostsSince (no pagination needed)
			postList, err = s.Platform.Store.Post().GetPostsSince(model.GetPostsSinceOptions{
				ChannelId: channelID,
				Time:      sinceTime,
			}, false, map[string]bool{})
		} else {
			// Full reindex: use paginated GetPosts
			postList, err = s.Platform.Store.Post().GetPosts(model.GetPostsOptions{
				ChannelId: channelID,
				Page:      offset / s.batchSize,
				PerPage:   s.batchSize,
			}, false, map[string]bool{})
		}

		if err != nil {
			errorCode := "sznsearch.reindex.get_posts"
			if sinceTime > 0 {
				errorCode = "sznsearch.reindex.get_posts_since"
			}
			return model.NewAppError("SznSearch.reindexChannelInternal", errorCode, nil, err.Error(), 500)
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

		// For delta reindex, GetPostsSince returns all matching posts at once (no pagination)
		if sinceTime > 0 {
			break
		}

		// For full reindex, check if we need to fetch next page
		offset += s.batchSize
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

// buildChannelsCache loads all active (non-deleted) channels into memory for efficient filtering
// Already filters out ignored teams and channels during construction
func (s *SznSearchImpl) buildChannelsCache(rctx request.CTX) (*channelsCache, *model.AppError) {
	cache := &channelsCache{
		byTeam:  make(map[string][]channelCacheItem),
		allList: make([]channelCacheItem, 0),
	}

	offset := 0
	totalChannels := 0
	skippedChannels := 0

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

		// Group channels by TeamID, skip ignored teams/channels
		for _, channel := range channels {
			teamID := channel.TeamId // Empty string for DM/GM

			// Skip ignored teams (but allow DM/GM with empty teamID)
			if teamID != "" && s.ignoreTeams[teamID] {
				skippedChannels++
				continue
			}

			// Skip ignored channels
			if s.ignoreChannels[channel.Id] {
				skippedChannels++
				continue
			}

			item := channelCacheItem{
				ID:     channel.Id,
				TeamID: teamID,
				Type:   channel.Type,
			}

			// Add to both structures
			cache.byTeam[teamID] = append(cache.byTeam[teamID], item)
			cache.allList = append(cache.allList, item)
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
		mlog.Int("skipped_channels", skippedChannels),
		mlog.Int("teams_count", len(cache.byTeam)),
	)

	return cache, nil
}
