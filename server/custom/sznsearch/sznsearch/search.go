package sznsearch

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

// SearchPosts searches for posts in ElasticSearch
func (s *SznSearchImpl) SearchPosts(channels model.ChannelList, searchParams []*model.SearchParams, page, perPage int) ([]string, model.PostSearchMatches, *model.AppError) {
	if atomic.LoadInt32(&s.ready) == 0 {
		return []string{}, nil, model.NewAppError("SznSearch.SearchPosts", "sznsearch.search_posts.disabled", nil, "", http.StatusInternalServerError)
	}

	// Extract channel IDs
	var channelIds []string
	for _, channel := range channels {
		channelIds = append(channelIds, channel.Id)
	}

	s.Platform.Log().Debug("SznSearch: SearchPosts called",
		mlog.Int("num_channels", len(channelIds)),
		mlog.Int("num_params", len(searchParams)),
		mlog.Int("page", page),
		mlog.Int("per_page", perPage),
	)

	// Build ElasticSearch query
	esQuery := s.buildSearchQuery(searchParams, channelIds, page, perPage)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(esQuery); err != nil {
		s.Platform.Log().Error("SznSearch: Failed to encode search query", mlog.Err(err))
		return nil, nil, model.NewAppError("SznSearch.SearchPosts", "sznsearch.search_posts.encode", nil, err.Error(), http.StatusInternalServerError)
	}

	s.Platform.Log().Debug("SznSearch: Executing query", mlog.String("query", buf.String()))

	// Execute search
	res, err := s.client.Search(
		s.client.Search.WithIndex(common.MessageIndex),
		s.client.Search.WithBody(&buf),
		s.client.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		s.Platform.Log().Error("SznSearch: Search request failed", mlog.Err(err))
		return nil, nil, model.NewAppError("SznSearch.SearchPosts", "sznsearch.search_posts.error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() {
		s.Platform.Log().Error("SznSearch: ElasticSearch error",
			mlog.Int("status_code", res.StatusCode),
			mlog.String("response", res.String()),
		)
		return nil, nil, model.NewAppError("SznSearch.SearchPosts", "sznsearch.search_posts.es_error", nil, res.String(), http.StatusInternalServerError)
	}

	// Parse response
	var result struct {
		Hits struct {
			Hits []struct {
				ID        string                `json:"_id"`
				Source    common.IndexedMessage `json:"_source"`
				Highlight map[string][]string   `json:"highlight"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		s.Platform.Log().Error("SznSearch: Failed to decode search response", mlog.Err(err))
		return nil, nil, model.NewAppError("SznSearch.SearchPosts", "sznsearch.search_posts.decode", nil, err.Error(), http.StatusInternalServerError)
	}

	s.Platform.Log().Debug("SznSearch: Search results received",
		mlog.Int("num_hits", len(result.Hits.Hits)),
	)

	// Extract post IDs and matches
	postIds := make([]string, 0, len(result.Hits.Hits))
	matches := make(model.PostSearchMatches)

	for _, hit := range result.Hits.Hits {
		postIds = append(postIds, hit.ID)

		// Extract highlighted terms
		if len(hit.Highlight) > 0 {
			terms := []string{}
			for _, highlights := range hit.Highlight {
				for _, highlight := range highlights {
					// Extract terms from highlight (strip ** markers)
					cleaned := strings.ReplaceAll(highlight, "**", "")
					terms = append(terms, cleaned)
				}
			}
			if len(terms) > 0 {
				matches[hit.ID] = terms
				s.Platform.Log().Debug("SznSearch: Extracted highlight terms",
					mlog.String("post_id", hit.ID),
					mlog.Int("num_terms", len(terms)),
				)
			}
		}
	}

	return postIds, matches, nil
}

// buildSearchQuery constructs an ElasticSearch query from search parameters
func (s *SznSearchImpl) buildSearchQuery(searchParams []*model.SearchParams, channelIds []string, page, perPage int) map[string]any {
	s.Platform.Log().Debug("SznSearch: Building search query",
		mlog.Int("num_params", len(searchParams)),
		mlog.Int("num_channels", len(channelIds)),
		mlog.Int("page", page),
		mlog.Int("per_page", perPage),
	)

	query := map[string]any{
		"from": page * perPage,
		"size": perPage,
		"sort": []any{
			"_score",
			map[string]string{"CreatedAt": "desc"},
		},
		"query": map[string]any{
			"bool": map[string]any{
				"filter":   []map[string]any{},
				"must":     []map[string]any{},
				"must_not": []map[string]any{},
			},
		},
		"highlight": map[string]any{
			"fields": map[string]map[string][]string{
				"Message": {
					"pre_tags":  {"**"},
					"post_tags": {"**"},
				},
			},
		},
	}

	boolQuery := query["query"].(map[string]any)["bool"].(map[string]any)
	filters := boolQuery["filter"].([]map[string]any)
	musts := boolQuery["must"].([]map[string]any)
	mustNots := boolQuery["must_not"].([]map[string]any)

	// Add channel filter
	filters = append(filters, map[string]any{
		"terms": map[string][]string{"ChannelId": channelIds},
	})
	s.Platform.Log().Debug("SznSearch: Added channel filter", mlog.Int("num_channels", len(channelIds)))

	// Process search parameters
	for i, params := range searchParams {
		s.Platform.Log().Debug("SznSearch: Processing search param",
			mlog.Int("param_index", i),
			mlog.String("terms", params.Terms),
			mlog.Int("num_from_users", len(params.FromUsers)),
			mlog.Int("num_excluded_users", len(params.ExcludedUsers)),
		)

		// Main search terms
		if params.Terms != "" {
			musts = append(musts, map[string]any{
				"multi_match": map[string]any{
					"query":    params.Terms,
					"fields":   []string{"Message", "Payload"},
					"operator": "and",
				},
			})
		}

		// User filters
		if len(params.FromUsers) > 0 {
			filters = append(filters, map[string]any{
				"terms": map[string][]string{"UserId": params.FromUsers},
			})
		}

		if len(params.ExcludedUsers) > 0 {
			mustNots = append(mustNots, map[string]any{
				"terms": map[string][]string{"UserId": params.ExcludedUsers},
			})
		}

		// Date filters
		if params.OnDate != "" {
			before, after := params.GetOnDateMillis()
			filters = append(filters, map[string]any{
				"range": map[string]any{
					"CreatedAt": map[string]int64{
						"gte": before,
						"lte": after,
					},
				},
			})
		} else {
			if params.AfterDate != "" || params.BeforeDate != "" {
				rangeFilter := map[string]any{
					"range": map[string]any{
						"CreatedAt": map[string]any{},
					},
				}

				if params.AfterDate != "" {
					rangeFilter["range"].(map[string]any)["CreatedAt"].(map[string]any)["gte"] = params.GetAfterDateMillis()
				}

				if params.BeforeDate != "" {
					rangeFilter["range"].(map[string]any)["CreatedAt"].(map[string]any)["lte"] = params.GetBeforeDateMillis()
				}

				filters = append(filters, rangeFilter)
			}
		}
	}

	boolQuery["filter"] = filters
	boolQuery["must"] = musts
	boolQuery["must_not"] = mustNots

	s.Platform.Log().Debug("SznSearch: Query built",
		mlog.Int("num_filters", len(filters)),
		mlog.Int("num_must", len(musts)),
		mlog.Int("num_must_not", len(mustNots)),
	)

	return query
}

// SearchChannels searches for channels (simplified implementation)
func (s *SznSearchImpl) SearchChannels(teamId, userID string, term string, isGuest, includeDeleted bool) ([]string, *model.AppError) {
	// For now, return empty - channels are typically searched via database
	// Could be implemented similarly to SearchPosts if needed
	return []string{}, nil
}

// SearchUsersInChannel searches for users in a channel
func (s *SznSearchImpl) SearchUsersInChannel(teamId, channelId string, restrictedToChannels []string, term string, options *model.UserSearchOptions) ([]string, []string, *model.AppError) {
	// For now, return empty - users are typically searched via database
	return []string{}, []string{}, nil
}

// SearchUsersInTeam searches for users in a team
func (s *SznSearchImpl) SearchUsersInTeam(teamId string, restrictedToChannels []string, term string, options *model.UserSearchOptions) ([]string, *model.AppError) {
	// For now, return empty - users are typically searched via database
	return []string{}, nil
}

// SearchFiles searches for files (not implemented yet)
func (s *SznSearchImpl) SearchFiles(channels model.ChannelList, searchParams []*model.SearchParams, page, perPage int) ([]string, *model.AppError) {
	// Not implemented for now
	return []string{}, nil
}
