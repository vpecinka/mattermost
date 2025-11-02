// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sznsearch

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/i18n"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/channels/app"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

const (
	CmdSznSearch = "sznsearch"
)

// SznSearchCommandProvider provides the /sznsearch slash command
type SznSearchCommandProvider struct {
	engine *SznSearchImpl
}

func init() {
	// Registration happens in Start() method when engine instance is created
}

// RegisterCommand registers the sznsearch command provider with the app
func (s *SznSearchImpl) RegisterCommand() {
	provider := &SznSearchCommandProvider{
		engine: s,
	}
	app.RegisterCommandProvider(provider)
	s.Platform.Log().Info("SznSearch: Registered /sznsearch command")
}

func (*SznSearchCommandProvider) GetTrigger() string {
	return CmdSznSearch
}

func (p *SznSearchCommandProvider) GetCommand(a *app.App, T i18n.TranslateFunc) *model.Command {
	return &model.Command{
		Trigger:          CmdSznSearch,
		AutoComplete:     true,
		AutoCompleteDesc: "Manage SznSearch indexing",
		// AutoCompleteHint: "[remove-index|full-reindex|team-reindex <team_id>|channel-reindex <channel_id>|delta-reindex <days>]",
		DisplayName: "sznsearch",
	}
}

func (p *SznSearchCommandProvider) DoCommand(a *app.App, rctx request.CTX, args *model.CommandArgs, message string) *model.CommandResponse {
	parts := strings.Fields(message)
	if len(parts) == 0 {
		return p.showHelp()
	}

	subcommand := parts[0]

	switch subcommand {
	case "remove-index":
		return p.handleRemoveIndex(a, rctx, args)
	case "full-reindex":
		return p.handleFullReindex(a, rctx, args)
	case "team-reindex":
		teamID := args.TeamId // Default to current team
		if len(parts) > 1 {
			teamID = parts[1]
		}
		return p.handleTeamReindex(a, rctx, args, teamID)
	case "channel-reindex":
		channelID := args.ChannelId // Default to current channel
		if len(parts) > 1 {
			channelID = parts[1]
		}
		return p.handleChannelReindex(a, rctx, args, channelID)
	case "delta-reindex":
		if len(parts) < 2 {
			return &model.CommandResponse{
				Text:         "Error: Please specify number of days (e.g., `/sznsearch delta-reindex 7`)",
				ResponseType: model.CommandResponseTypeEphemeral,
			}
		}
		return p.handleDeltaReindex(a, rctx, args, parts[1])
	default:
		return p.showHelp()
	}
}

// formatReindexError formats an error message about a running reindex operation
func (p *SznSearchCommandProvider) formatReindexError(a *app.App, info *common.ReindexInfo) string {
	var reindexType string
	var target string

	switch info.Type {
	case common.ReindexTypeFull:
		reindexType = "Full reindex"
		target = "all data"
	case common.ReindexTypeDelta:
		reindexType = "Delta reindex"
		target = fmt.Sprintf("posts from the last **%s days**", info.TargetID)
	case common.ReindexTypeTeam:
		reindexType = "Team reindex"
		if team, err := p.engine.Platform.Store.Team().Get(info.TargetID); err == nil {
			target = fmt.Sprintf("team **%s**", team.DisplayName)
		} else {
			target = fmt.Sprintf("team `%s`", info.TargetID)
		}
	case common.ReindexTypeChannel:
		reindexType = "Channel reindex"
		if channel, err := p.engine.Platform.Store.Channel().Get(info.TargetID, true); err == nil {
			channelName := channel.DisplayName
			if channelName == "" {
				channelName = channel.Name
			}
			target = fmt.Sprintf("channel **%s**", channelName)
		} else {
			target = fmt.Sprintf("channel `%s`", info.TargetID)
		}
	default:
		reindexType = "Reindex"
		target = "unknown target"
	}

	// Get user info
	userName := "Unknown user"
	if user, err := a.GetUser(info.UserID); err == nil {
		userName = fmt.Sprintf("@%s", user.Username)
	}

	// Format timestamp
	startTime := time.Unix(info.StartedAt, 0)
	timeStr := startTime.Format("02.01.2006 15:04")

	return fmt.Sprintf(
		"**Error:** %s is already running for %s.\n\nStarted by %s on %s.\n\nPlease wait for it to complete before starting another reindex operation.",
		reindexType,
		target,
		userName,
		timeStr,
	)
}

func (p *SznSearchCommandProvider) showHelp() *model.CommandResponse {
	help := `### SznSearch Management Commands

Available commands:
- **/sznsearch remove-index** - Remove and recreate the search index (System Admin only)
- **/sznsearch full-reindex** - Reindex all posts from database (System Admin only)
- **/sznsearch team-reindex [team_id]** - Reindex all channels in a team (System Admin or Team Admin for their team)
- **/sznsearch channel-reindex [channel_id]** - Reindex a specific channel (Channel Admin or channel members for DMs/GMs)
- **/sznsearch delta-reindex <days>** - Reindex posts from the last N days across all channels (System Admin only)

**Note:** Omit team_id/channel_id to use the current team/channel.`

	return &model.CommandResponse{
		Text:         help,
		ResponseType: model.CommandResponseTypeEphemeral,
	}
}

func (p *SznSearchCommandProvider) handleRemoveIndex(a *app.App, rctx request.CTX, args *model.CommandArgs) *model.CommandResponse {
	// Only system admins can remove index
	if !a.HasPermissionTo(args.UserId, model.PermissionManageSystem) {
		return &model.CommandResponse{
			Text:         "Error: Only System Administrators can remove the search index.",
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	rctx.Logger().Info("SznSearch: User requested index removal",
		mlog.String("user_id", args.UserId),
		mlog.String("team_id", args.TeamId),
	)

	// Call PurgeIndexes to remove and recreate indices
	if err := p.engine.PurgeIndexes(rctx); err != nil {
		return &model.CommandResponse{
			Text:         fmt.Sprintf("Error removing index: %s", err.Error()),
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	return &model.CommandResponse{
		Text:         "Search index removed and recreated successfully. New posts will be indexed automatically.",
		ResponseType: model.CommandResponseTypeEphemeral,
	}
}

func (p *SznSearchCommandProvider) handleFullReindex(a *app.App, rctx request.CTX, args *model.CommandArgs) *model.CommandResponse {
	// Only system admins can do full reindex
	if !a.HasPermissionTo(args.UserId, model.PermissionManageSystem) {
		return &model.CommandResponse{
			Text:         "Error: Only System Administrators can perform a full reindex.",
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	// Check if a reindex is already running
	if runningInfo := p.engine.getRunningReindex(); runningInfo != nil {
		return &model.CommandResponse{
			Text:         p.formatReindexError(a, runningInfo),
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	rctx.Logger().Info("SznSearch: User requested full reindex",
		mlog.String("user_id", args.UserId),
	)

	// Start async reindexing
	userID := args.UserId
	go func() {
		ctx := request.EmptyContext(rctx.Logger())
		if err := p.engine.FullReindexFromDatabase(ctx, userID); err != nil {
			rctx.Logger().Error("SznSearch: Full reindex failed", mlog.Err(err))
		} else {
			rctx.Logger().Info("SznSearch: Full reindex completed successfully")
		}
	}()

	return &model.CommandResponse{
		Text:         "Full reindex started in the background. Check server logs for progress.",
		ResponseType: model.CommandResponseTypeEphemeral,
	}
}

func (p *SznSearchCommandProvider) handleTeamReindex(a *app.App, rctx request.CTX, args *model.CommandArgs, teamID string) *model.CommandResponse {
	if teamID == "" {
		return &model.CommandResponse{
			Text:         "Error: No team specified and no current team context.",
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	// Check permissions: System admin can reindex any team, Team admin can reindex their own team
	isSystemAdmin := a.HasPermissionTo(args.UserId, model.PermissionManageSystem)
	isTeamAdmin := a.HasPermissionToTeam(rctx, args.UserId, teamID, model.PermissionManageTeam)

	if !isSystemAdmin && !isTeamAdmin {
		return &model.CommandResponse{
			Text:         "Error: You must be a System Administrator or Team Administrator to reindex a team.",
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	// Get team to verify it exists and for logging
	team, err := p.engine.Platform.Store.Team().Get(teamID)
	if err != nil {
		return &model.CommandResponse{
			Text:         fmt.Sprintf("Error: Team not found: %s", err.Error()),
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	// Check if a reindex is already running
	if runningInfo := p.engine.getRunningReindex(); runningInfo != nil {
		return &model.CommandResponse{
			Text:         p.formatReindexError(a, runningInfo),
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	rctx.Logger().Info("SznSearch: User requested team reindex",
		mlog.String("user_id", args.UserId),
		mlog.String("team_id", teamID),
		mlog.String("team_name", team.Name),
	)

	// Start async reindexing
	userID := args.UserId
	go func() {
		ctx := request.EmptyContext(rctx.Logger())
		if err := p.engine.ReindexTeam(ctx, teamID, userID); err != nil {
			rctx.Logger().Error("SznSearch: Team reindex failed",
				mlog.String("team_id", teamID),
				mlog.Err(err),
			)
		} else {
			rctx.Logger().Info("SznSearch: Team reindex completed successfully",
				mlog.String("team_id", teamID),
			)
		}
	}()

	return &model.CommandResponse{
		Text:         fmt.Sprintf("Reindexing team **%s** in the background. Check server logs for progress.", team.DisplayName),
		ResponseType: model.CommandResponseTypeEphemeral,
	}
}

func (p *SznSearchCommandProvider) handleChannelReindex(a *app.App, rctx request.CTX, args *model.CommandArgs, channelID string) *model.CommandResponse {
	if channelID == "" {
		return &model.CommandResponse{
			Text:         "Error: No channel specified and no current channel context.",
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	// Get channel to check type and permissions
	channel, err := p.engine.Platform.Store.Channel().Get(channelID, true)
	if err != nil {
		return &model.CommandResponse{
			Text:         fmt.Sprintf("Error: Channel not found: %s", err.Error()),
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	// Check permissions based on channel type
	isSystemAdmin := a.HasPermissionTo(args.UserId, model.PermissionManageSystem)
	canReindex := isSystemAdmin

	switch channel.Type {
	case model.ChannelTypeOpen:
		// Public channel: need channel admin permission
		canReindex = canReindex || a.HasPermissionToChannel(rctx, args.UserId, channelID, model.PermissionManagePublicChannelProperties)
	case model.ChannelTypePrivate:
		// Private channel: need channel admin permission
		canReindex = canReindex || a.HasPermissionToChannel(rctx, args.UserId, channelID, model.PermissionManagePrivateChannelProperties)
	case model.ChannelTypeDirect, model.ChannelTypeGroup:
		// DM/GM: check if user is a member
		_, memberErr := p.engine.Platform.Store.Channel().GetMember(rctx.Context(), channelID, args.UserId)
		canReindex = canReindex || memberErr == nil
	default:
		return &model.CommandResponse{
			Text:         "Error: Unsupported channel type.",
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	if !canReindex {
		return &model.CommandResponse{
			Text:         "Error: You don't have permission to reindex this channel.",
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	// Check if a reindex is already running
	if runningInfo := p.engine.getRunningReindex(); runningInfo != nil {
		return &model.CommandResponse{
			Text:         p.formatReindexError(a, runningInfo),
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	rctx.Logger().Info("SznSearch: User requested channel reindex",
		mlog.String("user_id", args.UserId),
		mlog.String("channel_id", channelID),
		mlog.String("channel_type", string(channel.Type)),
	)

	// Start async reindexing
	userID := args.UserId
	go func() {
		ctx := request.EmptyContext(rctx.Logger())
		if err := p.engine.ReindexChannel(ctx, channelID, userID); err != nil {
			rctx.Logger().Error("SznSearch: Channel reindex failed",
				mlog.String("channel_id", channelID),
				mlog.Err(err),
			)
		} else {
			rctx.Logger().Info("SznSearch: Channel reindex completed successfully",
				mlog.String("channel_id", channelID),
			)
		}
	}()

	channelName := channel.DisplayName
	if channelName == "" {
		channelName = channel.Name
	}

	return &model.CommandResponse{
		Text:         fmt.Sprintf("Reindexing channel **%s** in the background. Check server logs for progress.", channelName),
		ResponseType: model.CommandResponseTypeEphemeral,
	}
}

func (p *SznSearchCommandProvider) handleDeltaReindex(a *app.App, rctx request.CTX, args *model.CommandArgs, daysStr string) *model.CommandResponse {
	// Only system admins can do delta reindex
	if !a.HasPermissionTo(args.UserId, model.PermissionManageSystem) {
		return &model.CommandResponse{
			Text:         "Error: Only System Administrators can perform a delta reindex.",
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	// Parse days parameter
	days, err := fmt.Sscanf(daysStr, "%d", new(int))
	if err != nil || days <= 0 {
		return &model.CommandResponse{
			Text:         "Error: Invalid number of days. Please provide a positive integer (e.g., `/sznsearch delta-reindex 7`)",
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	var daysInt int
	fmt.Sscanf(daysStr, "%d", &daysInt)

	// Check if a reindex is already running
	if runningInfo := p.engine.getRunningReindex(); runningInfo != nil {
		return &model.CommandResponse{
			Text:         p.formatReindexError(a, runningInfo),
			ResponseType: model.CommandResponseTypeEphemeral,
		}
	}

	rctx.Logger().Info("SznSearch: User requested delta reindex",
		mlog.String("user_id", args.UserId),
		mlog.Int("days", daysInt),
	)

	// Start async reindexing
	userID := args.UserId
	go func() {
		ctx := request.EmptyContext(rctx.Logger())
		if err := p.engine.DeltaReindexFromDatabase(ctx, userID, daysInt); err != nil {
			rctx.Logger().Error("SznSearch: Delta reindex failed",
				mlog.Int("days", daysInt),
				mlog.Err(err),
			)
		} else {
			rctx.Logger().Info("SznSearch: Delta reindex completed successfully",
				mlog.Int("days", daysInt),
			)
		}
	}()

	return &model.CommandResponse{
		Text:         fmt.Sprintf("Delta reindex started for posts from the last **%d days** in the background. Check server logs for progress.", daysInt),
		ResponseType: model.CommandResponseTypeEphemeral,
	}
}
