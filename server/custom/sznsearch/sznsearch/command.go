// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sznsearch

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/i18n"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/channels/app"
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
		AutoCompleteHint: "[remove-index|full-reindex|team-reindex <team_id>|channel-reindex <channel_id>]",
		DisplayName:      "sznsearch",
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
		if len(parts) > 1 && parts[1] != "current" {
			teamID = parts[1]
		}
		return p.handleTeamReindex(a, rctx, args, teamID)
	case "channel-reindex":
		channelID := args.ChannelId // Default to current channel
		if len(parts) > 1 && parts[1] != "current" {
			channelID = parts[1]
		}
		return p.handleChannelReindex(a, rctx, args, channelID)
	default:
		return p.showHelp()
	}
}

func (p *SznSearchCommandProvider) showHelp() *model.CommandResponse {
	help := `### SznSearch Management Commands

Available commands:
- **/sznsearch remove-index** - Remove and recreate the search index (System Admin only)
- **/sznsearch full-reindex** - Reindex all posts from database (System Admin only)
- **/sznsearch team-reindex [team_id|current]** - Reindex all channels in a team (System Admin or Team Admin for their team)
- **/sznsearch channel-reindex [channel_id|current]** - Reindex a specific channel (Channel Admin or channel members for DMs/GMs)

**Note:** Omit team_id/channel_id or use "current" to use the current team/channel.`

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

	rctx.Logger().Info("SznSearch: User requested full reindex",
		mlog.String("user_id", args.UserId),
	)

	// Start async reindexing
	go func() {
		ctx := request.EmptyContext(rctx.Logger())
		if err := p.engine.FullReindexFromDatabase(ctx); err != nil {
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

	rctx.Logger().Info("SznSearch: User requested team reindex",
		mlog.String("user_id", args.UserId),
		mlog.String("team_id", teamID),
		mlog.String("team_name", team.Name),
	)

	// Start async reindexing
	go func() {
		ctx := request.EmptyContext(rctx.Logger())
		if err := p.engine.ReindexTeam(ctx, teamID); err != nil {
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

	rctx.Logger().Info("SznSearch: User requested channel reindex",
		mlog.String("user_id", args.UserId),
		mlog.String("channel_id", channelID),
		mlog.String("channel_type", string(channel.Type)),
	)

	// Start async reindexing
	go func() {
		ctx := request.EmptyContext(rctx.Logger())
		if err := p.engine.ReindexChannel(ctx, channelID); err != nil {
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
