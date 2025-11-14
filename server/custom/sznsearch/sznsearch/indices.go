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

// Index settings for user autocomplete
var userIndexSettings = map[string]any{
	"settings": map[string]any{
		"index": map[string]any{
			"number_of_replicas": 2,
			"number_of_shards":   3,
		},
	},
	"mappings": map[string]any{
		"properties": map[string]any{
			"suggestions_with_fullname": map[string]any{
				"type": "keyword",
			},
			"suggestions_without_fullname": map[string]any{
				"type": "keyword",
			},
			"team_id": map[string]any{
				"type": "keyword",
			},
			"channel_id": map[string]any{
				"type": "keyword",
			},
			"delete_at": map[string]any{
				"type": "long",
			},
			"roles": map[string]any{
				"type": "keyword",
			},
		},
	},
}

// Index settings for channel autocomplete
var channelIndexSettings = map[string]any{
	"settings": map[string]any{
		"index": map[string]any{
			"number_of_replicas": 2,
			"number_of_shards":   3,
		},
	},
	"mappings": map[string]any{
		"properties": map[string]any{
			"name_suggestions": map[string]any{
				"type": "keyword",
			},
			"team_id": map[string]any{
				"type": "keyword",
			},
			"user_ids": map[string]any{
				"type": "keyword",
			},
			"team_member_ids": map[string]any{
				"type": "keyword",
			},
			"type": map[string]any{
				"type": "keyword",
			},
			"delete_at": map[string]any{
				"type": "long",
			},
		},
	},
}

// indexDefinition holds index name and its settings
type indexDefinition struct {
	name     string
	settings map[string]any
}

// ensureIndices creates ElasticSearch indices if they don't exist
func (s *SznSearchImpl) ensureIndices() error {
	s.Platform.Log().Info("SznSearch: Checking if indices exist")

	indices := []indexDefinition{
		{name: common.MessageIndex, settings: messageIndexSettings},
		{name: common.UserIndex, settings: userIndexSettings},
		{name: common.ChannelIndex, settings: channelIndexSettings},
	}

	for _, idx := range indices {
		if err := s.ensureIndex(idx.name, idx.settings); err != nil {
			return err
		}
	}

	return nil
}

// ensureIndex checks if an index exists and creates it if needed
func (s *SznSearchImpl) ensureIndex(indexName string, settings map[string]any) error {
	res, err := s.client.Indices.Exists([]string{indexName})
	if err != nil {
		return model.NewAppError("SznSearch.ensureIndex", "sznsearch.ensure_index.check_error", nil, err.Error(), http.StatusInternalServerError)
	}
	res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		s.Platform.Log().Info("SznSearch: Index not found, creating...",
			mlog.String("index_name", indexName),
		)
		if appErr := s.createIndex(indexName, settings); appErr != nil {
			return appErr
		}
		s.Platform.Log().Info("SznSearch: Index created successfully",
			mlog.String("index_name", indexName),
		)
	} else {
		s.Platform.Log().Info("SznSearch: Index already exists",
			mlog.String("index_name", indexName),
		)
	}

	return nil
}

// createIndex creates an ElasticSearch index with the given settings
func (s *SznSearchImpl) createIndex(indexName string, settings map[string]any) error {
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
