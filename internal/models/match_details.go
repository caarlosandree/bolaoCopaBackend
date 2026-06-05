package models

import (
	"encoding/json"
	"time"
)

type SourceStatus struct {
	Source   string     `json:"source"`
	Section  string     `json:"section"`
	Status   string     `json:"status"`
	Message  string     `json:"message,omitempty"`
	SyncedAt *time.Time `json:"synced_at,omitempty"`
}

type MatchDetails struct {
	MatchID       int             `json:"match_id"`
	Predictions   json.RawMessage `json:"predictions,omitempty"`
	RecentForm    json.RawMessage `json:"recent_form,omitempty"`
	HeadToHead    json.RawMessage `json:"head_to_head,omitempty"`
	Lineups       json.RawMessage `json:"lineups,omitempty"`
	Statistics    json.RawMessage `json:"statistics,omitempty"`
	Injuries      json.RawMessage `json:"injuries,omitempty"`
	Events        json.RawMessage `json:"events,omitempty"`
	Media         json.RawMessage `json:"media,omitempty"`
	SourceStatus  []SourceStatus  `json:"source_status"`
	LastSyncedAt  *time.Time      `json:"last_synced_at,omitempty"`
	LineupsSynced *time.Time      `json:"lineups_synced_at,omitempty"`
	UpdatedAt     *time.Time      `json:"updated_at,omitempty"`
}

type MatchDetailsAvailability struct {
	Predictions bool `json:"predictions"`
	Form        bool `json:"form"`
	H2H         bool `json:"h2h"`
	Lineups     bool `json:"lineups"`
	Statistics  bool `json:"statistics"`
	Injuries    bool `json:"injuries"`
	Events      bool `json:"events"`
	Media       bool `json:"media"`
}

type MatchDetailsResponse struct {
	MatchID       int                      `json:"match_id"`
	Availability  MatchDetailsAvailability `json:"availability"`
	Predictions   json.RawMessage          `json:"predictions"`
	RecentForm    json.RawMessage          `json:"recent_form"`
	HeadToHead    json.RawMessage          `json:"head_to_head"`
	Lineups       json.RawMessage          `json:"lineups"`
	Statistics    json.RawMessage          `json:"statistics"`
	Injuries      json.RawMessage          `json:"injuries"`
	Events        json.RawMessage          `json:"events"`
	Media         json.RawMessage          `json:"media"`
	SourceStatus  []SourceStatus           `json:"source_status"`
	LastSyncedAt  *time.Time               `json:"last_synced_at"`
	LineupsSynced *time.Time               `json:"lineups_synced_at"`
	UpdatedAt     *time.Time               `json:"updated_at"`
}

func (d MatchDetails) Response() MatchDetailsResponse {
	return MatchDetailsResponse{
		MatchID: d.MatchID,
		Availability: MatchDetailsAvailability{
			Predictions: hasJSON(d.Predictions),
			Form:        hasJSON(d.RecentForm),
			H2H:         hasJSON(d.HeadToHead),
			Lineups:     hasJSON(d.Lineups),
			Statistics:  hasJSON(d.Statistics),
			Injuries:    hasJSON(d.Injuries),
			Events:      hasJSON(d.Events),
			Media:       hasJSON(d.Media),
		},
		Predictions:   nullableJSON(d.Predictions),
		RecentForm:    nullableJSON(d.RecentForm),
		HeadToHead:    nullableJSON(d.HeadToHead),
		Lineups:       nullableJSON(d.Lineups),
		Statistics:    nullableJSON(d.Statistics),
		Injuries:      nullableJSON(d.Injuries),
		Events:        nullableJSON(d.Events),
		Media:         nullableJSON(d.Media),
		SourceStatus:  d.SourceStatus,
		LastSyncedAt:  d.LastSyncedAt,
		LineupsSynced: d.LineupsSynced,
		UpdatedAt:     d.UpdatedAt,
	}
}

func hasJSON(value json.RawMessage) bool {
	return len(value) > 0 && string(value) != "null"
}

func nullableJSON(value json.RawMessage) json.RawMessage {
	if hasJSON(value) {
		return value
	}
	return json.RawMessage("null")
}
