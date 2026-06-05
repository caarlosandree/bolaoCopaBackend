package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
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

	statsRepo := repositories.NewStatisticsRepository(database)
	authH := handlers.NewAuthHandler(userRepo, cfg.JWTSecret)
	roundH := handlers.NewRoundHandler(roundRepo)
	guessH := handlers.NewGuessHandler(guessRepo, matchRepo)
	rankH := handlers.NewRankingHandler(userRepo)
	adminH := handlers.NewAdminHandler(guessRepo, matchRepo, scoreSvc, auditSvc, userRepo)
	userH := handlers.NewUserHandler(userRepo, cfg.BaseURL)
	matchDetailsH := handlers.NewMatchDetailsHandler(matchDetailsRepo)

	theSportsDBClient := services.NewTheSportsDBClient(cfg.TheSportsDBBaseURL, cfg.TheSportsDBAPIKey)
	matchSync := services.NewMatchSyncService(database, matchRepo, roundRepo, scoreSvc, theSportsDBClient, cfg.TheSportsDBLeagueID, cfg.TheSportsDBSeason, logger, cfg.WorldCup26BaseURL)
	matchDetailsSync := services.NewMatchDetailsSyncService(
		database,
		matchRepo,
		matchDetailsRepo,
		theSportsDBClient,
		logger,
		cfg.TheSportsDBLeagueID,
		cfg.TheSportsDBSeason,
		time.Duration(cfg.MatchDetailsLineupMinutes)*time.Minute,
		time.Duration(cfg.MatchDetailsDailyHours)*time.Hour,
	)
	syncH := handlers.NewSyncHandler(matchSync, matchDetailsSync, matchRepo, auditSvc)
	statsH := handlers.NewStatisticsHandler(statsRepo, theSportsDBClient, cfg.TheSportsDBLeagueID, cfg.TheSportsDBSeason)

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
	corsOrigins := []string{"http://localhost:3000"}
	if v := os.Getenv("CORS_ALLOWED_ORIGINS"); v != "" {
		corsOrigins = strings.Split(v, ",")
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: corsOrigins,
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
	protected.GET("/rounds", roundH.ListSummary)
	protected.GET("/rounds/active", roundH.GetActiveRound)
	protected.GET("/rounds/:id", roundH.GetByID)
	protected.POST("/guesses", guessH.SaveGuess)
	protected.GET("/matches/:id/details", matchDetailsH.GetByMatchID)
	protected.GET("/me", userH.GetMe)
	protected.POST("/me/avatar", userH.UploadAvatar)
	protected.GET("/statistics/copa", statsH.GetCopaOverview)
	protected.GET("/statistics/groups", statsH.GetGroupStandings)
	protected.GET("/statistics/bracket", statsH.GetBracket)
	protected.GET("/statistics/bolao", statsH.GetBolaoStats)

	admin := api.Group("/admin")
	admin.Use(jwtmw.JWTAuth(cfg.JWTSecret))
	admin.Use(jwtmw.AdminOnly)
	admin.GET("/users", adminH.GetUsers)
	admin.GET("/rounds", roundH.ListAll)
	admin.POST("/rounds/:id/activate", roundH.Activate)
	admin.POST("/matches/:id/score", adminH.UpdateMatchScore)
	admin.POST("/sync/schedule", syncH.SyncSchedule)
	admin.POST("/sync/results", syncH.SyncResults)
	admin.POST("/sync/reset-schedule", syncH.ResetSchedule)
	admin.POST("/sync/match-details", syncH.SyncMatchDetails)
	admin.POST("/sync/matches/:id/details", syncH.SyncOneMatchDetails)
	admin.GET("/sync/logs", syncH.ListSyncLogs)
	admin.GET("/matches", syncH.ListMatches)

	port := os.Getenv("PORT")
	if port == "" {
		port = "1323"
	}
	if err := e.Start(":" + port); err != nil {
		logger.Error("servidor encerrado", "error", err)
	}
}
