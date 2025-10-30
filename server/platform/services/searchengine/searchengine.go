// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package searchengine

import (
	"github.com/mattermost/mattermost/server/public/model"
)

func NewBroker(cfg *model.Config) *Broker {
	return &Broker{
		cfg: cfg,
	}
}

func (seb *Broker) RegisterElasticsearchEngine(es SearchEngineInterface) {
	seb.ElasticsearchEngine = es
}

func (seb *Broker) RegisterSznSearchEngine(szn SearchEngineInterface) {
	seb.SznSearchEngine = szn
}

type Broker struct {
	cfg                 *model.Config
	ElasticsearchEngine SearchEngineInterface
	SznSearchEngine     SearchEngineInterface
}

func (seb *Broker) UpdateConfig(cfg *model.Config) *model.AppError {
	seb.cfg = cfg
	if seb.ElasticsearchEngine != nil {
		seb.ElasticsearchEngine.UpdateConfig(cfg)
	}
	if seb.SznSearchEngine != nil {
		seb.SznSearchEngine.UpdateConfig(cfg)
	}

	return nil
}

func (seb *Broker) GetActiveEngines() []SearchEngineInterface {
	engines := []SearchEngineInterface{}

	// Prioritize SznSearchEngine over ElasticsearchEngine
	if seb.SznSearchEngine != nil && seb.SznSearchEngine.IsActive() {
		engines = append(engines, seb.SznSearchEngine)
	} else if seb.ElasticsearchEngine != nil && seb.ElasticsearchEngine.IsActive() {
		engines = append(engines, seb.ElasticsearchEngine)
	}

	return engines
}

func (seb *Broker) ActiveEngine() string {
	activeEngines := seb.GetActiveEngines()
	if len(activeEngines) > 0 {
		return activeEngines[0].GetName()
	}
	if *seb.cfg.SqlSettings.DisableDatabaseSearch {
		return "none"
	}
	return "database"
}
