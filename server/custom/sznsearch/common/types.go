package common

import (
	"encoding/json"
)

const (
	// Index names and types
	MessageIndexHunspell = "hunspell_index"
	MessageIndexStandard = "czech_standard_index"

	MessageIndexType = MessageIndexHunspell
	MessageIndex     = "messages"

	// Special markers
	NoTeamID = "@private"
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

// SznSearchSettings holds configuration for the SznSearch engine
type SznSearchSettings struct {
	EnableIndexing        *bool   `json:"EnableIndexing"`
	EnableSearching       *bool   `json:"EnableSearching"`
	EnableAutocomplete    *bool   `json:"EnableAutocomplete"`
	IgnoreChannels        *string `json:"IgnoreChannels"`
	IgnoreTeams           *string `json:"IgnoreTeams"`
	BatchSize             *int    `json:"BatchSize"`
	RequestTimeoutSeconds *int    `json:"RequestTimeoutSeconds"`
	ReindexChannelPool    *int    `json:"ReindexChannelPool"`
	ClientCert            *string `json:"ClientCert"`
	ClientKey             *string `json:"ClientKey"`
	CA                    *string `json:"CA"`
	SkipTLSVerification   *bool   `json:"SkipTLSVerification"`
	ConnectionURL         *string `json:"ConnectionURL"`
	Username              *string `json:"Username"`
	Password              *string `json:"Password"`
	MessageQueueSize      *int    `json:"MessageQueueSize"` // Maximum size of the message queue
}
