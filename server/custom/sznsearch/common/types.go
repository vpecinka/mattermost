package common

import (
	"encoding/json"
)

const (
	// Index names and types
	MessageIndexHunspell = "hunspell_index"
	MessageIndexStandard = "czech_standard_index"
	StateIndexStandard   = "simple"

	MessageIndexType = MessageIndexHunspell
	StateIndexType   = StateIndexStandard
	MessageIndex     = "messages"
	StateIndex       = "state"

	// Indexing phases
	IndexingPhaseFull  = "full"
	IndexingPhaseDelta = "delta"

	// Special markers
	NoTeamID         = "@private"
	ChannelReindex   = "#reindex-channel"
	QueryChannelHere = "_"

	// Worker states
	WorkerStateStarting     = "starting"
	WorkerStateStopping     = "stopping"
	WorkerStateIdle         = "idle"
	WorkerStateFullRequest  = "full_request"
	WorkerStateDeltaRequest = "delta_request"
	WorkerStateIndexing     = "indexing"
	WorkerStateStopped      = "stopped"
	WorkerStateError        = "error"
)

// Channel types (matching model.ChannelType but as int8 for ES)
const (
	ChannelTypePublic  = int8(0)
	ChannelTypePrivate = int8(1)
	ChannelTypeGroup   = int8(2)
	ChannelTypeDirect  = int8(3)
)

// IndexedMessage represents a message document in ElasticSearch
type IndexedMessage struct {
	ID          string   `json:"Id"`
	Message     string   `json:"Message"`
	Payload     string   `json:"Payload"`
	Hashtags    []string `json:"Hashtags"`
	CreatedAt   int64    `json:"CreatedAt"`
	ChannelID   string   `json:"ChannelId"`
	ChannelType int8     `json:"ChannelType"`
	TeamID      string   `json:"TeamId"`
	UserID      string   `json:"UserId"`
	Members     []string `json:"Members"`
}

// FoundIndexedMessage represents a search result from ElasticSearch
type FoundIndexedMessage struct {
	Message   IndexedMessage      `json:"_source"`
	Score     float64             `json:"_score"`
	Highlight map[string][]string `json:"highlight"`
}

// ChannelIndexingState tracks the indexing progress for a channel
type ChannelIndexingState struct {
	ChannelID         string `json:"channel_id"`
	State             string `json:"state"` // Worker state
	StartedAt         int64  `json:"started_at"`
	FinishedAt        int64  `json:"finished_at"`
	IndexedCount      int64  `json:"indexed_count"`
	Error             string `json:"error,omitempty"`
	IndexedMessages   int64  `json:"indexed_message"`
	FailedMessages    int64  `json:"failed_messages"`
	LastPostTimestamp int64  `json:"last_post_timestamp"`
	IndexingPhase     string `json:"indexing_phase"`
}

// ChannelIndexingJob represents a channel indexing task for worker pool
type ChannelIndexingJob struct {
	ChannelID   string
	TeamID      string
	MemberList  *[]string
	FullReindex bool
}

// ChannelIndexingResult represents the result of channel indexing
type ChannelIndexingResult struct {
	ChannelID string
	Error     error
	Stopped   bool
}

// GetChannelTypeInt converts model.ChannelType to int8
func GetChannelTypeInt(channelType string) int8 {
	switch channelType {
	case "O": // Open/Public
		return ChannelTypePublic
	case "P": // Private
		return ChannelTypePrivate
	case "G": // Group
		return ChannelTypeGroup
	case "D": // Direct
		return ChannelTypeDirect
	default:
		return ChannelTypePublic
	}
}

// ToJSON converts IndexedMessage to JSON bytes
func (m *IndexedMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// ReindexType represents the type of reindex operation
type ReindexType string

const (
	ReindexTypeFull    ReindexType = "full"
	ReindexTypeTeam    ReindexType = "team"
	ReindexTypeChannel ReindexType = "channel"
	ReindexTypeDelta   ReindexType = "delta"
)

// ReindexInfo tracks information about a running reindex operation
type ReindexInfo struct {
	Type      ReindexType `json:"type"`
	TargetID  string      `json:"target_id"`  // TeamID or ChannelID (empty for full)
	UserID    string      `json:"user_id"`    // User who started the reindex
	StartedAt int64       `json:"started_at"` // Unix timestamp
}

// GetKey returns a unique key for this reindex operation
func (r *ReindexInfo) GetKey() string {
	if r.Type == ReindexTypeFull {
		return "full"
	}
	return string(r.Type) + ":" + r.TargetID
}
