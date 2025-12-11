// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package szncluster

import (
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/v8/channels/app/platform"
)

func init() {
	// Register the SznCluster implementation
	platform.RegisterClusterInterface(NewSznCluster)
	mlog.Info("SznCluster: Cluster interface registered")
}
