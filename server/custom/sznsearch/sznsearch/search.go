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
		s.Platform.Log().Warn("SznSearch.SearchPosts: engine not ready, returning error")
		return []string{}, nil, model.NewAppError("SznSearch.SearchPosts", "sznsearch.search_posts.disabled", nil, "", http.StatusInternalServerError)
	}

	// Extract channel IDs
	var channelIds []string
	for _, channel := range channels {
		channelIds = append(channelIds, channel.Id)
	}

	// Collect search terms for manual matching (especially for hashtags)
	searchTerms := make(map[string]bool)
	for _, params := range searchParams {
		if params.Terms != "" {
			if params.IsHashtag {
				// Extract hashtag terms
				searchTerm := strings.ToLower(strings.TrimPrefix(params.Terms, "#"))
				for _, term := range strings.Fields(searchTerm) {
					searchTerms[term] = true
				}
			} else {
				// Extract regular search terms (split by space)
				for _, term := range strings.Fields(strings.ToLower(params.Terms)) {
					searchTerms[term] = true
				}
			}
		}
	}

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

		termSet := make(map[string]bool) // Use set to avoid duplicates

		// Extract highlighted terms from ElasticSearch response
		if len(hit.Highlight) > 0 {
			for fieldName, highlights := range hit.Highlight {
				s.Platform.Log().Debug("SznSearch: Processing highlight",
					mlog.String("post_id", hit.ID),
					mlog.String("field", fieldName),
					mlog.Int("num_fragments", len(highlights)),
				)

				for _, fragment := range highlights {
					// Extract terms between ** markers
					parts := strings.Split(fragment, "**")
					for i, part := range parts {
						// Odd indices contain highlighted terms (between markers)
						if i%2 == 1 && part != "" {
							// Normalize: trim spaces and convert to lower for deduplication
							normalized := strings.TrimSpace(strings.ToLower(part))
							if normalized != "" {
								termSet[normalized] = true
							}
						}
					}
				}
			}
		}

		// For hashtag searches, also check if any search term appears in the post's hashtags
		// (since keyword fields don't get highlighted by ElasticSearch)
		for searchTerm := range searchTerms {
			// Check in hashtags
			for _, hashtag := range hit.Source.Hashtags {
				if strings.Contains(strings.ToLower(hashtag), searchTerm) {
					termSet[hashtag] = true
				}
			}
			// Also check in message and payload for regular terms
			if strings.Contains(strings.ToLower(hit.Source.Message), searchTerm) {
				termSet[searchTerm] = true
			}
			if strings.Contains(strings.ToLower(hit.Source.Payload), searchTerm) {
				termSet[searchTerm] = true
			}
		}

		// Convert set to slice
		if len(termSet) > 0 {
			terms := make([]string, 0, len(termSet))
			for term := range termSet {
				terms = append(terms, term)
			}
			matches[hit.ID] = terms

			s.Platform.Log().Debug("SznSearch: Extracted terms",
				mlog.String("post_id", hit.ID),
				mlog.Int("num_terms", len(terms)),
			)
		}
	}

	return postIds, matches, nil
}

// buildSearchQuery constructs an ElasticSearch query from search parameters
func (s *SznSearchImpl) buildSearchQuery(searchParams []*model.SearchParams, channelIds []string, page, perPage int) map[string]any {
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
			"pre_tags":  []string{"**"},
			"post_tags": []string{"**"},
			"fields": map[string]any{
				"Message": map[string]any{
					"type":                "unified",
					"number_of_fragments": 3,
					"fragment_size":       150,
				},
				"Payload": map[string]any{
					"type":                "unified",
					"number_of_fragments": 3,
					"fragment_size":       150,
				},
				"Hashtags.text": map[string]any{
					"type":                "unified",
					"number_of_fragments": 5,
					"fragment_size":       50,
				},
			},
		},
	}

	boolQuery := query["query"].(map[string]any)["bool"].(map[string]any)
	filters := boolQuery["filter"].([]map[string]any)
	musts := boolQuery["must"].([]map[string]any)
	mustNots := boolQuery["must_not"].([]map[string]any)

	// Add channel filter - security constraint (only search in allowed channels)
	filters = append(filters, map[string]any{
		"terms": map[string][]string{"ChannelId": channelIds},
	})

	// Process search parameters
	for i, params := range searchParams {
		// Determine operator based on OrTerms flag
		operator := "and"
		if params.OrTerms {
			operator = "or"
		}

		// Main search terms (process for each item - they can be different)
		if params.Terms != "" {
			// Determine fields and query type based on search type
			if params.IsHashtag {
				// Hashtag search - use bool query with should clause to match both exact keyword and text analysis
				// Remove # prefix and normalize to lowercase (we store hashtags lowercase)
				searchTerm := strings.ToLower(strings.TrimPrefix(params.Terms, "#"))

				// Split multiple hashtags (space-separated)
				hashtagTerms := strings.Fields(searchTerm)

				if len(hashtagTerms) == 1 {
					// Single hashtag - use bool query combining exact match with text match for highlighting
					musts = append(musts, map[string]any{
						"bool": map[string]any{
							"should": []map[string]any{
								{
									"term": map[string]any{
										"Hashtags": hashtagTerms[0],
									},
								},
								{
									"match": map[string]any{
										"Hashtags.text": hashtagTerms[0],
									},
								},
							},
							"minimum_should_match": 1,
						},
					})
				} else {
					// Multiple hashtags - use terms query with minimum_should_match
					if params.OrTerms {
						// OR: at least one hashtag must match
						musts = append(musts, map[string]any{
							"bool": map[string]any{
								"should": []map[string]any{
									{
										"terms": map[string]any{
											"Hashtags": hashtagTerms,
										},
									},
									{
										"multi_match": map[string]any{
											"query":    searchTerm,
											"fields":   []string{"Hashtags.text"},
											"operator": "or",
										},
									},
								},
								"minimum_should_match": 1,
							},
						})
					} else {
						// AND: all hashtags must match (add separate term queries)
						for _, tag := range hashtagTerms {
							musts = append(musts, map[string]any{
								"bool": map[string]any{
									"should": []map[string]any{
										{
											"term": map[string]any{
												"Hashtags": tag,
											},
										},
										{
											"match": map[string]any{
												"Hashtags.text": tag,
											},
										},
									},
									"minimum_should_match": 1,
								},
							})
						}
					}
				}
			} else {
				// Regular search - search in Message and Payload with multi_match
				searchFields := []string{"Message", "Payload"}
				musts = append(musts, map[string]any{
					"multi_match": map[string]any{
						"query":    params.Terms,
						"fields":   searchFields,
						"operator": operator,
					},
				})
			}
		} else if params.ExcludedTerms != "" {
			// Pure negative search (only excluded terms, no positive terms)
			// Add match_all to match all documents, then filter with must_not
			musts = append(musts, map[string]any{
				"match_all": map[string]any{},
			})
		}

		// Excluded terms (-word or -#hashtag) (process for each item - they can be different)
		if params.ExcludedTerms != "" {
			if params.IsHashtag {
				// Excluded hashtag search - normalize to lowercase
				searchTerm := strings.ToLower(strings.TrimPrefix(params.ExcludedTerms, "#"))
				hashtagTerms := strings.Fields(searchTerm)

				if len(hashtagTerms) == 1 {
					mustNots = append(mustNots, map[string]any{
						"term": map[string]any{
							"Hashtags": hashtagTerms[0],
						},
					})
				} else {
					// Multiple excluded hashtags
					if params.OrTerms {
						// OR: exclude if any hashtag matches
						mustNots = append(mustNots, map[string]any{
							"terms": map[string]any{
								"Hashtags": hashtagTerms,
							},
						})
					} else {
						// AND: exclude if all hashtags match
						for _, tag := range hashtagTerms {
							mustNots = append(mustNots, map[string]any{
								"term": map[string]any{
									"Hashtags": tag,
								},
							})
						}
					}
				}
			} else {
				// Regular excluded search - search in Message and Payload
				searchFields := []string{"Message", "Payload"}
				mustNots = append(mustNots, map[string]any{
					"multi_match": map[string]any{
						"query":    params.ExcludedTerms,
						"fields":   searchFields,
						"operator": operator,
					},
				})
			}
		}

		// Global filters - process only once (same for all searchParams items)
		if i == 0 {
			// Add user channel filters (in:channel)
			if len(params.InChannels) > 0 {
				filters = append(filters, map[string]any{
					"terms": map[string][]string{"ChannelId": params.InChannels},
				})
			}
			// Exclude channels (-in:channel)
			if len(params.ExcludedChannels) > 0 {
				mustNots = append(mustNots, map[string]any{
					"terms": map[string][]string{"ChannelId": params.ExcludedChannels},
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

			// Excluded date filters (-after:, -before:, -on:)
			if params.ExcludedDate != "" {
				before, after := params.GetExcludedDateMillis()
				mustNots = append(mustNots, map[string]any{
					"range": map[string]any{
						"CreatedAt": map[string]int64{
							"gte": before,
							"lte": after,
						},
					},
				})
			}

			if params.ExcludedAfterDate != "" {
				mustNots = append(mustNots, map[string]any{
					"range": map[string]any{
						"CreatedAt": map[string]int64{
							"gte": params.GetExcludedAfterDateMillis(),
						},
					},
				})
			}

			if params.ExcludedBeforeDate != "" {
				mustNots = append(mustNots, map[string]any{
					"range": map[string]any{
						"CreatedAt": map[string]int64{
							"lte": params.GetExcludedBeforeDateMillis(),
						},
					},
				})
			}
		}
	}

	boolQuery["filter"] = filters
	boolQuery["must"] = musts
	boolQuery["must_not"] = mustNots

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
