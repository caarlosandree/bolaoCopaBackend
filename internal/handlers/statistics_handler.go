package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"backend/internal/repositories"
	"backend/internal/respond"
	"backend/internal/services"

	"github.com/labstack/echo/v5"
)

// ==========================================
// Cache
// ==========================================

type cacheEntry struct {
	data      []byte
	expiresAt time.Time
}

type statisticsCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

func newStatisticsCache() *statisticsCache {
	return &statisticsCache{entries: make(map[string]cacheEntry)}
}

func (c *statisticsCache) get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.data, true
}

func (c *statisticsCache) set(key string, data []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{data: data, expiresAt: time.Now().Add(ttl)}
}

// ==========================================
// Handler
// ==========================================

type StatisticsHandler struct {
	repo     *repositories.StatisticsRepository
	users    *repositories.UserRepository
	sdb      *services.TheSportsDBClient
	cache    *statisticsCache
	leagueID string
	season   string
}

func NewStatisticsHandler(
	repo *repositories.StatisticsRepository,
	users *repositories.UserRepository,
	sdb *services.TheSportsDBClient,
	leagueID, season string,
) *StatisticsHandler {
	return &StatisticsHandler{
		repo:     repo,
		users:    users,
		sdb:      sdb,
		cache:    newStatisticsCache(),
		leagueID: leagueID,
		season:   season,
	}
}

func (h *StatisticsHandler) serveJSON(c *echo.Context, key string, ttl time.Duration, build func() (any, error)) error {
	if cached, ok := h.cache.get(key); ok {
		c.Response().Header().Set("Content-Type", "application/json")
		return c.Blob(http.StatusOK, "application/json", cached)
	}

	payload, err := build()
	if err != nil {
		return respond.InternalError(c, "erro ao buscar estatísticas")
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return respond.InternalError(c, "erro ao serializar resposta")
	}
	h.cache.set(key, data, ttl)

	c.Response().Header().Set("Content-Type", "application/json")
	return c.Blob(http.StatusOK, "application/json", data)
}

// ==========================================
// GET /statistics/copa
// ==========================================

type copaOverviewResponse struct {
	TotalGoals  int            `json:"total_goals"`
	BiggestWin  *biggestWinDTO `json:"biggest_win"`
	TopScorer   *topScorerDTO  `json:"top_scorer"`
	NextMatches []nextMatchDTO `json:"next_matches"`
}

type biggestWinDTO struct {
	HomeTeam  string `json:"home_team"`
	AwayTeam  string `json:"away_team"`
	HomeScore int    `json:"home_score"`
	AwayScore int    `json:"away_score"`
	MatchTime string `json:"match_time"`
}

type topScorerDTO struct {
	Player string `json:"player"`
	Team   string `json:"team"`
	Goals  int    `json:"goals"`
	Badge  string `json:"badge"`
}

type nextMatchDTO struct {
	HomeTeam  string `json:"home_team"`
	AwayTeam  string `json:"away_team"`
	HomeBadge string `json:"home_badge"`
	AwayBadge string `json:"away_badge"`
	MatchTime string `json:"match_time"`
	Round     string `json:"round"`
	Group     string `json:"group,omitempty"`
	Venue     string `json:"venue,omitempty"`
}

func (h *StatisticsHandler) GetCopaOverview(c *echo.Context) error {
	return h.serveJSON(c, "copa_overview", 2*time.Minute, func() (any, error) {
		ctx := c.Request().Context()

		localStats, err := h.repo.GetCopaLocalStats(ctx)
		if err != nil {
			return nil, err
		}

		resp := copaOverviewResponse{
			TotalGoals:  localStats.TotalGoals,
			NextMatches: []nextMatchDTO{},
		}

		if localStats.BiggestWin != nil {
			bw := localStats.BiggestWin
			resp.BiggestWin = &biggestWinDTO{
				HomeTeam:  bw.HomeTeam,
				AwayTeam:  bw.AwayTeam,
				HomeScore: bw.HomeScore,
				AwayScore: bw.AwayScore,
				MatchTime: bw.MatchTime,
			}
		}

		// Artilheiro: agrega gols das partidas ativas
		activeIDs, err := h.repo.GetActiveMatchExternalIDs(ctx)
		if err == nil && len(activeIDs) > 0 {
			goalMap := map[string]*topScorerDTO{}
			for _, id := range activeIDs {
				timeline, err := h.sdb.LookupTimeline(ctx, id)
				if err != nil {
					continue
				}
				for _, ev := range timeline {
					if ev.StrTimeline != "Goal" {
						continue
					}
					key := ev.StrPlayer + "|" + ev.StrTeam
					if _, ok := goalMap[key]; !ok {
						goalMap[key] = &topScorerDTO{
							Player: ev.StrPlayer,
							Team:   ev.StrTeam,
							Badge:  ev.StrBadge,
						}
					}
					goalMap[key].Goals++
				}
			}
			var best *topScorerDTO
			for _, v := range goalMap {
				if best == nil || v.Goals > best.Goals {
					best = v
				}
			}
			resp.TopScorer = best
		}

		// Próximas partidas
		nextEvents, err := h.sdb.ListNextLeagueEvents(ctx, h.leagueID)
		if err == nil {
			limit := 5
			for i, ev := range nextEvents {
				if i >= limit {
					break
				}
				resp.NextMatches = append(resp.NextMatches, nextMatchDTO{
					HomeTeam:  ev.HomeTeam,
					AwayTeam:  ev.AwayTeam,
					HomeBadge: ev.HomeBadge,
					AwayBadge: ev.AwayBadge,
					MatchTime: ev.Timestamp,
					Round:     ev.Round,
					Group:     ev.Group,
					Venue:     ev.Venue,
				})
			}
		}

		return resp, nil
	})
}

// ==========================================
// GET /statistics/groups
// ==========================================

type groupStandingsResponse struct {
	Groups []groupDTO `json:"groups"`
}

type groupDTO struct {
	Name  string            `json:"name"`
	Teams []teamStandingDTO `json:"teams"`
}

type teamStandingDTO struct {
	Name   string `json:"name"`
	Badge  string `json:"badge"`
	Played int    `json:"played"`
	Won    int    `json:"won"`
	Drawn  int    `json:"drawn"`
	Lost   int    `json:"lost"`
	GF     int    `json:"gf"`
	GA     int    `json:"ga"`
	GD     int    `json:"gd"`
	Points int    `json:"points"`
}

func (h *StatisticsHandler) GetGroupStandings(c *echo.Context) error {
	return h.serveJSON(c, "group_standings", 5*time.Minute, func() (any, error) {
		ctx := c.Request().Context()

		standings, err := h.sdb.LookupLeagueTable(ctx, h.leagueID, h.season)
		if err != nil {
			return nil, err
		}

		groupMap := map[string]*groupDTO{}
		var groupOrder []string

		for _, s := range standings {
			norm := normalizeTeamName(s.StrTeam)
			groupName, ok := teamToGroupMap[norm]
			if !ok {
				groupName = s.StrDescription
				if groupName == "" || groupName == "Playoffs" {
					groupName = "Grupo A"
				}
			}
			if _, ok := groupMap[groupName]; !ok {
				groupMap[groupName] = &groupDTO{Name: groupName}
				groupOrder = append(groupOrder, groupName)
			}
			groupMap[groupName].Teams = append(groupMap[groupName].Teams, teamStandingDTO{
				Name:   s.StrTeam,
				Badge:  s.StrTeamBadge,
				Played: atoi(s.IntPlayed),
				Won:    atoi(s.IntWin),
				Drawn:  atoi(s.IntDraw),
				Lost:   atoi(s.IntLoss),
				GF:     atoi(s.IntGoalsFor),
				GA:     atoi(s.IntGoalsAgainst),
				GD:     atoi(s.IntGoalDifference),
				Points: atoi(s.IntPoints),
			})
		}

		// Sort teams within each group (by points, then GD, then GF, then Name)
		for _, g := range groupMap {
			sort.Slice(g.Teams, func(i, j int) bool {
				if g.Teams[i].Points != g.Teams[j].Points {
					return g.Teams[i].Points > g.Teams[j].Points
				}
				if g.Teams[i].GD != g.Teams[j].GD {
					return g.Teams[i].GD > g.Teams[j].GD
				}
				if g.Teams[i].GF != g.Teams[j].GF {
					return g.Teams[i].GF > g.Teams[j].GF
				}
				return g.Teams[i].Name < g.Teams[j].Name
			})
		}

		sort.Strings(groupOrder)
		resp := groupStandingsResponse{}
		for _, name := range groupOrder {
			resp.Groups = append(resp.Groups, *groupMap[name])
		}
		if resp.Groups == nil {
			resp.Groups = []groupDTO{}
		}
		return resp, nil
	})
}

// ==========================================
// GET /statistics/bracket
// ==========================================

type bracketResponse struct {
	Rounds map[string][]bracketMatchDTO `json:"rounds"`
}

type bracketMatchDTO struct {
	ID        string         `json:"id"`
	Home      bracketTeamDTO `json:"home"`
	Away      bracketTeamDTO `json:"away"`
	Status    string         `json:"status"`
	MatchTime string         `json:"match_time"`
	Slot      int            `json:"slot"`
}

type bracketTeamDTO struct {
	Name  string `json:"name"`
	Badge string `json:"badge"`
	Score *int   `json:"score"`
}

var knockoutRoundMap = map[string]string{
	"4":               "r32",
	"5":               "r16",
	"6":               "qf",
	"7":               "sf",
	"8":               "third",
	"9":               "final",
	"32":              "r32",
	"round of 32":     "r32",
	"16":              "r16",
	"round of 16":     "r16",
	"quarter-final":   "qf",
	"quarter-finals":  "qf",
	"quarterfinal":    "qf",
	"semi-final":      "sf",
	"semi-finals":     "sf",
	"semifinal":       "sf",
	"final":           "final",
	"3rd place final": "third",
	"3rd place":       "third",
	"third place":     "third",
}

var localKnockoutRoundMap = map[int]string{
	4:   "r32",
	5:   "r16",
	6:   "qf",
	7:   "sf",
	8:   "third",
	9:   "final",
	100: "r32",
	101: "r16",
	102: "qf",
	103: "sf",
	104: "third",
	105: "final",
}

const roundOf32MatchCount = 16

func (h *StatisticsHandler) GetBracket(c *echo.Context) error {
	return h.serveJSON(c, "bracket", 5*time.Minute, func() (any, error) {
		ctx := c.Request().Context()

		roundOrder := []string{"r32", "r16", "qf", "sf", "final", "third"}
		rounds := map[string][]bracketMatchDTO{}
		for _, r := range roundOrder {
			rounds[r] = []bracketMatchDTO{}
		}

		events, _, externalErr := h.sdb.ListLeagueSchedule(ctx, h.leagueID, h.season)
		if externalErr == nil {
			for _, ev := range events {
				roundKey := normalizeRound(ev.Round)
				if roundKey == "" {
					continue
				}

				upsertBracketMatch(rounds, roundKey, eventToBracketMatch(ev))
			}
		}

		localMatches, localErr := h.repo.ListLocalBracketMatches(ctx)
		if localErr != nil {
			if externalErr != nil {
				return nil, localErr
			}
		} else {
			for _, match := range localMatches {
				roundKey := localKnockoutRoundMap[match.RoundNumber]
				if roundKey == "" {
					continue
				}
				upsertBracketMatch(rounds, roundKey, localToBracketMatch(match))
			}
		}

		standings, standingsErr := h.sdb.LookupLeagueTable(ctx, h.leagueID, h.season)
		if standingsErr == nil {
			existingTeams := roundOf32TeamSet(rounds["r32"])
			remainingSlots := max(0, roundOf32MatchCount-len(rounds["r32"]))
			for _, match := range deriveQualifiedRoundOf32Entries(standings, existingTeams, remainingSlots) {
				upsertBracketMatch(rounds, "r32", match)
			}
		}

		// Atribui slots sequenciais por rodada
		for key := range rounds {
			for i := range rounds[key] {
				rounds[key][i].Slot = i
			}
		}

		return bracketResponse{Rounds: rounds}, nil
	})
}

func eventToBracketMatch(ev services.TheSportsDBEvent) bracketMatchDTO {
	var homeScore, awayScore *int
	if ev.HomeScore != nil {
		if v, err := strconv.Atoi(*ev.HomeScore); err == nil {
			homeScore = &v
		}
	}
	if ev.AwayScore != nil {
		if v, err := strconv.Atoi(*ev.AwayScore); err == nil {
			awayScore = &v
		}
	}

	status := "scheduled"
	switch ev.Status {
	case "Match Finished":
		status = "finished"
	case "In Progress", "HT":
		status = "ongoing"
	}

	return bracketMatchDTO{
		ID: ev.IDEvent,
		Home: bracketTeamDTO{
			Name:  ev.HomeTeam,
			Badge: ev.HomeBadge,
			Score: homeScore,
		},
		Away: bracketTeamDTO{
			Name:  ev.AwayTeam,
			Badge: ev.AwayBadge,
			Score: awayScore,
		},
		Status:    status,
		MatchTime: ev.Timestamp,
	}
}

func localToBracketMatch(match repositories.LocalBracketMatch) bracketMatchDTO {
	id := fmt.Sprintf("local-%d", match.ID)
	if match.TheSportsDBEventID != nil {
		id = *match.TheSportsDBEventID
	}

	return bracketMatchDTO{
		ID: id,
		Home: bracketTeamDTO{
			Name:  match.HomeTeam,
			Score: match.HomeScore,
		},
		Away: bracketTeamDTO{
			Name:  match.AwayTeam,
			Score: match.AwayScore,
		},
		Status:    match.Status,
		MatchTime: match.MatchTime.UTC().Format(time.RFC3339),
	}
}

func upsertBracketMatch(rounds map[string][]bracketMatchDTO, roundKey string, match bracketMatchDTO) {
	if strings.TrimSpace(match.Home.Name) == "" && strings.TrimSpace(match.Away.Name) == "" {
		return
	}

	for i, existing := range rounds[roundKey] {
		if existing.ID == match.ID {
			rounds[roundKey][i] = mergeBracketMatch(existing, match)
			return
		}
	}
	rounds[roundKey] = append(rounds[roundKey], match)
}

func mergeBracketMatch(existing, incoming bracketMatchDTO) bracketMatchDTO {
	if strings.TrimSpace(incoming.Home.Name) == "" {
		incoming.Home.Name = existing.Home.Name
	}
	if strings.TrimSpace(incoming.Away.Name) == "" {
		incoming.Away.Name = existing.Away.Name
	}
	if incoming.Home.Badge == "" {
		incoming.Home.Badge = existing.Home.Badge
	}
	if incoming.Away.Badge == "" {
		incoming.Away.Badge = existing.Away.Badge
	}
	return incoming
}

func deriveQualifiedRoundOf32Entries(
	standings []services.TheSportsDBStanding,
	existingTeams map[string]bool,
	limit int,
) []bracketMatchDTO {
	if limit <= 0 {
		return nil
	}

	qualified := explicitQualifiedStandings(standings)
	addCompletedGroupTopTwo(qualified, standings)

	matches := make([]bracketMatchDTO, 0, len(qualified))
	for normalized, team := range qualified {
		if existingTeams[normalized] {
			continue
		}
		matches = append(matches, qualifiedStandingToBracketMatch(normalized, team))
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Home.Name < matches[j].Home.Name
	})
	if len(matches) > limit {
		return matches[:limit]
	}
	return matches
}

func explicitQualifiedStandings(
	standings []services.TheSportsDBStanding,
) map[string]services.TheSportsDBStanding {
	qualified := map[string]services.TheSportsDBStanding{}
	for _, standing := range standings {
		if !isRoundOf32Description(standing.StrDescription) {
			continue
		}
		qualified[normalizeTeamName(standing.StrTeam)] = standing
	}
	return qualified
}

func addCompletedGroupTopTwo(
	qualified map[string]services.TheSportsDBStanding,
	standings []services.TheSportsDBStanding,
) {
	groupTeams := completedGroupTeams(standings)
	for groupName := range groupTeams {
		teams := groupTeams[groupName]
		for rank := 0; rank < 2 && rank < len(teams); rank++ {
			team := teams[rank]
			qualified[normalizeTeamName(team.StrTeam)] = team
		}
	}
}

func qualifiedStandingToBracketMatch(
	normalized string,
	standing services.TheSportsDBStanding,
) bracketMatchDTO {
	return bracketMatchDTO{
		ID: fmt.Sprintf("qualified-r32-%s", normalized),
		Home: bracketTeamDTO{
			Name:  standing.StrTeam,
			Badge: standingBadge(standing),
		},
		Status:    "scheduled",
		MatchTime: "",
	}
}

func isRoundOf32Description(description string) bool {
	return strings.EqualFold(strings.TrimSpace(description), "Round of 32")
}

func roundOf32TeamSet(matches []bracketMatchDTO) map[string]bool {
	teams := map[string]bool{}
	for _, match := range matches {
		if strings.TrimSpace(match.Home.Name) != "" {
			teams[normalizeTeamName(match.Home.Name)] = true
		}
		if strings.TrimSpace(match.Away.Name) != "" {
			teams[normalizeTeamName(match.Away.Name)] = true
		}
	}
	return teams
}

func standingBadge(standing services.TheSportsDBStanding) string {
	if standing.StrTeamBadge != "" {
		return standing.StrTeamBadge
	}
	return standing.StrBadge
}

func completedGroupTeams(standings []services.TheSportsDBStanding) map[string][]services.TheSportsDBStanding {
	groups := map[string][]services.TheSportsDBStanding{}
	for _, standing := range standings {
		groupName := groupForStanding(standing)
		groups[groupName] = append(groups[groupName], standing)
	}

	for groupName, teams := range groups {
		if !isCompletedGroup(teams) {
			delete(groups, groupName)
			continue
		}
		sort.Slice(teams, func(i, j int) bool {
			return standingLess(teams[i], teams[j])
		})
		groups[groupName] = teams
	}
	return groups
}

func groupForStanding(standing services.TheSportsDBStanding) string {
	norm := normalizeTeamName(standing.StrTeam)
	if groupName, ok := teamToGroupMap[norm]; ok {
		return groupName
	}
	if standing.StrDescription != "" && standing.StrDescription != "Playoffs" {
		return standing.StrDescription
	}
	return "Grupo A"
}

func isCompletedGroup(teams []services.TheSportsDBStanding) bool {
	if len(teams) < 4 {
		return false
	}
	for _, team := range teams {
		if atoi(team.IntPlayed) < 3 {
			return false
		}
	}
	return true
}

func standingLess(a, b services.TheSportsDBStanding) bool {
	if atoi(a.IntPoints) != atoi(b.IntPoints) {
		return atoi(a.IntPoints) > atoi(b.IntPoints)
	}
	if atoi(a.IntGoalDifference) != atoi(b.IntGoalDifference) {
		return atoi(a.IntGoalDifference) > atoi(b.IntGoalDifference)
	}
	if atoi(a.IntGoalsFor) != atoi(b.IntGoalsFor) {
		return atoi(a.IntGoalsFor) > atoi(b.IntGoalsFor)
	}
	return a.StrTeam < b.StrTeam
}

// ==========================================
// GET /statistics/bolao
// ==========================================

type bolaoStatsResponse struct {
	Overview          bolaoOverviewDTO       `json:"overview"`
	PointsEvolution   []userEvolutionDTO     `json:"points_evolution"`
	GuessDistribution []guessDistributionDTO `json:"guess_distribution"`
	AccuracyRanking   []accuracyRankingDTO   `json:"accuracy_ranking"`
}

type bolaoOverviewDTO struct {
	TotalGuesses int           `json:"total_guesses"`
	ExactHits    int           `json:"exact_hits"`
	HitRate      float64       `json:"hit_rate"`
	BestRound    *bestRoundDTO `json:"best_round"`
}

type bestRoundDTO struct {
	Name        string `json:"name"`
	TotalPoints int    `json:"total_points"`
}

type userEvolutionDTO struct {
	User   string          `json:"user"`
	UserID int             `json:"user_id"`
	Avatar *string         `json:"avatar_url"`
	Points []roundPointDTO `json:"points"`
}

type roundPointDTO struct {
	Round            int `json:"round"`
	CumulativePoints int `json:"cumulative_points"`
}

type guessDistributionDTO struct {
	Score string `json:"score"`
	Count int    `json:"count"`
}

type accuracyRankingDTO struct {
	UserID    int     `json:"user_id"`
	Name      string  `json:"name"`
	AvatarURL *string `json:"avatar_url"`
	Exact     int     `json:"exact"`
	Partial   int     `json:"partial"`
	Wrong     int     `json:"wrong"`
	Total     int     `json:"total"`
	Rate      float64 `json:"rate"`
}

func (h *StatisticsHandler) GetBolaoStats(c *echo.Context) error {
	return h.serveJSON(c, "bolao_stats", 2*time.Minute, func() (any, error) {
		ctx := c.Request().Context()

		overview, err := h.repo.GetBolaoOverview(ctx)
		if err != nil {
			return nil, err
		}

		evolution, err := h.repo.GetPointsEvolution(ctx)
		if err != nil {
			return nil, err
		}

		distribution, err := h.repo.GetGuessDistribution(ctx)
		if err != nil {
			return nil, err
		}

		accuracy, err := h.repo.GetAccuracyRanking(ctx)
		if err != nil {
			return nil, err
		}

		hiddenIDs, err := h.users.ListHiddenUserIDs(ctx)
		if err != nil {
			return nil, err
		}
		hiddenMap := make(map[int]bool, len(hiddenIDs))
		for _, id := range hiddenIDs {
			hiddenMap[id] = true
		}

		resp := bolaoStatsResponse{}

		resp.Overview = bolaoOverviewDTO{
			TotalGuesses: overview.TotalGuesses,
			ExactHits:    overview.ExactHits,
			HitRate:      overview.HitRate,
		}
		if overview.BestRound != nil {
			resp.Overview.BestRound = &bestRoundDTO{
				Name:        overview.BestRound.Name,
				TotalPoints: overview.BestRound.TotalPoints,
			}
		}

		// Agrupa evolução por usuário e acumula pontos
		type userKey struct {
			id   string
			name string
		}
		userMap := map[string]*userEvolutionDTO{}
		var userOrder []string
		for _, row := range evolution {
			if hiddenMap[row.UserID] {
				continue
			}
			key := fmt.Sprintf("%d", row.UserID)
			if _, ok := userMap[key]; !ok {
				userMap[key] = &userEvolutionDTO{
					User:   row.Name,
					UserID: row.UserID,
					Avatar: row.AvatarURL,
					Points: []roundPointDTO{},
				}
				userOrder = append(userOrder, key)
			}
			prev := 0
			if len(userMap[key].Points) > 0 {
				prev = userMap[key].Points[len(userMap[key].Points)-1].CumulativePoints
			}
			userMap[key].Points = append(userMap[key].Points, roundPointDTO{
				Round:            row.RoundNumber,
				CumulativePoints: prev + row.RoundPoints,
			})
		}
		for _, k := range userOrder {
			resp.PointsEvolution = append(resp.PointsEvolution, *userMap[k])
		}
		if resp.PointsEvolution == nil {
			resp.PointsEvolution = []userEvolutionDTO{}
		}

		for _, g := range distribution {
			resp.GuessDistribution = append(resp.GuessDistribution, guessDistributionDTO{
				Score: fmt.Sprintf("%d×%d", g.HomeGuess, g.AwayGuess),
				Count: g.Total,
			})
		}
		if resp.GuessDistribution == nil {
			resp.GuessDistribution = []guessDistributionDTO{}
		}

		for _, a := range accuracy {
			if hiddenMap[a.UserID] {
				continue
			}
			rate := 0.0
			if a.Total > 0 {
				rate = float64(a.Exact+a.Partial) / float64(a.Total)
			}
			resp.AccuracyRanking = append(resp.AccuracyRanking, accuracyRankingDTO{
				UserID:    a.UserID,
				Name:      a.Name,
				AvatarURL: a.AvatarURL,
				Exact:     a.Exact,
				Partial:   a.Partial,
				Wrong:     a.Wrong,
				Total:     a.Total,
				Rate:      rate,
			})
		}
		if resp.AccuracyRanking == nil {
			resp.AccuracyRanking = []accuracyRankingDTO{}
		}

		return resp, nil
	})
}

// ==========================================
// Helpers
// ==========================================

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func normalizeRound(round string) string {
	lower := strings.ToLower(strings.TrimSpace(round))
	if v, ok := knockoutRoundMap[lower]; ok {
		return v
	}
	if strings.HasPrefix(lower, "round ") {
		number := strings.TrimSpace(strings.TrimPrefix(lower, "round "))
		if v, ok := knockoutRoundMap[number]; ok {
			return v
		}
	}
	return ""
}

func normalizeTeamName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "&", "and")
	name = strings.ReplaceAll(name, "ç", "c")
	name = strings.ReplaceAll(name, "ã", "a")
	name = strings.ReplaceAll(name, "ó", "o")
	name = strings.ReplaceAll(name, "ô", "o")
	name = strings.ReplaceAll(name, "é", "e")
	name = strings.ReplaceAll(name, "í", "i")
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var teamToGroupMap = map[string]string{
	"mexico":                       "Grupo A",
	"southafrica":                  "Grupo A",
	"southkorea":                   "Grupo A",
	"czechrepublic":                "Grupo A",
	"canada":                       "Grupo B",
	"bosniaherzegovina":            "Grupo B",
	"bosniaandherzegovina":         "Grupo B",
	"qatar":                        "Grupo B",
	"switzerland":                  "Grupo B",
	"brazil":                       "Grupo C",
	"morocco":                      "Grupo C",
	"haiti":                        "Grupo C",
	"scotland":                     "Grupo C",
	"usa":                          "Grupo D",
	"unitedstates":                 "Grupo D",
	"paraguay":                     "Grupo D",
	"australia":                    "Grupo D",
	"turkey":                       "Grupo D",
	"germany":                      "Grupo E",
	"curacao":                      "Grupo E",
	"ivorycoast":                   "Grupo E",
	"cotedivoire":                  "Grupo E",
	"ecuador":                      "Grupo E",
	"netherlands":                  "Grupo F",
	"japan":                        "Grupo F",
	"sweden":                       "Grupo F",
	"tunisia":                      "Grupo F",
	"belgium":                      "Grupo G",
	"iran":                         "Grupo G",
	"egypt":                        "Grupo G",
	"newzealand":                   "Grupo G",
	"spain":                        "Grupo H",
	"capeverde":                    "Grupo H",
	"saudiarabia":                  "Grupo H",
	"uruguay":                      "Grupo H",
	"france":                       "Grupo I",
	"senegal":                      "Grupo I",
	"iraq":                         "Grupo I",
	"norway":                       "Grupo I",
	"argentina":                    "Grupo J",
	"algeria":                      "Grupo J",
	"austria":                      "Grupo J",
	"jordan":                       "Grupo J",
	"portugal":                     "Grupo K",
	"drcongo":                      "Grupo K",
	"democraticrepublicofthecongo": "Grupo K",
	"uzbekistan":                   "Grupo K",
	"colombia":                     "Grupo K",
	"england":                      "Grupo L",
	"croatia":                      "Grupo L",
	"ghana":                        "Grupo L",
	"panama":                       "Grupo L",
}
