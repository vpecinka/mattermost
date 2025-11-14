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

	return s.reindexChannelInternal(rctx, channelID, 0, false)
}

// FullReindexFromDatabase performs a full reindex of all posts from the database
func (s *SznSearchImpl) FullReindexFromDatabase(rctx request.CTX, userID string, shouldRecreateIndex bool) *model.AppError {
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

	rctx.Logger().Info("SznSearch: Starting full database reindex",
		mlog.Bool("recreate_index", shouldRecreateIndex),
	)

	// Optionally purge and recreate indices if requested
	if shouldRecreateIndex {
		rctx.Logger().Info("SznSearch: Purging and recreating indices")
		if err := s.PurgeIndexes(rctx); err != nil {
			rctx.Logger().Error("SznSearch: Failed to purge indices", mlog.Err(err))
			return err
		}
		rctx.Logger().Info("SznSearch: Indices recreated successfully")
	}

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
				if err := s.reindexChannelInternal(rctx, channel.ID, sinceTime, false); err != nil {
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

// indexChannelMetadata indexes channel metadata for autocomplete
func (s *SznSearchImpl) indexChannelMetadata(rctx request.CTX, channelID string) *model.AppError {
	// Get channel from database
	channel, err := s.Platform.Store.Channel().Get(channelID, true)
	if err != nil {
		return model.NewAppError("SznSearch.indexChannelMetadata", "sznsearch.index_channel_metadata.get_channel", nil, err.Error(), 500)
	}

	// Get user IDs for private channels
	var userIDs []string
	if channel.Type == model.ChannelTypePrivate {
		members, membersErr := s.Platform.Store.Channel().GetMembers(model.ChannelMembersGetOptions{
			ChannelID: channelID,
			Offset:    0,
			Limit:     10000,
		})
		if membersErr != nil {
			rctx.Logger().Error("SznSearch: Failed to get channel members for metadata indexing",
				mlog.String("channel_id", channelID),
				mlog.Err(membersErr),
			)
			return model.NewAppError("SznSearch.indexChannelMetadata", "sznsearch.index_channel_metadata.get_members", nil, membersErr.Error(), 500)
		}
		userIDs = make([]string, 0, len(members))
		for _, member := range members {
			userIDs = append(userIDs, member.UserId)
		}
	}

	// Get team member IDs if channel has a team
	var teamMemberIDs []string
	if channel.TeamId != "" {
		teamMembers, teamErr := s.Platform.Store.Team().GetMembers(channel.TeamId, 0, 100000, nil)
		if teamErr != nil {
			rctx.Logger().Error("SznSearch: Failed to get team members for channel metadata indexing",
				mlog.String("channel_id", channelID),
				mlog.String("team_id", channel.TeamId),
				mlog.Err(teamErr),
			)
			// Not critical, continue without team member IDs
		} else {
			teamMemberIDs = make([]string, 0, len(teamMembers))
			for _, member := range teamMembers {
				teamMemberIDs = append(teamMemberIDs, member.UserId)
			}
		}
	}

	// Index through the public interface (which has all the retry logic and error handling)
	return s.IndexChannel(rctx, channel, userIDs, teamMemberIDs)
}

// reindexChannelInternal performs the actual channel reindex without state checking
// This is used internally by full/team/delta reindex operations
// If sinceTime is 0, reindexes all posts. Otherwise, reindexes only posts since that timestamp.
// If metadataOnly is true, only indexes channel metadata (not posts) for autocomplete.
func (s *SznSearchImpl) reindexChannelInternal(rctx request.CTX, channelID string, sinceTime int64, metadataOnly bool) *model.AppError {
	// If metadata-only mode, just index the channel document and return
	if metadataOnly {
		return s.indexChannelMetadata(rctx, channelID)
	}
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
	const maxPerPage = 1000 // Mattermost API limit for GetPosts

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
				Page:      offset / maxPerPage,
				PerPage:   maxPerPage,
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
			// Skip deleted posts (DeleteAt > 0)
			// Note: Skipping posts here doesn't affect pagination logic - we still process
			// all pages based on postList.Posts count from DB, not indexed count
			if post.DeleteAt > 0 {
				continue
			}

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
		// Pagination is based on posts returned from DB (postList.Posts), not posts actually indexed
		// This ensures we traverse all DB pages correctly even if we skip some deleted posts
		offset += maxPerPage
		if len(postList.Posts) < maxPerPage {
			break // Last page - DB returned fewer posts than requested
		}
	}

	rctx.Logger().Debug("SznSearch: Channel reindex completed",
		mlog.String("channel_id", channelID),
		mlog.Int("total_posts", totalPosts),
	)

	return nil
}

// buildChannelsCache loads all active (non-deleted) channels into memory for efficient filtering
// buildChannelsCache is a wrapper that uses current configuration for ignored channels/teams
func (s *SznSearchImpl) buildChannelsCache(rctx request.CTX) (*channelsCache, *model.AppError) {
	return s.buildChannelsCacheWithFilter(rctx, s.ignoreChannels, s.ignoreTeams, true)
}

// buildChannelsCacheWithFilter builds a cache of all channels, filtering by provided ignored channels/teams
// includeDMGM controls whether to include DM/GM channels in the cache
func (s *SznSearchImpl) buildChannelsCacheWithFilter(rctx request.CTX, ignoreChannelIDs, ignoreTeamIDs map[string]bool, includeDMGM bool) (*channelsCache, *model.AppError) {
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
			return nil, model.NewAppError("SznSearch.buildChannelsCacheWithFilter", "sznsearch.reindex.get_channels", nil, err.Error(), 500)
		}

		if len(channels) == 0 {
			break
		}

		// Group channels by TeamID, skip ignored teams/channels
		for _, channel := range channels {
			teamID := channel.TeamId // Empty string for DM/GM

			// Skip ignored teams (but allow DM/GM with empty teamID)
			if teamID != "" && ignoreTeamIDs[teamID] {
				skippedChannels++
				continue
			}

			// Skip ignored channels
			if ignoreChannelIDs[channel.Id] {
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

	// 2. Load DM/GM channels separately iterating over all users (only if requested)
	dmChannels := 0
	if includeDMGM {
		users, err := s.Platform.Store.User().GetAll()
		if err != nil {
			return nil, model.NewAppError("SznSearch.buildChannelsCacheWithFilter", "sznsearch.reindex.get_users", nil, err.Error(), 500)
		}

		seenChannels := make(map[string]bool)
		for _, user := range users {
			// skip bot users
			if user.IsBot {
				rctx.Logger().Debug("SznSearch: Skipping bot user for DM/GM channels", mlog.String("user_id", user.Id), mlog.String("username", user.Username))
				continue
			}
			// Get DM/GM channels for this user
			userChannels, err := s.Platform.Store.Channel().GetChannelsByUser(user.Id, false, 0, -1, "")
			if err != nil {
				// log this error and continue with other users
				rctx.Logger().Warn("SznSearch: Failed to get user channels", mlog.String("user_id", user.Id), mlog.Err(err))
				continue
			}

			for _, channel := range userChannels {
				// Skip already seen channels (to avoid duplicates)
				if seenChannels[channel.Id] {
					continue
				}
				// skip non-DM/GM channels
				if channel.Type != model.ChannelTypeDirect && channel.Type != model.ChannelTypeGroup {
					continue
				}
				// Skip ignored channels
				if ignoreChannelIDs[channel.Id] {
					skippedChannels++
					continue
				}

				dmChannels++
				seenChannels[channel.Id] = true
				item := channelCacheItem{
					ID:     channel.Id,
					TeamID: "", // DM/GM channels have no TeamID
					Type:   channel.Type,
				}

				// Add to both structures
				cache.byTeam[""] = append(cache.byTeam[""], item)
				cache.allList = append(cache.allList, item)
				totalChannels++
			}
		}
	}

	rctx.Logger().Info("SznSearch: Channels cache built",
		mlog.Int("total_channels", totalChannels),
		mlog.Int("skipped_channels", skippedChannels),
		mlog.Int("dm_channels", dmChannels),
		mlog.Int("teams_count", len(cache.byTeam)),
	)

	return cache, nil
}

// ReindexUser reindexes a specific user or all users
func (s *SznSearchImpl) ReindexUser(rctx request.CTX, username string) *model.AppError {
	if !s.IsActive() {
		return model.NewAppError("SznSearch.ReindexUser", "sznsearch.reindex_user.not_active", nil, "", 500)
	}

	if username != "" {
		// Reindex specific user
		rctx.Logger().Info("SznSearch: Starting user reindex",
			mlog.String("username", username),
		)
		return s.reindexSingleUser(rctx, username)
	}

	// Reindex all users
	rctx.Logger().Info("SznSearch: Starting full user reindex")
	return s.reindexAllUsers(rctx)
}

// reindexSingleUser reindexes a single user by username
func (s *SznSearchImpl) reindexSingleUser(rctx request.CTX, username string) *model.AppError {
	// Get user by username
	user, err := s.Platform.Store.User().GetByUsername(username)
	if err != nil {
		return model.NewAppError("SznSearch.reindexSingleUser", "sznsearch.reindex_user.get_user", nil, err.Error(), 404)
	}

	// Get user's teams
	teamMembers, err := s.Platform.Store.Team().GetTeamsForUser(rctx, user.Id, "", true)
	if err != nil {
		rctx.Logger().Error("SznSearch: Failed to get teams for user",
			mlog.String("user_id", user.Id),
			mlog.Err(err),
		)
		return model.NewAppError("SznSearch.reindexSingleUser", "sznsearch.reindex_user.get_teams", nil, err.Error(), 500)
	}

	teamIds := make([]string, 0, len(teamMembers))
	for _, tm := range teamMembers {
		teamIds = append(teamIds, tm.TeamId)
	}

	// Get user's channels
	channelMembers, err := s.Platform.Store.Channel().GetAllChannelMembersForUser(rctx, user.Id, false, true)
	if err != nil {
		rctx.Logger().Error("SznSearch: Failed to get channels for user",
			mlog.String("user_id", user.Id),
			mlog.Err(err),
		)
		return model.NewAppError("SznSearch.reindexSingleUser", "sznsearch.reindex_user.get_channels", nil, err.Error(), 500)
	}

	channelIds := make([]string, 0, len(channelMembers))
	for channelId := range channelMembers {
		channelIds = append(channelIds, channelId)
	}

	// Index the user
	if indexErr := s.IndexUser(rctx, user, teamIds, channelIds); indexErr != nil {
		return indexErr
	}

	rctx.Logger().Info("SznSearch: User reindexed successfully",
		mlog.String("user_id", user.Id),
		mlog.String("username", user.Username),
		mlog.Int("teams_count", len(teamIds)),
		mlog.Int("channels_count", len(channelIds)),
	)

	return nil
}

// reindexAllUsers reindexes all users in the system
func (s *SznSearchImpl) reindexAllUsers(rctx request.CTX) *model.AppError {
	const batchSize = 100
	page := 0
	totalIndexed := 0
	totalErrors := 0

	rctx.Logger().Info("SznSearch: Starting batch user indexing",
		mlog.Int("batch_size", batchSize),
	)

	for {
		// Get batch of users
		users, err := s.Platform.Store.User().GetAllProfiles(&model.UserGetOptions{
			Page:    page,
			PerPage: batchSize,
		})
		if err != nil {
			rctx.Logger().Error("SznSearch: Failed to get users batch",
				mlog.Int("page", page),
				mlog.Err(err),
			)
			return model.NewAppError("SznSearch.reindexAllUsers", "sznsearch.reindex_all_users.get_users", nil, err.Error(), 500)
		}

		if len(users) == 0 {
			break
		}

		// Index each user in the batch
		for _, user := range users {
			// Skip deleted users
			if user.DeleteAt != 0 {
				continue
			}

			// Get user's teams
			teamMembers, teamErr := s.Platform.Store.Team().GetTeamsForUser(rctx, user.Id, "", true)
			if teamErr != nil {
				rctx.Logger().Error("SznSearch: Failed to get teams for user during batch reindex",
					mlog.String("user_id", user.Id),
					mlog.Err(teamErr),
				)
				totalErrors++
				continue
			}

			teamIds := make([]string, 0, len(teamMembers))
			for _, tm := range teamMembers {
				teamIds = append(teamIds, tm.TeamId)
			}

			// Get user's channels
			channelMembers, chanErr := s.Platform.Store.Channel().GetAllChannelMembersForUser(rctx, user.Id, false, true)
			if chanErr != nil {
				rctx.Logger().Error("SznSearch: Failed to get channels for user during batch reindex",
					mlog.String("user_id", user.Id),
					mlog.Err(chanErr),
				)
				totalErrors++
				continue
			}

			channelIds := make([]string, 0, len(channelMembers))
			for channelId := range channelMembers {
				channelIds = append(channelIds, channelId)
			}

			// Index the user
			if indexErr := s.IndexUser(rctx, user, teamIds, channelIds); indexErr != nil {
				rctx.Logger().Error("SznSearch: Failed to index user during batch reindex",
					mlog.String("user_id", user.Id),
					mlog.String("username", user.Username),
					mlog.Err(indexErr),
				)
				totalErrors++
				continue
			}

			totalIndexed++
		}

		rctx.Logger().Info("SznSearch: Batch user indexing progress",
			mlog.Int("page", page),
			mlog.Int("batch_size", len(users)),
			mlog.Int("total_indexed", totalIndexed),
			mlog.Int("total_errors", totalErrors),
		)

		// Move to next page
		page++

		// Small delay to avoid overwhelming the system
		time.Sleep(100 * time.Millisecond)
	}

	rctx.Logger().Info("SznSearch: Batch user indexing completed",
		mlog.Int("total_indexed", totalIndexed),
		mlog.Int("total_errors", totalErrors),
	)

	if totalErrors > 0 {
		return model.NewAppError("SznSearch.reindexAllUsers", "sznsearch.reindex_all_users.partial_failure",
			map[string]any{"TotalIndexed": totalIndexed, "TotalErrors": totalErrors}, "", 500)
	}

	return nil
}

// ReindexMetadata reindexes metadata for all channels (without posts)
// In the future, this can be extended to reindex other metadata like users
func (s *SznSearchImpl) ReindexMetadata(rctx request.CTX) *model.AppError {
	if !s.IsActive() {
		return model.NewAppError("SznSearch.ReindexMetadata", "sznsearch.reindex.not_active", nil, "", 500)
	}

	rctx.Logger().Info("SznSearch: Starting metadata reindexing")

	// Build channels cache WITHOUT filtering - we need all channels for autocomplete
	// Exclude DM/GM channels - they are not used in channel autocomplete
	emptyIgnoreChannels := make(map[string]bool)
	emptyIgnoreTeams := make(map[string]bool)
	cache, err := s.buildChannelsCacheWithFilter(rctx, emptyIgnoreChannels, emptyIgnoreTeams, false)
	if err != nil {
		return model.NewAppError("SznSearch.ReindexMetadata", "sznsearch.reindex_metadata.build_cache", nil, err.Error(), 500)
	}

	totalIndexed := 0
	totalErrors := 0

	// Iterate through all channels in cache (DM/GM already excluded by includeDMGM=false)
	for _, channelItem := range cache.allList {
		// Index channel metadata
		if indexErr := s.indexChannelMetadata(rctx, channelItem.ID); indexErr != nil {
			rctx.Logger().Error("SznSearch: Failed to index channel metadata during reindex",
				mlog.String("channel_id", channelItem.ID),
				mlog.Err(indexErr),
			)
			totalErrors++
			continue
		}

		totalIndexed++

		// Log progress every 100 channels
		if totalIndexed%100 == 0 {
			rctx.Logger().Info("SznSearch: Metadata reindex progress",
				mlog.Int("total_indexed", totalIndexed),
				mlog.Int("total_errors", totalErrors),
			)
		}
	}

	rctx.Logger().Info("SznSearch: Metadata reindexing completed",
		mlog.Int("total_indexed", totalIndexed),
		mlog.Int("total_errors", totalErrors),
	)

	if totalErrors > 0 {
		return model.NewAppError("SznSearch.ReindexMetadata", "sznsearch.reindex_metadata.partial_failure",
			map[string]any{"TotalIndexed": totalIndexed, "TotalErrors": totalErrors}, "", 500)
	}

	return nil
}
