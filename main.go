package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"time"

	"backend/internal/audit"
	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/handlers"
	"backend/internal/logging"
	jwtmw "backend/internal/middleware"
	"backend/internal/migrations"
	"backend/internal/repositories"
	"backend/internal/respond"
	"backend/internal/services"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("erro ao carregar configuração: %v", err)
	}
	logger := logging.New(logging.Config{
		Env:    cfg.AppEnv,
		Level:  cfg.LogLevel,
		Format: cfg.LogFormat,
	})
	slog.SetDefault(logger)

	database, err := db.Connect(cfg.DSN())
	if err != nil {
		logger.Error("erro ao conectar no banco", "error", err)
		return
	}
	defer database.Close()

	if err := migrations.Run(context.Background(), database); err != nil {
		logger.Error("erro ao aplicar migrations", "error", err)
		return
	}

	userRepo := repositories.NewUserRepository(database)
	roundRepo := repositories.NewRoundRepository(database)
	guessRepo := repositories.NewGuessRepository(database)
	matchRepo := repositories.NewMatchRepository(database)
	matchDetailsRepo := repositories.NewMatchDetailsRepository(database)
	auditRepo := audit.NewRepository(database)
	auditSvc := audit.NewService(cfg.AuditEnabled, auditRepo, logger)
	auditSvc.CleanupExpired(context.Background(), cfg.AuditRetentionDays)
	scoreSvc := services.NewMatchScoreService(database, guessRepo, matchRepo)

	authH := handlers.NewAuthHandler(userRepo, cfg.JWTSecret)
	roundH := handlers.NewRoundHandler(roundRepo)
	guessH := handlers.NewGuessHandler(guessRepo, matchRepo)
	rankH := handlers.NewRankingHandler(userRepo)
	adminH := handlers.NewAdminHandler(guessRepo, matchRepo, scoreSvc, auditSvc, userRepo)
	userH := handlers.NewUserHandler(userRepo, cfg.BaseURL)
	matchDetailsH := handlers.NewMatchDetailsHandler(matchDetailsRepo)

	matchSync := services.NewMatchSyncService(database, matchRepo, scoreSvc, logger, cfg.OpenFootballURL, cfg.WorldCup26BaseURL)
	theSportsDBClient := services.NewTheSportsDBClient(cfg.TheSportsDBBaseURL, cfg.TheSportsDBAPIKey)
	oddsClient := services.NewOddsAPIClient(cfg.OddsAPIBaseURL, cfg.OddsAPIKey)
	matchDetailsSync := services.NewMatchDetailsSyncService(
		database,
		matchRepo,
		matchDetailsRepo,
		theSportsDBClient,
		oddsClient,
		logger,
		cfg.TheSportsDBLeagueID,
		cfg.TheSportsDBSeason,
		time.Duration(cfg.MatchDetailsLineupMinutes)*time.Minute,
		time.Duration(cfg.MatchDetailsDailyHours)*time.Hour,
	)
	syncH := handlers.NewSyncHandler(matchSync, matchDetailsSync, matchRepo)

	if cfg.MatchSyncEnabled {
		matchSync.Start(
			context.Background(),
			time.Duration(cfg.MatchResultRetryMinutes)*time.Minute,
			time.Duration(cfg.MatchResultCheckAfterMinutes)*time.Minute,
		)
	}
	if cfg.MatchDetailsSyncEnabled {
		matchDetailsSync.Start(context.Background())
	}

	e := echo.New()
	e.Logger = logger
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		status := http.StatusInternalServerError
		message := http.StatusText(status)

		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			status = httpErr.Code
			message = http.StatusText(status)
			if status < http.StatusInternalServerError && httpErr.Message != "" {
				message = fmt.Sprint(httpErr.Message)
			}
		}

		if response, ok := c.Response().(*echo.Response); ok && response.Committed {
			return
		}
		_ = respond.Error(c, status, message)
	}
	e.Pre(jwtmw.RequestID)
	e.Use(jwtmw.Observability(jwtmw.ObservabilityConfig{
		Logger: logger,
		Audit:  auditSvc,
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:3000"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch},
	}))

	// Servir avatares enviados pelos usuários
	e.Static("/uploads", "uploads")

	e.GET("/health", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	api := e.Group("/api")

	api.POST("/auth/register", authH.Register)
	api.POST("/auth/login", authH.Login)

	api.GET("/ranking", rankH.GetRanking)

	protected := api.Group("")
	protected.Use(jwtmw.JWTAuth(cfg.JWTSecret))
	protected.GET("/rounds/active", roundH.GetActiveRound)
	protected.POST("/guesses", guessH.SaveGuess)
	protected.GET("/matches/:id/details", matchDetailsH.GetByMatchID)
	protected.GET("/me", userH.GetMe)
	protected.POST("/me/avatar", userH.UploadAvatar)

	admin := api.Group("/admin")
	admin.Use(jwtmw.JWTAuth(cfg.JWTSecret))
	admin.Use(jwtmw.AdminOnly)
	admin.GET("/users", adminH.GetUsers)
	admin.GET("/rounds", roundH.ListAll)
	admin.POST("/matches/:id/score", adminH.UpdateMatchScore)
	admin.POST("/sync/schedule", syncH.SyncSchedule)
	admin.POST("/sync/results", syncH.SyncResults)
	admin.POST("/sync/match-details", syncH.SyncMatchDetails)
	admin.POST("/sync/matches/:id/details", syncH.SyncOneMatchDetails)
	admin.GET("/matches/recent", syncH.ListRecentMatches)

	if err := e.Start(":1323"); err != nil {
		logger.Error("servidor encerrado", "error", err)
	}
}
