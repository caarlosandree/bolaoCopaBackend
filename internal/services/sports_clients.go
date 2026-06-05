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

type OddsAPIClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

type OddsEvent struct {
	ID     json.RawMessage `json:"id"`
	Home   string          `json:"home"`
	Away   string          `json:"away"`
	Date   string          `json:"date"`
	Status string          `json:"status"`
}

func (e OddsEvent) IDString() string {
	var s string
	if err := json.Unmarshal(e.ID, &s); err == nil {
		return s
	}
	var n int64
	if err := json.Unmarshal(e.ID, &n); err == nil {
		return fmt.Sprint(n)
	}
	return strings.Trim(string(e.ID), `"`)
}

func NewOddsAPIClient(baseURL, apiKey string) *OddsAPIClient {
	return &OddsAPIClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     strings.TrimSpace(apiKey),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *OddsAPIClient) ListFootballEvents(ctx context.Context) ([]OddsEvent, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("ODDS_API_KEY não configurada")
	}
	values := url.Values{}
	values.Set("apiKey", c.APIKey)
	values.Set("sport", "football")
	var events []OddsEvent
	if err := c.getJSON(ctx, "/events?"+values.Encode(), &events); err != nil {
		return nil, err
	}
	return events, nil
}

func (c *OddsAPIClient) EventOddsRaw(ctx context.Context, eventID string) (json.RawMessage, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("ODDS_API_KEY não configurada")
	}
	values := url.Values{}
	values.Set("apiKey", c.APIKey)
	values.Set("eventId", eventID)
	values.Set("bookmakers", "bet365,pinnacle,betmgm,draftkings,fanduel")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/odds?"+values.Encode(), nil)
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

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET /odds retornou HTTP %d", resp.StatusCode)
	}
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *OddsAPIClient) getJSON(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bolao-copa/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s retornou HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}
