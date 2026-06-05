package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
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

	if time.Now().UTC().After(mt.MatchTime.Add(-5 * time.Minute)) {
		return respond.Error(c, http.StatusForbidden, "palpites bloqueados 5 minutos antes do início")
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
	DB      *sql.DB
	Guesses *repositories.GuessRepository
	Matches *repositories.MatchRepository
	Audit   *audit.Service
}

func NewAdminHandler(db *sql.DB, guesses *repositories.GuessRepository, matches *repositories.MatchRepository, auditSvc *audit.Service) *AdminHandler {
	return &AdminHandler{DB: db, Guesses: guesses, Matches: matches, Audit: auditSvc}
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
	tx, err := h.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return respond.InternalError(c, "erro ao iniciar transação")
	}
	defer tx.Rollback()

	if err := h.Matches.UpdateScoreAndStatus(ctx, tx, matchID, req.HomeScore, req.AwayScore); err != nil {
		return respond.InternalError(c, "erro ao atualizar placar")
	}

	guesses, err := h.Guesses.FindByMatch(ctx, tx, matchID)
	if err != nil {
		return respond.InternalError(c, "erro ao buscar palpites")
	}

	pointsDeltaTotal := 0
	for _, g := range guesses {
		newPoints := services.CalculatePoints(g.HomeGuess, g.AwayGuess, req.HomeScore, req.AwayScore)

		oldPoints := 0
		if g.PointsEarned != nil {
			oldPoints = *g.PointsEarned
		}

		if err := h.Matches.UpdateGuessPoints(ctx, tx, g.ID, newPoints); err != nil {
			return respond.InternalError(c, "erro ao atualizar pontos do palpite")
		}

		delta := newPoints - oldPoints
		pointsDeltaTotal += delta
		if delta != 0 {
			if err := h.Matches.AdjustUserPoints(ctx, tx, g.UserID, delta); err != nil {
				return respond.InternalError(c, "erro ao atualizar pontuação do usuário")
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return respond.InternalError(c, "erro ao confirmar transação")
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
				"guesses_recalculated": len(guesses),
				"points_delta_total":   pointsDeltaTotal,
			},
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":  "placar atualizado e pontos calculados",
		"match_id": matchID,
		"score":    map[string]int{"home": req.HomeScore, "away": req.AwayScore},
	})
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
