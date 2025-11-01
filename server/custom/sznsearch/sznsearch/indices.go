package sznsearch

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

// Index settings for Czech language with Hunspell analyzer
var messageIndexSettings = map[string]any{
	"settings": map[string]any{
		"index": map[string]any{
			"number_of_replicas": 2,
			"number_of_shards":   3,
			"analysis": map[string]any{
				"analyzer": map[string]any{
					"czech": map[string]any{
						"type":      "custom",
						"tokenizer": "standard",
						"filter": []string{
							"czech_stop",
							"czech_hunspell",
							"lowercase",
							"czech_stop",
							"icu_folding",
							"unique_on_same_position",
						},
					},
				},
				"filter": map[string]any{
					"czech_hunspell": map[string]any{
						"type":   "hunspell",
						"locale": "cs_CZ",
					},
					"czech_stop": map[string]any{
						"type":      "stop",
						"stopwords": []string{"že", "_czech_"},
					},
					"unique_on_same_position": map[string]any{
						"type":                  "unique",
						"only_on_same_position": true,
					},
				},
			},
		},
	},
	"mappings": map[string]any{
		"properties": map[string]any{
			"Message": map[string]any{
				"type":     "text",
				"analyzer": "czech",
			},
			"Payload": map[string]any{
				"type":     "text",
				"analyzer": "czech",
			},
			"Hashtags": map[string]any{
				"type": "keyword",
				"fields": map[string]any{
					"text": map[string]any{
						"type":     "text",
						"analyzer": "standard",
					},
				},
			},
			"CreatedAt": map[string]any{
				"type": "long",
			},
			"ChannelId": map[string]any{
				"type": "keyword",
			},
			"ChannelType": map[string]any{
				"type": "byte",
			},
			"TeamId": map[string]any{
				"type": "keyword",
			},
			"UserId": map[string]any{
				"type": "keyword",
			},
			"Members": map[string]any{
				"type": "keyword",
			},
		},
	},
}

var stateIndexSettings = map[string]any{
	"settings": map[string]any{
		"index": map[string]any{
			"number_of_replicas": 2,
			"number_of_shards":   3,
		},
	},
}

// ensureIndices creates ElasticSearch indices if they don't exist
func (s *SznSearchImpl) ensureIndices() *model.AppError {
	s.Platform.Log().Info("SznSearch: Checking if indices exist")

	// Check if message index exists
	res, err := s.client.Indices.Exists([]string{common.MessageIndex})
	if err != nil {
		return model.NewAppError("SznSearch.ensureIndices", "sznsearch.ensure_indices.check_error", nil, err.Error(), http.StatusInternalServerError)
	}
	res.Body.Close()

	// Create message index if it doesn't exist
	if res.StatusCode == http.StatusNotFound {
		s.Platform.Log().Info("SznSearch: Message index not found, creating...",
			mlog.String("index_name", common.MessageIndex),
		)
		if appErr := s.createIndex(common.MessageIndex, messageIndexSettings); appErr != nil {
			return appErr
		}
		s.Platform.Log().Info("SznSearch: Message index created successfully",
			mlog.String("index_name", common.MessageIndex),
		)
	} else {
		s.Platform.Log().Info("SznSearch: Message index already exists",
			mlog.String("index_name", common.MessageIndex),
		)
	}

	// Check if state index exists
	res, err = s.client.Indices.Exists([]string{common.StateIndex})
	if err != nil {
		return model.NewAppError("SznSearch.ensureIndices", "sznsearch.ensure_indices.check_error", nil, err.Error(), http.StatusInternalServerError)
	}
	res.Body.Close()

	// Create state index if it doesn't exist
	if res.StatusCode == http.StatusNotFound {
		s.Platform.Log().Info("SznSearch: State index not found, creating...",
			mlog.String("index_name", common.StateIndex),
		)
		if appErr := s.createIndex(common.StateIndex, stateIndexSettings); appErr != nil {
			return appErr
		}
		s.Platform.Log().Info("SznSearch: State index created successfully",
			mlog.String("index_name", common.StateIndex),
		)
	} else {
		s.Platform.Log().Info("SznSearch: State index already exists",
			mlog.String("index_name", common.StateIndex),
		)
	}

	return nil
}

// createIndex creates an ElasticSearch index with the given settings
func (s *SznSearchImpl) createIndex(indexName string, settings map[string]any) *model.AppError {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(settings); err != nil {
		return model.NewAppError("SznSearch.createIndex", "sznsearch.create_index.encode", nil, err.Error(), http.StatusInternalServerError)
	}

	res, err := s.client.Indices.Create(
		indexName,
		s.client.Indices.Create.WithBody(&buf),
	)
	if err != nil {
		return model.NewAppError("SznSearch.createIndex", "sznsearch.create_index.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() {
		return model.NewAppError("SznSearch.createIndex", "sznsearch.create_index.es_error", nil, res.String(), http.StatusInternalServerError)
	}

	return nil
}
