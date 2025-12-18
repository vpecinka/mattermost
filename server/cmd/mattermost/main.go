// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"os"

	"github.com/mattermost/mattermost/server/v8/cmd/mattermost/commands"
	// Import and register app layer slash commands
	_ "github.com/mattermost/mattermost/server/v8/channels/app/slashcommands"
	// OAuth Providers
	_ "github.com/mattermost/mattermost/server/v8/channels/app/oauthproviders/openid"

	// Enterprise Imports
	// _ "github.com/mattermost/mattermost/server/v8/enterprise"

	// Custom SznCluster - Provides cluster support without enterprise
	_ "github.com/mattermost/mattermost/server/v8/custom/szncluster"

	// Custom SznSearch Engine - MUST be after enterprise to override elasticsearch
	_ "github.com/mattermost/mattermost/server/v8/custom/sznsearch/sznsearch"

	// Custom SznMetrics - Provides Prometheus metrics support without enterprise license
	_ "github.com/mattermost/mattermost/server/v8/custom/sznmetrics"
)

func main() {
	if err := commands.Run(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
