package sznsearch

import (
	"github.com/mattermost/mattermost/server/v8/channels/app/platform"
	"github.com/mattermost/mattermost/server/v8/platform/services/searchengine"
)

func init() {
	// Register SznSearch engine with Mattermost platform
	platform.RegisterElasticsearchInterface(func(ps *platform.PlatformService) searchengine.SearchEngineInterface {
		return NewSznSearchEngine(ps)
	})
}
