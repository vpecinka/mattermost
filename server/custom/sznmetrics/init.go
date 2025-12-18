// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

import (
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/v8/channels/app/platform"
	"github.com/mattermost/mattermost/server/v8/einterfaces"
)

func init() {
	// Register the SznMetrics implementation
	platform.RegisterMetricsInterface(func(ps *platform.PlatformService, driver, dataSource string) einterfaces.MetricsInterface {
		return New(ps, driver, dataSource)
	})
	mlog.Info("SznMetrics: Metrics interface registered")
}
