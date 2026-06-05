package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/models"
	"backend/internal/repositories"
	"backend/internal/requestctx"
	"backend/internal/respond"
	"backend/internal/services"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

// ==========================================
// 1. AUTH HANDLERS
// ==========================================

type AuthHandler struct {
	Users     *repositories.UserRepository
	JWTSecret string
}

func NewAuthHandler(users *repositories.UserRepository, jwtSecret string) *AuthHandler {
	return &AuthHandler{Users: users, JWTSecret: jwtSecret}
}

func (h *AuthHandler) generateToken(userID int, role string) (string, error) {
	claims := middleware.Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.JWTSecret))
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Register(c *echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil || req.Name == "" || req.Email == "" || len(req.Password) < 6 {
		return respond.Error(c, http.StatusBadRequest, "dados inválidos (senha mínimo 6 caracteres)")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return respond.InternalError(c, "erro interno")
	}

	user, err := h.Users.Create(c.Request().Context(), req.Name, req.Email, string(hash))
	if err != nil {
		return respond.Error(c, http.StatusConflict, "e-mail já cadastrado")
	}

	token, err := h.generateToken(user.ID, user.Role)
	if err != nil {
		return respond.InternalError(c, "erro ao gerar token")
	}

	c.Set("userID", user.ID)
	c.Set("role", user.Role)
	return c.JSON(http.StatusCreated, map[string]interface{}{"token": token, "user": user})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return respond.Error(c, http.StatusBadRequest, "dados inválidos")
	}

	user, err := h.Users.FindByEmail(c.Request().Context(), req.Email)
	if err != nil {
		return respond.Error(c, http.StatusUnauthorized, "credenciais inválidas")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return respond.Error(c, http.StatusUnauthorized, "credenciais inválidas")
	}

	token, err := h.generateToken(user.ID, user.Role)
	if err != nil {
		return respond.InternalError(c, "erro ao gerar token")
	}

	c.Set("userID", user.ID)
	c.Set("role", user.Role)
	user.PasswordHash = ""
	return c.JSON(http.StatusOK, map[string]interface{}{"token": token, "user": user})
}

// ==========================================
// 2. ROUND HANDLER
// ==========================================

type RoundHandler struct {
	Rounds *repositories.RoundRepository
}

func NewRoundHandler(rounds *repositories.RoundRepository) *RoundHandler {
	return &RoundHandler{Rounds: rounds}
}

func (h *RoundHandler) GetActiveRound(c *echo.Context) error {
	userID, _ := c.Get("userID").(int)

	round, err := h.Rounds.FindActive(c.Request().Context())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusOK, map[string]interface{}{"round": nil, "matches": []models.Match{}})
		}
		return respond.InternalError(c, "erro ao buscar rodada")
	}

	matches, err := h.Rounds.FindMatchesByRound(c.Request().Context(), round.ID, userID)
	if err != nil {
		return respond.InternalError(c, "erro ao buscar partidas")
	}
	if matches == nil {
		matches = []models.Match{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"round": round, "matches": matches})
}

func (h *RoundHandler) ListAll(c *echo.Context) error {
	rounds, err := h.Rounds.ListSummary(c.Request().Context())
	if err != nil {
		return respond.InternalError(c, "erro ao buscar rodadas")
	}
	if rounds == nil {
		rounds = []models.RoundSummary{}
	}
	return c.JSON(http.StatusOK, rounds)
}

func (h *RoundHandler) ListSummary(c *echo.Context) error {
	rounds, err := h.Rounds.ListSummary(c.Request().Context())
	if err != nil {
		return respond.InternalError(c, "erro ao buscar rodadas")
	}
	if rounds == nil {
		rounds = []models.RoundSummary{}
	}
	return c.JSON(http.StatusOK, rounds)
}

func (h *RoundHandler) GetByID(c *echo.Context) error {
	userID, _ := c.Get("userID").(int)

	roundID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return respond.Error(c, http.StatusBadRequest, "ID de rodada inválido")
	}

	round, err := h.Rounds.FindByID(c.Request().Context(), roundID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return respond.Error(c, http.StatusNotFound, "rodada não encontrada")
		}
		return respond.InternalError(c, "erro ao buscar rodada")
	}

	matches, err := h.Rounds.FindMatchesByRound(c.Request().Context(), round.ID, userID)
	if err != nil {
		return respond.InternalError(c, "erro ao buscar partidas")
	}
	if matches == nil {
		matches = []models.Match{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"round": round, "matches": matches})
}

func (h *RoundHandler) Activate(c *echo.Context) error {
	roundID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return respond.Error(c, http.StatusBadRequest, "ID de rodada inválido")
	}
	if err := h.Rounds.SetStatus(c.Request().Context(), roundID, "active"); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return respond.Error(c, http.StatusNotFound, "rodada não encontrada")
		}
		return respond.InternalError(c, "erro ao ativar rodada")
	}
	return c.JSON(http.StatusOK, map[string]any{"message": "rodada ativada", "round_id": roundID})
}

// ==========================================
// 3. GUESS HANDLER
// ==========================================

type GuessHandler struct {
	Guesses *repositories.GuessRepository
	Matches *repositories.MatchRepository
}

func NewGuessHandler(guesses *repositories.GuessRepository, matches *repositories.MatchRepository) *GuessHandler {
	return &GuessHandler{Guesses: guesses, Matches: matches}
}

type saveGuessRequest struct {
	MatchID   int `json:"match_id"`
	HomeGuess int `json:"home_guess"`
	AwayGuess int `json:"away_guess"`
}

func (h *GuessHandler) SaveGuess(c *echo.Context) error {
	userID, _ := c.Get("userID").(int)

	var req saveGuessRequest
	if err := c.Bind(&req); err != nil || req.MatchID == 0 {
		return respond.Error(c, http.StatusBadRequest, "dados do palpite inválidos")
	}

	mt, err := h.Matches.FindMatchTime(c.Request().Context(), req.MatchID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return respond.Error(c, http.StatusNotFound, "partida não encontrada")
		}
		return respond.InternalError(c, "erro interno")
	}

	if mt.Status == "finished" || mt.Status == "ongoing" {
		return respond.Error(c, http.StatusForbidden, "partida já em andamento ou finalizada")
	}

	if time.Now().UTC().After(mt.MatchTime.Add(-10 * time.Minute)) {
		return respond.Error(c, http.StatusForbidden, "palpites bloqueados 10 minutos antes do início")
	}

	if err := h.Guesses.Upsert(c.Request().Context(), userID, req.MatchID, req.HomeGuess, req.AwayGuess); err != nil {
		return respond.InternalError(c, "erro ao salvar palpite")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "palpite salvo com sucesso"})
}

// ==========================================
// 4. RANKING HANDLER
// ==========================================

type RankingHandler struct {
	Users *repositories.UserRepository
}

func NewRankingHandler(users *repositories.UserRepository) *RankingHandler {
	return &RankingHandler{Users: users}
}

func (h *RankingHandler) GetRanking(c *echo.Context) error {
	ranking, err := h.Users.GetRanking(c.Request().Context())
	if err != nil {
		return respond.InternalError(c, "erro ao buscar ranking")
	}
	if ranking == nil {
		ranking = []models.Ranking{}
	}
	return c.JSON(http.StatusOK, ranking)
}

// ==========================================
// 5. ADMIN HANDLER
// ==========================================

type AdminHandler struct {
	Guesses *repositories.GuessRepository
	Matches *repositories.MatchRepository
	Score   *services.MatchScoreService
	Audit   *audit.Service
	Users   *repositories.UserRepository
}

func NewAdminHandler(guesses *repositories.GuessRepository, matches *repositories.MatchRepository, score *services.MatchScoreService, auditSvc *audit.Service, users *repositories.UserRepository) *AdminHandler {
	return &AdminHandler{Guesses: guesses, Matches: matches, Score: score, Audit: auditSvc, Users: users}
}

type updateScoreRequest struct {
	HomeScore int `json:"home_score"`
	AwayScore int `json:"away_score"`
}

func (h *AdminHandler) UpdateMatchScore(c *echo.Context) error {
	matchID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return respond.Error(c, http.StatusBadRequest, "ID de partida inválido")
	}

	var req updateScoreRequest
	if err := c.Bind(&req); err != nil {
		return respond.Error(c, http.StatusBadRequest, "placar inválido")
	}

	ctx := c.Request().Context()
	result, err := h.Score.UpdateFinalScore(ctx, matchID, req.HomeScore, req.AwayScore)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return respond.Error(c, http.StatusNotFound, "partida não encontrada")
		}
		return respond.InternalError(c, "erro ao atualizar placar")
	}

	if h.Audit != nil {
		h.Audit.Record(ctx, audit.Event{
			RequestID:    requestctx.RequestID(c),
			ActorUserID:  actorUserID(c),
			ActorRole:    actorRole(c),
			Action:       "admin.match_score.updated",
			ResourceType: "match",
			ResourceID:   strconv.Itoa(matchID),
			Method:       c.Request().Method,
			Path:         c.Request().URL.Path,
			StatusCode:   http.StatusOK,
			Outcome:      "success",
			IP:           c.RealIP(),
			UserAgent:    c.Request().UserAgent(),
			Metadata: map[string]any{
				"changed":              result.Changed,
				"guesses_recalculated": result.GuessesRecalculated,
				"points_delta_total":   result.PointsDeltaTotal,
			},
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":  "placar atualizado e pontos calculados",
		"match_id": matchID,
		"score":    map[string]int{"home": req.HomeScore, "away": req.AwayScore},
	})
}

func (h *AdminHandler) GetUsers(c *echo.Context) error {
	users, err := h.Users.ListAll(c.Request().Context())
	if err != nil {
		return respond.InternalError(c, "erro ao buscar usuários")
	}
	if users == nil {
		users = []models.User{}
	}
	return c.JSON(http.StatusOK, users)
}

// ==========================================
// 6. SYNC HANDLER
// ==========================================

type SyncHandler struct {
	Sync        *services.MatchSyncService
	DetailsSync *services.MatchDetailsSyncService
	Matches     *repositories.MatchRepository
	Audit       *audit.Service
}

func NewSyncHandler(sync *services.MatchSyncService, detailsSync *services.MatchDetailsSyncService, matches *repositories.MatchRepository, auditSvc *audit.Service) *SyncHandler {
	return &SyncHandler{Sync: sync, DetailsSync: detailsSync, Matches: matches, Audit: auditSvc}
}

func (h *SyncHandler) SyncSchedule(c *echo.Context) error {
	imported, err := h.Sync.SyncSchedule(c.Request().Context())
	if err != nil {
		h.recordSyncAudit(c, "admin.sync.schedule", http.StatusBadGateway, map[string]any{"error": err.Error()})
		return respond.Error(c, http.StatusBadGateway, "falha ao sincronizar calendário: "+err.Error())
	}
	h.recordSyncAudit(c, "admin.sync.schedule", http.StatusOK, map[string]any{"imported": imported})
	return c.JSON(http.StatusOK, map[string]any{
		"message":  "calendário sincronizado",
		"imported": imported,
	})
}

func (h *SyncHandler) SyncResults(c *echo.Context) error {
	summary, err := h.Sync.SyncResults(c.Request().Context())
	if err != nil {
		h.recordSyncAudit(c, "admin.sync.results", http.StatusBadGateway, map[string]any{"error": err.Error()})
		return respond.Error(c, http.StatusBadGateway, "falha ao sincronizar resultados: "+err.Error())
	}
	h.recordSyncAudit(c, "admin.sync.results", http.StatusOK, map[string]any{
		"linked":         summary.Linked,
		"scores_updated": summary.ScoresUpdated,
		"scores_skipped": summary.ScoresSkipped,
	})
	return c.JSON(http.StatusOK, map[string]any{
		"message":        "resultados sincronizados",
		"linked":         summary.Linked,
		"scores_updated": summary.ScoresUpdated,
		"scores_skipped": summary.ScoresSkipped,
	})
}

func (h *SyncHandler) ListMatches(c *echo.Context) error {
	page := 1
	if v := c.QueryParam("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	pageSize := 12
	if v := c.QueryParam("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			pageSize = n
		}
	}
	matches, err := h.Matches.ListAdminPage(c.Request().Context(), page, pageSize)
	if err != nil {
		return respond.InternalError(c, "erro ao buscar partidas")
	}
	return c.JSON(http.StatusOK, matches)
}

func (h *SyncHandler) SyncMatchDetails(c *echo.Context) error {
	if h.DetailsSync == nil {
		return respond.Error(c, http.StatusServiceUnavailable, "sync de detalhes não configurado")
	}
	summary, err := h.DetailsSync.SyncAll(c.Request().Context())
	if err != nil {
		h.recordSyncAudit(c, "admin.sync.match_details", http.StatusBadGateway, map[string]any{"error": err.Error()})
		return respond.Error(c, http.StatusBadGateway, "falha ao sincronizar detalhes: "+err.Error())
	}
	h.recordSyncAudit(c, "admin.sync.match_details", http.StatusOK, map[string]any{
		"schedule_imported": summary.ScheduleImported,
		"details_updated":   summary.DetailsUpdated,
		"odds_linked":       summary.OddsLinked,
		"failures":          len(summary.Failures),
	})
	return c.JSON(http.StatusOK, map[string]any{
		"message":           "detalhes das partidas sincronizados",
		"schedule_imported": summary.ScheduleImported,
		"details_updated":   summary.DetailsUpdated,
		"odds_linked":       summary.OddsLinked,
		"failures":          summary.Failures,
	})
}

func (h *SyncHandler) SyncOneMatchDetails(c *echo.Context) error {
	if h.DetailsSync == nil {
		return respond.Error(c, http.StatusServiceUnavailable, "sync de detalhes não configurado")
	}
	matchID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return respond.Error(c, http.StatusBadRequest, "ID de partida inválido")
	}
	if err := h.DetailsSync.SyncMatchByID(c.Request().Context(), matchID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return respond.Error(c, http.StatusNotFound, "partida não encontrada")
		}
		return respond.Error(c, http.StatusBadGateway, "falha ao sincronizar detalhes: "+err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{
		"message":  "detalhes da partida sincronizados",
		"match_id": matchID,
	})
}

func (h *SyncHandler) ResetSchedule(c *echo.Context) error {
	ctx := c.Request().Context()
	if err := h.Matches.ResetSchedule(ctx); err != nil {
		h.recordSyncAudit(c, "admin.sync.reset_schedule", http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return respond.InternalError(c, "falha ao resetar calendário: "+err.Error())
	}
	imported, err := h.Sync.SyncSchedule(ctx)
	if err != nil {
		h.recordSyncAudit(c, "admin.sync.reset_schedule", http.StatusBadGateway, map[string]any{"error": err.Error()})
		return respond.Error(c, http.StatusBadGateway, "reset ok, mas reimportação falhou: "+err.Error())
	}
	detailsSummary := map[string]any{"skipped": true}
	if h.DetailsSync != nil {
		summary, detailsErr := h.DetailsSync.SyncAll(ctx)
		if detailsErr == nil {
			detailsSummary = map[string]any{
				"schedule_imported": summary.ScheduleImported,
				"details_updated":   summary.DetailsUpdated,
				"odds_linked":       summary.OddsLinked,
				"failures":          len(summary.Failures),
			}
		}
	}
	h.recordSyncAudit(c, "admin.sync.reset_schedule", http.StatusOK, map[string]any{
		"imported": imported,
		"details":  detailsSummary,
	})
	return c.JSON(http.StatusOK, map[string]any{
		"message":  "calendário resetado e reimportado",
		"imported": imported,
		"details":  detailsSummary,
	})
}

func (h *SyncHandler) ListSyncLogs(c *echo.Context) error {
	if h.Audit == nil || h.Audit.Repo == nil {
		return c.JSON(http.StatusOK, []audit.LogEntry{})
	}

	limit := 30
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	var actions []string
	switch c.QueryParam("scope") {
	case "results":
		actions = []string{"admin.sync.results"}
	default:
		actions = []string{
			"admin.sync.schedule",
			"admin.sync.reset_schedule",
			"admin.sync.match_details",
		}
	}

	logs, err := h.Audit.Repo.ListByActions(c.Request().Context(), actions, limit)
	if err != nil {
		return respond.InternalError(c, "erro ao buscar histórico de cargas")
	}
	return c.JSON(http.StatusOK, logs)
}

func (h *SyncHandler) recordSyncAudit(c *echo.Context, action string, status int, metadata map[string]any) {
	if h.Audit == nil {
		return
	}
	h.Audit.Record(c.Request().Context(), audit.Event{
		RequestID:    requestctx.RequestID(c),
		ActorUserID:  actorUserID(c),
		ActorRole:    actorRole(c),
		Action:       action,
		ResourceType: "sync",
		Method:       c.Request().Method,
		Path:         c.Request().URL.Path,
		StatusCode:   status,
		Outcome:      audit.OutcomeFromStatus(status),
		IP:           c.RealIP(),
		UserAgent:    c.Request().UserAgent(),
		Metadata:     metadata,
	})
}

// ==========================================
// 7. USER HANDLER (perfil / avatar)
// ==========================================

type MatchDetailsHandler struct {
	Details *repositories.MatchDetailsRepository
}

func NewMatchDetailsHandler(details *repositories.MatchDetailsRepository) *MatchDetailsHandler {
	return &MatchDetailsHandler{Details: details}
}

func (h *MatchDetailsHandler) GetByMatchID(c *echo.Context) error {
	matchID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return respond.Error(c, http.StatusBadRequest, "ID de partida inválido")
	}
	details, err := h.Details.FindByMatchID(c.Request().Context(), matchID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusOK, map[string]any{
				"match_id": matchID,
				"availability": map[string]bool{
					"odds":        false,
					"predictions": false,
					"form":        false,
					"h2h":         false,
					"lineups":     false,
					"statistics":  false,
					"injuries":    false,
					"events":      false,
					"media":       false,
				},
				"odds":              nil,
				"predictions":       nil,
				"recent_form":       nil,
				"head_to_head":      nil,
				"lineups":           nil,
				"statistics":        nil,
				"injuries":          nil,
				"events":            nil,
				"media":             nil,
				"source_status":     []models.SourceStatus{},
				"last_synced_at":    nil,
				"lineups_synced_at": nil,
				"updated_at":        nil,
			})
		}
		return respond.InternalError(c, "erro ao buscar detalhes da partida")
	}
	return c.JSON(http.StatusOK, details.Response())
}

const (
	maxAvatarSize = 5 << 20 // 5 MB
	uploadsDir    = "uploads/avatars"
)

var allowedAvatarTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

type UserHandler struct {
	Users   *repositories.UserRepository
	BaseURL string
}

func NewUserHandler(users *repositories.UserRepository, baseURL string) *UserHandler {
	return &UserHandler{Users: users, BaseURL: baseURL}
}

func (h *UserHandler) GetMe(c *echo.Context) error {
	userID, _ := c.Get("userID").(int)
	user, err := h.Users.FindByID(c.Request().Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return respond.Error(c, http.StatusNotFound, "usuário não encontrado")
		}
		return respond.InternalError(c, "erro interno")
	}
	user.PasswordHash = ""
	return c.JSON(http.StatusOK, user)
}

func (h *UserHandler) UploadAvatar(c *echo.Context) error {
	userID, _ := c.Get("userID").(int)

	if err := c.Request().ParseMultipartForm(maxAvatarSize); err != nil {
		return respond.Error(c, http.StatusBadRequest, "arquivo muito grande (máx 5 MB)")
	}

	file, header, err := c.Request().FormFile("avatar")
	if err != nil {
		return respond.Error(c, http.StatusBadRequest, "campo 'avatar' obrigatório")
	}
	defer file.Close()

	// Detecta o Content-Type pelo cabeçalho MIME
	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedAvatarTypes[contentType]
	if !ok {
		return respond.Error(c, http.StatusUnprocessableEntity, "formato não suportado (jpeg, png ou webp)")
	}

	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		return respond.InternalError(c, "erro ao criar diretório de uploads")
	}

	filename := fmt.Sprintf("%d%s", userID, ext)
	destPath := filepath.Join(uploadsDir, filename)

	dst, err := os.Create(destPath)
	if err != nil {
		return respond.InternalError(c, "erro ao salvar arquivo")
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return respond.InternalError(c, "erro ao salvar arquivo")
	}

	avatarURL := strings.TrimRight(h.BaseURL, "/") + "/uploads/avatars/" + filename
	if err := h.Users.UpdateAvatarURL(c.Request().Context(), userID, avatarURL); err != nil {
		return respond.InternalError(c, "erro ao atualizar avatar")
	}

	return c.JSON(http.StatusOK, map[string]string{"avatar_url": avatarURL})
}

func actorUserID(c *echo.Context) *int {
	userID, ok := c.Get("userID").(int)
	if !ok {
		return nil
	}
	return &userID
}

func actorRole(c *echo.Context) string {
	role, _ := c.Get("role").(string)
	return role
}
