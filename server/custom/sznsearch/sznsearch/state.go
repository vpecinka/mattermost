package sznsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

// readChannelIndexingState reads the indexing state for a channel from ElasticSearch
func (s *SznSearchImpl) readChannelIndexingState(channelID string) (*common.ChannelIndexingState, *model.AppError) {
	req := esapi.GetRequest{
		Index:      common.StateIndex,
		DocumentID: channelID,
	}

	res, err := req.Do(context.Background(), s.client)
	if err != nil {
		return nil, model.NewAppError("SznSearch.readChannelIndexingState", "sznsearch.state.read_error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	// Channel not indexed yet - return empty state
	if res.StatusCode == http.StatusNotFound {
		return &common.ChannelIndexingState{
			ChannelID: channelID,
			State:     common.WorkerStateStopped,
		}, nil
	}

	if res.IsError() {
		return nil, model.NewAppError("SznSearch.readChannelIndexingState", "sznsearch.state.es_error", nil, res.String(), http.StatusInternalServerError)
	}

	// Parse response
	var result struct {
		Source common.ChannelIndexingState `json:"_source"`
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, model.NewAppError("SznSearch.readChannelIndexingState", "sznsearch.state.read_body", nil, err.Error(), http.StatusInternalServerError)
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, model.NewAppError("SznSearch.readChannelIndexingState", "sznsearch.state.unmarshal", nil, err.Error(), http.StatusInternalServerError)
	}

	return &result.Source, nil
}

// saveChannelIndexingState saves the indexing state for a channel to ElasticSearch
func (s *SznSearchImpl) saveChannelIndexingState(state *common.ChannelIndexingState) *model.AppError {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(state); err != nil {
		return model.NewAppError("SznSearch.saveChannelIndexingState", "sznsearch.state.encode", nil, err.Error(), http.StatusInternalServerError)
	}

	req := esapi.IndexRequest{
		Index:      common.StateIndex,
		DocumentID: state.ChannelID,
		Body:       &buf,
		Refresh:    "true",
	}

	res, err := req.Do(context.Background(), s.client)
	if err != nil {
		return model.NewAppError("SznSearch.saveChannelIndexingState", "sznsearch.state.save_error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() {
		return model.NewAppError("SznSearch.saveChannelIndexingState", "sznsearch.state.es_error", nil, res.String(), http.StatusInternalServerError)
	}

	return nil
}

// emptyIndexingState resets the indexing state for a channel
func (s *SznSearchImpl) emptyIndexingState(channelID string) *model.AppError {
	state := &common.ChannelIndexingState{
		ChannelID: channelID,
		State:     common.WorkerStateStopped,
	}
	return s.saveChannelIndexingState(state)
}

// getAllChannelIndexingStates retrieves all channel indexing states
func (s *SznSearchImpl) getAllChannelIndexingStates() (map[string]*common.ChannelIndexingState, *model.AppError) {
	query := map[string]any{
		"query": map[string]any{
			"match_all": map[string]any{},
		},
		"size": 10000, // Get all states
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, model.NewAppError("SznSearch.getAllChannelIndexingStates", "sznsearch.state.encode", nil, err.Error(), http.StatusInternalServerError)
	}

	res, err := s.client.Search(
		s.client.Search.WithContext(context.Background()),
		s.client.Search.WithIndex(common.StateIndex),
		s.client.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, model.NewAppError("SznSearch.getAllChannelIndexingStates", "sznsearch.state.search_error", nil, err.Error(), http.StatusInternalServerError)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, model.NewAppError("SznSearch.getAllChannelIndexingStates", "sznsearch.state.es_error", nil, res.String(), http.StatusInternalServerError)
	}

	// Parse response
	var result struct {
		Hits struct {
			Hits []struct {
				Source common.ChannelIndexingState `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, model.NewAppError("SznSearch.getAllChannelIndexingStates", "sznsearch.state.read_body", nil, err.Error(), http.StatusInternalServerError)
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, model.NewAppError("SznSearch.getAllChannelIndexingStates", "sznsearch.state.unmarshal", nil, err.Error(), http.StatusInternalServerError)
	}

	states := make(map[string]*common.ChannelIndexingState)
	for _, hit := range result.Hits.Hits {
		state := hit.Source
		states[state.ChannelID] = &state
	}

	return states, nil
}
