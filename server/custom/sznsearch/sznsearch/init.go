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
	"github.com/mattermost/mattermost/server/v8/channels/app/platform"
	"github.com/mattermost/mattermost/server/v8/platform/services/searchengine"
)

func init() {
	// Register SznSearch engine with Mattermost platform (no license required)
	platform.RegisterSznSearchInterface(func(ps *platform.PlatformService) searchengine.SearchEngineInterface {
		return NewSznSearchEngine(ps)
	})
}
