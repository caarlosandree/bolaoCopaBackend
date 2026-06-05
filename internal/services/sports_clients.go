package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TheSportsDBClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

type TheSportsDBEvent struct {
	IDEvent       string `json:"idEvent"`
	IDAPIFootball string `json:"idAPIfootball"`
	Season        string `json:"strSeason"`
	League        string `json:"strLeague"`
	HomeTeam      string `json:"strHomeTeam"`
	AwayTeam      string `json:"strAwayTeam"`
	HomeTeamID    string `json:"idHomeTeam"`
	AwayTeamID    string `json:"idAwayTeam"`
	HomeScore     *int   `json:"intHomeScore"`
	AwayScore     *int   `json:"intAwayScore"`
	Round         string `json:"intRound"`
	Timestamp     string `json:"strTimestamp"`
	Date          string `json:"dateEvent"`
	Time          string `json:"strTime"`
	Group         string `json:"strGroup"`
	Venue         string `json:"strVenue"`
	Country       string `json:"strCountry"`
	City          string `json:"strCity"`
	HomeBadge     string `json:"strHomeTeamBadge"`
	AwayBadge     string `json:"strAwayTeamBadge"`
	Thumb         string `json:"strThumb"`
	Poster        string `json:"strPoster"`
	Banner        string `json:"strBanner"`
	Status        string `json:"strStatus"`
}

func NewTheSportsDBClient(baseURL, apiKey string) *TheSportsDBClient {
	return &TheSportsDBClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     strings.TrimSpace(apiKey),
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *TheSportsDBClient) ListLeagueSchedule(ctx context.Context, leagueID, season string) ([]TheSportsDBEvent, json.RawMessage, error) {
	var payload struct {
		Schedule []TheSportsDBEvent `json:"schedule"`
	}
	raw, err := c.getJSON(ctx, fmt.Sprintf("/schedule/league/%s/%s", url.PathEscape(leagueID), url.PathEscape(season)), &payload)
	if err != nil {
		return nil, nil, err
	}
	return payload.Schedule, raw, nil
}

func (c *TheSportsDBClient) LookupEventRaw(ctx context.Context, eventID string) (json.RawMessage, error) {
	return c.getJSON(ctx, "/lookup/event/"+url.PathEscape(eventID), nil)
}

func (c *TheSportsDBClient) LookupLineupRaw(ctx context.Context, eventID string) (json.RawMessage, error) {
	return c.getJSON(ctx, "/lookup/event_lineup/"+url.PathEscape(eventID), nil)
}

func (c *TheSportsDBClient) LookupStatsRaw(ctx context.Context, eventID string) (json.RawMessage, error) {
	return c.getJSON(ctx, "/lookup/event_stats/"+url.PathEscape(eventID), nil)
}

func (c *TheSportsDBClient) LookupTimelineRaw(ctx context.Context, eventID string) (json.RawMessage, error) {
	return c.getJSON(ctx, "/lookup/event_timeline/"+url.PathEscape(eventID), nil)
}

func (c *TheSportsDBClient) TeamPreviousRaw(ctx context.Context, teamID string) (json.RawMessage, error) {
	return c.getJSON(ctx, "/schedule/previous/team/"+url.PathEscape(teamID), nil)
}

type TheSportsDBStanding struct {
	IDTeam            string `json:"idTeam"`
	StrTeam           string `json:"strTeam"`
	StrTeamBadge      string `json:"strTeamBadge"`
	IntPlayed         string `json:"intPlayed"`
	IntWin            string `json:"intWin"`
	IntDraw           string `json:"intDraw"`
	IntLoss           string `json:"intLoss"`
	IntGoalsFor       string `json:"intGoalsFor"`
	IntGoalsAgainst   string `json:"intGoalsAgainst"`
	IntGoalDifference string `json:"intGoalDifference"`
	IntPoints         string `json:"intPoints"`
	StrDescription    string `json:"strDescription"`
}

type TheSportsDBTimelineEvent struct {
	IDTimeline  string `json:"idTimeline"`
	IDEvent     string `json:"idEvent"`
	StrTimeline string `json:"strTimeline"`
	StrPlayer   string `json:"strPlayer"`
	StrTeam     string `json:"strTeam"`
	IntTime     string `json:"intTime"`
	StrBadge    string `json:"strBadge"`
}

// LookupLeagueTable busca a classificação dos grupos via API V1 (lookuptable.php).
func (c *TheSportsDBClient) LookupLeagueTable(ctx context.Context, leagueID, season string) ([]TheSportsDBStanding, error) {
	v1URL := fmt.Sprintf(
		"https://www.thesportsdb.com/api/v1/json/%s/lookuptable.php?l=%s&s=%s",
		url.QueryEscape(c.APIKey),
		url.QueryEscape(leagueID),
		url.QueryEscape(season),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v1URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bolao-copa/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("lookuptable retornou HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Table []TheSportsDBStanding `json:"table"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Table, nil
}

func (c *TheSportsDBClient) ListNextLeagueEvents(ctx context.Context, leagueID string) ([]TheSportsDBEvent, error) {
	var payload struct {
		Schedule []TheSportsDBEvent `json:"schedule"`
	}
	_, err := c.getJSON(ctx, "/schedule/next/league/"+url.PathEscape(leagueID), &payload)
	return payload.Schedule, err
}

func (c *TheSportsDBClient) LookupTimeline(ctx context.Context, eventID string) ([]TheSportsDBTimelineEvent, error) {
	var payload struct {
		Timeline []TheSportsDBTimelineEvent `json:"timeline"`
	}
	_, err := c.getJSON(ctx, "/lookup/event_timeline/"+url.PathEscape(eventID), &payload)
	return payload.Timeline, err
}

func (c *TheSportsDBClient) getJSON(ctx context.Context, path string, dest any) (json.RawMessage, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("THESPORTSDB_API_KEY não configurada")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bolao-copa/1.0")
	req.Header.Set("X-API-KEY", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s retornou HTTP %d", path, resp.StatusCode)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	if dest != nil {
		if err := json.Unmarshal(raw, dest); err != nil {
			return nil, err
		}
	}
	return raw, nil
}
