package sznsearch

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

// SearchPosts searches for posts in ElasticSearch
func (s *SznSearchImpl) SearchPosts(channels model.ChannelList, searchParams []*model.SearchParams, page, perPage int) ([]string, model.PostSearchMatches, *model.AppError) {
	if !s.IsSearchEnabled() {
		s.Platform.Log().Warn("SznSearch.SearchPosts: search not enabled, returning error")
		return []string{}, nil, model.NewAppError("SznSearch.SearchPosts", "sznsearch.search_posts.disabled", nil, "", http.StatusInternalServerError)
	}

	if !s.isBackendHealthy() {
		s.Platform.Log().Warn("SznSearch.SearchPosts: ES backend unhealthy, returning error")
		return []string{}, nil, model.NewAppError("SznSearch.SearchPosts", "sznsearch.search_posts.backend_unhealthy", nil, "", http.StatusServiceUnavailable)
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

	// Execute search with retry + backoff
	var result struct {
		Hits struct {
			Hits []struct {
				ID        string                `json:"_id"`
				Source    common.IndexedMessage `json:"_source"`
				Score     float64               `json:"_score"`
				Highlight map[string][]string   `json:"highlight"`
			} `json:"hits"`
		} `json:"hits"`
	}

	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		bufCopy := bytes.NewBuffer(buf.Bytes())
		res, searchErr := s.client.Search(
			s.client.Search.WithIndex(common.MessageIndex),
			s.client.Search.WithBody(bufCopy),
			s.client.Search.WithTrackTotalHits(true),
		)
		if searchErr != nil {
			return searchErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 4xx errors are client errors - don't retry, return immediately
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				// Use a special error type that stops retry
				err := model.NewAppError("SearchPosts", "es_client_error", nil, res.String(), res.StatusCode)
				// Mark as non-retryable by wrapping in a way that RetryWithBackoff won't retry
				return &common.NonRetryableError{Err: err}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("SearchPosts", "es_error", nil, res.String(), res.StatusCode)
			}
		}

		// Parse response
		if parseErr := json.NewDecoder(res.Body).Decode(&result); parseErr != nil {
			return parseErr
		}
		return nil
	})

	if err != nil {
		s.Platform.Log().Error("SznSearch: Search request failed", mlog.Err(err))
		// Record circuit breaker failure for:
		// - 5xx errors (server errors)
		// - Network errors (timeouts, connection failures)
		// Note: 4xx errors are wrapped in NonRetryableError by retry wrapper, so they stop retry immediately
		var appErr *model.AppError
		if errors.As(err, &appErr) && appErr.StatusCode >= 500 {
			// 5xx server errors trigger circuit breaker
			s.circuitBreaker.RecordFailure()
		} else if !errors.As(err, &appErr) {
			// Non-AppError means network/timeout error - trigger circuit breaker
			s.circuitBreaker.RecordFailure()
		}
		return nil, nil, model.NewAppError("SznSearch.SearchPosts", "sznsearch.search_posts.error", nil, err.Error(), http.StatusInternalServerError)
	}

	s.circuitBreaker.RecordSuccess()

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
	if !s.IsAutocompletionEnabled() {
		return []string{}, nil
	}

	if !s.isBackendHealthy() {
		s.Platform.Log().Warn("SznSearch.SearchChannels: ES backend unhealthy, returning empty")
		return []string{}, nil
	}

	// Build ES query
	esQuery := s.buildChannelSearchQuery(teamId, userID, term, isGuest, includeDeleted)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(esQuery); err != nil {
		s.Platform.Log().Error("SznSearch: Failed to encode channel search query", mlog.Err(err))
		return nil, model.NewAppError("SznSearch.SearchChannels", "sznsearch.search_channels.encode", nil, err.Error(), http.StatusInternalServerError)
	}

	s.Platform.Log().Debug("SznSearch: Executing channel search query", mlog.String("query", buf.String()))

	// Execute search with retry
	var result struct {
		Hits struct {
			Hits []struct {
				Source common.ESChannel `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		bufCopy := bytes.NewBuffer(buf.Bytes())
		res, searchErr := s.client.Search(
			s.client.Search.WithIndex(common.ChannelIndex),
			s.client.Search.WithBody(bufCopy),
		)
		if searchErr != nil {
			return searchErr
		}
		defer res.Body.Close()

		if res.IsError() {
			// 4xx errors are client errors - don't retry
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{
					Err: model.NewAppError("SearchChannels", "es_client_error", nil, res.String(), res.StatusCode),
				}
			}
			// 5xx errors are server errors - retry
			if res.StatusCode >= 500 {
				return model.NewAppError("SearchChannels", "es_server_error", nil, res.String(), res.StatusCode)
			}
		}

		if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
			return &common.NonRetryableError{
				Err: model.NewAppError("SearchChannels", "es_decode_error", nil, err.Error(), http.StatusInternalServerError),
			}
		}
		return nil
	})

	if err != nil {
		// Record circuit breaker failure for server/network errors
		var appErr *model.AppError
		if errors.As(err, &appErr) && appErr.StatusCode >= 500 {
			s.circuitBreaker.RecordFailure()
		} else if !errors.As(err, &appErr) {
			// Non-AppError means network/timeout error
			s.circuitBreaker.RecordFailure()
		}

		if errors.As(err, &appErr) {
			return nil, appErr
		}
		return nil, model.NewAppError("SznSearch.SearchChannels", "sznsearch.search_channels.error", nil, err.Error(), http.StatusInternalServerError)
	}

	s.circuitBreaker.RecordSuccess()

	// Extract channel IDs
	channelIds := make([]string, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		channelIds = append(channelIds, hit.Source.Id)
	}

	s.Platform.Log().Debug("SznSearch: Channel search completed",
		mlog.String("term", term),
		mlog.Int("results", len(channelIds)),
	)

	return channelIds, nil
}

// buildChannelSearchQuery builds an ElasticSearch query for channel autocomplete
func (s *SznSearchImpl) buildChannelSearchQuery(teamId, userID string, term string, isGuest, includeDeleted bool) map[string]any {
	// Base prefix match on name_suggestions
	prefixMatch := map[string]any{
		"prefix": map[string]any{
			"name_suggestions": strings.ToLower(term),
		},
	}

	// Build visibility filter (public channels OR private channels where user is member)
	var visibilityFilter []map[string]any

	if !isGuest {
		// Regular users can see public channels and private channels they're members of
		visibilityFilter = []map[string]any{
			// Public channels (not private)
			{
				"bool": map[string]any{
					"must_not": []map[string]any{
						{
							"term": map[string]any{
								"type": model.ChannelTypePrivate,
							},
						},
					},
				},
			},
			// Private channels where user is member
			{
				"bool": map[string]any{
					"must": []map[string]any{
						{
							"term": map[string]any{
								"type": model.ChannelTypePrivate,
							},
						},
						{
							"term": map[string]any{
								"user_ids": userID,
							},
						},
					},
				},
			},
		}
	} else {
		// Guests can only see public channels
		visibilityFilter = []map[string]any{
			{
				"bool": map[string]any{
					"must_not": []map[string]any{
						{
							"term": map[string]any{
								"type": model.ChannelTypePrivate,
							},
						},
					},
				},
			},
		}
	}

	// Build filter array
	filters := []map[string]any{}

	// Add team filter
	if teamId != "" {
		filters = append(filters, map[string]any{
			"term": map[string]any{
				"team_id": teamId,
			},
		})
	} else {
		// Search across all teams, but only channels where user is team member
		filters = append(filters, map[string]any{
			"term": map[string]any{
				"team_member_ids": userID,
			},
		})
	}

	// Add deletion filter
	if !includeDeleted {
		filters = append(filters, map[string]any{
			"term": map[string]any{
				"delete_at": 0,
			},
		})
	}

	// Combine everything
	query := map[string]any{
		"query": map[string]any{
			"bool": map[string]any{
				"filter": append(filters, map[string]any{
					"bool": map[string]any{
						"should":               visibilityFilter,
						"minimum_should_match": 1,
						"must":                 []map[string]any{prefixMatch},
					},
				}),
			},
		},
		"size": model.ChannelSearchDefaultLimit,
	}

	return query
}

// SearchUsersInChannel searches for users in a channel
func (s *SznSearchImpl) SearchUsersInChannel(teamId, channelId string, restrictedToChannels []string, term string, options *model.UserSearchOptions) ([]string, []string, *model.AppError) {
	if restrictedToChannels != nil && len(restrictedToChannels) == 0 {
		return []string{}, []string{}, nil
	}

	// Search for users in the channel
	uchan, err := s.autocompleteUsersInChannel(channelId, term, options)
	if err != nil {
		return nil, nil, err
	}

	// Search for users not in the channel (but in the team)
	var nuchan []common.ESUser
	nuchan, err = s.autocompleteUsersNotInChannel(teamId, channelId, restrictedToChannels, term, options)
	if err != nil {
		return nil, nil, err
	}

	// Extract user IDs
	uchanIds := make([]string, 0, len(uchan))
	for _, user := range uchan {
		uchanIds = append(uchanIds, user.Id)
	}
	nuchanIds := make([]string, 0, len(nuchan))
	for _, user := range nuchan {
		nuchanIds = append(nuchanIds, user.Id)
	}

	return uchanIds, nuchanIds, nil
}

// SearchUsersInTeam searches for users in a team
func (s *SznSearchImpl) SearchUsersInTeam(teamId string, restrictedToChannels []string, term string, options *model.UserSearchOptions) ([]string, *model.AppError) {
	if restrictedToChannels != nil && len(restrictedToChannels) == 0 {
		return []string{}, nil
	}

	var users []common.ESUser
	var err *model.AppError
	if restrictedToChannels == nil {
		users, err = s.autocompleteUsersInTeam(teamId, term, options)
	} else {
		users, err = s.autocompleteUsersInChannels(restrictedToChannels, term, options)
	}
	if err != nil {
		return nil, err
	}

	// Limit results
	if len(users) >= options.Limit {
		users = users[:options.Limit]
	}

	// Extract user IDs
	usersIds := make([]string, 0, len(users))
	for _, user := range users {
		usersIds = append(usersIds, user.Id)
	}

	return usersIds, nil
}

// executeUserSearch executes an ElasticSearch query and returns user results
// This is a helper function to reduce code duplication in autocomplete methods
func (s *SznSearchImpl) executeUserSearch(searchRequest map[string]any, contextName string) ([]common.ESUser, *model.AppError) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(searchRequest); err != nil {
		s.Platform.Log().Error("SznSearch: Failed to encode user search query", mlog.Err(err))
		return nil, model.NewAppError("SznSearch."+contextName, "sznsearch."+contextName+".encode", nil, err.Error(), http.StatusInternalServerError)
	}

	// Execute search with retry
	var result struct {
		Hits struct {
			Hits []struct {
				Source common.ESUser `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		bufCopy := bytes.NewBuffer(buf.Bytes())
		res, searchErr := s.client.Search(
			s.client.Search.WithIndex(common.UserIndex),
			s.client.Search.WithBody(bufCopy),
		)
		if searchErr != nil {
			return searchErr
		}
		defer res.Body.Close()

		if res.IsError() {
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				return &common.NonRetryableError{Err: model.NewAppError(contextName, "es_client_error", nil, res.String(), res.StatusCode)}
			}
			if res.StatusCode >= 500 {
				return model.NewAppError(contextName, "es_server_error", nil, res.String(), res.StatusCode)
			}
		}

		if decodeErr := json.NewDecoder(res.Body).Decode(&result); decodeErr != nil {
			return &common.NonRetryableError{Err: model.NewAppError(contextName, "es_decode_error", nil, decodeErr.Error(), http.StatusInternalServerError)}
		}

		return nil
	})

	if err != nil {
		// Record circuit breaker failure for server/network errors
		var appErr *model.AppError
		if errors.As(err, &appErr) && appErr.StatusCode >= 500 {
			s.circuitBreaker.RecordFailure()
		} else if !errors.As(err, &appErr) {
			// Non-AppError means network/timeout error
			s.circuitBreaker.RecordFailure()
		}

		if errors.As(err, &appErr) {
			return nil, appErr
		}
		return nil, model.NewAppError("SznSearch."+contextName, "sznsearch."+contextName+".error", nil, err.Error(), http.StatusInternalServerError)
	}

	s.circuitBreaker.RecordSuccess()

	// Extract users from results
	users := make([]common.ESUser, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		users = append(users, hit.Source)
	}

	return users, nil
}

// autocompleteUsers is a generic autocomplete function for users
func (s *SznSearchImpl) autocompleteUsers(contextCategory string, categoryIds []string, term string, options *model.UserSearchOptions) ([]common.ESUser, *model.AppError) {
	if !s.IsAutocompletionEnabled() {
		return nil, model.NewAppError("SznSearch.autocompleteUsers", "sznsearch.autocomplete_users.disabled", nil, "", http.StatusInternalServerError)
	}

	if !s.isBackendHealthy() {
		return nil, model.NewAppError("SznSearch.autocompleteUsers", "sznsearch.autocomplete_users.backend_unhealthy", nil, "", http.StatusServiceUnavailable)
	}

	// Build ES query
	query := map[string]any{
		"bool": map[string]any{
			"must":   []any{},
			"filter": []any{},
		},
	}

	// Add prefix search on suggestions
	if term != "" {
		var suggestionField string
		if options.AllowFullNames {
			suggestionField = "suggestions_with_fullname"
		} else {
			suggestionField = "suggestions_without_fullname"
		}
		query["bool"].(map[string]any)["must"] = append(
			query["bool"].(map[string]any)["must"].([]any),
			map[string]any{
				"prefix": map[string]any{
					suggestionField: strings.ToLower(term),
				},
			},
		)
	}

	// Add category filter (team_id or channel_id)
	if len(categoryIds) > 0 {
		var nonEmptyCategoryIds []string
		for _, id := range categoryIds {
			if id != "" {
				nonEmptyCategoryIds = append(nonEmptyCategoryIds, id)
			}
		}
		if len(nonEmptyCategoryIds) > 0 {
			query["bool"].(map[string]any)["filter"] = append(
				query["bool"].(map[string]any)["filter"].([]any),
				map[string]any{
					"terms": map[string]any{
						contextCategory: nonEmptyCategoryIds,
					},
				},
			)
		}
	}

	// Filter inactive users
	if !options.AllowInactive {
		query["bool"].(map[string]any)["filter"] = append(
			query["bool"].(map[string]any)["filter"].([]any),
			map[string]any{
				"bool": map[string]any{
					"should": []any{
						map[string]any{
							"range": map[string]any{
								"delete_at": map[string]any{
									"lte": 0,
								},
							},
						},
						map[string]any{
							"bool": map[string]any{
								"must_not": []any{
									map[string]any{
										"exists": map[string]any{
											"field": "delete_at",
										},
									},
								},
							},
						},
					},
				},
			},
		)
	}

	// Filter by role
	if options.Role != "" {
		query["bool"].(map[string]any)["filter"] = append(
			query["bool"].(map[string]any)["filter"].([]any),
			map[string]any{
				"term": map[string]any{
					"roles": options.Role,
				},
			},
		)
	}

	// Build search request
	searchRequest := map[string]any{
		"query": query,
		"size":  options.Limit,
	}

	// Use helper function to execute search
	return s.executeUserSearch(searchRequest, "autocompleteUsers")
}

// autocompleteUsersInChannel searches for users in a specific channel
func (s *SznSearchImpl) autocompleteUsersInChannel(channelId, term string, options *model.UserSearchOptions) ([]common.ESUser, *model.AppError) {
	return s.autocompleteUsers("channel_id", []string{channelId}, term, options)
}

// autocompleteUsersInChannels searches for users in multiple channels
func (s *SznSearchImpl) autocompleteUsersInChannels(channelIds []string, term string, options *model.UserSearchOptions) ([]common.ESUser, *model.AppError) {
	return s.autocompleteUsers("channel_id", channelIds, term, options)
}

// autocompleteUsersInTeam searches for users in a team
func (s *SznSearchImpl) autocompleteUsersInTeam(teamId, term string, options *model.UserSearchOptions) ([]common.ESUser, *model.AppError) {
	return s.autocompleteUsers("team_id", []string{teamId}, term, options)
}

// autocompleteUsersNotInChannel searches for users in a team but not in a specific channel
func (s *SznSearchImpl) autocompleteUsersNotInChannel(teamId, channelId string, restrictedToChannels []string, term string, options *model.UserSearchOptions) ([]common.ESUser, *model.AppError) {
	if !s.IsAutocompletionEnabled() {
		return nil, model.NewAppError("SznSearch.autocompleteUsersNotInChannel", "sznsearch.autocomplete_users_not_in_channel.disabled", nil, "", http.StatusInternalServerError)
	}

	if !s.isBackendHealthy() {
		return nil, model.NewAppError("SznSearch.autocompleteUsersNotInChannel", "sznsearch.autocomplete_users_not_in_channel.backend_unhealthy", nil, "", http.StatusServiceUnavailable)
	}

	// Build filter - search across all teams (not restricted to current team)
	var filterMust []any

	// Add restriction to specific channels if provided (for guest users)
	if len(restrictedToChannels) > 0 {
		filterMust = append(filterMust, map[string]any{
			"terms": map[string]any{
				"channel_id": restrictedToChannels,
			},
		})
	}

	// Build query to exclude users in the specified channel
	query := map[string]any{
		"bool": map[string]any{
			"must_not": []any{
				map[string]any{
					"term": map[string]any{
						"channel_id": channelId,
					},
				},
			},
		},
	}

	// Add filter constraints if any exist
	if len(filterMust) > 0 {
		query["bool"].(map[string]any)["filter"] = []any{
			map[string]any{
				"bool": map[string]any{
					"must": filterMust,
				},
			},
		}
	}

	// Add prefix search on suggestions
	if term != "" {
		var suggestionField string
		if options.AllowFullNames {
			suggestionField = "suggestions_with_fullname"
		} else {
			suggestionField = "suggestions_without_fullname"
		}
		query["bool"].(map[string]any)["must"] = []any{
			map[string]any{
				"prefix": map[string]any{
					suggestionField: strings.ToLower(term),
				},
			},
		}
	}

	// Filter inactive users
	if !options.AllowInactive {
		query["bool"].(map[string]any)["filter"] = append(
			query["bool"].(map[string]any)["filter"].([]any),
			map[string]any{
				"bool": map[string]any{
					"should": []any{
						map[string]any{
							"range": map[string]any{
								"delete_at": map[string]any{
									"lte": 0,
								},
							},
						},
						map[string]any{
							"bool": map[string]any{
								"must_not": []any{
									map[string]any{
										"exists": map[string]any{
											"field": "delete_at",
										},
									},
								},
							},
						},
					},
				},
			},
		)
	}

	// Filter by role
	if options.Role != "" {
		query["bool"].(map[string]any)["filter"] = append(
			query["bool"].(map[string]any)["filter"].([]any),
			map[string]any{
				"term": map[string]any{
					"roles": options.Role,
				},
			},
		)
	}

	// Build search request
	searchRequest := map[string]any{
		"query": query,
		"size":  options.Limit,
	}

	// Use helper function to execute search
	return s.executeUserSearch(searchRequest, "autocompleteUsersNotInChannel")
}

// SearchFiles searches for files (not implemented yet)
func (s *SznSearchImpl) SearchFiles(channels model.ChannelList, searchParams []*model.SearchParams, page, perPage int) ([]string, *model.AppError) {
	// Not implemented for now
	return []string{}, nil
}
