package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"

	"backend/internal/audit"
	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/handlers"
	"backend/internal/logging"
	jwtmw "backend/internal/middleware"
	"backend/internal/migrations"
	"backend/internal/repositories"
	"backend/internal/respond"

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
	auditRepo := audit.NewRepository(database)
	auditSvc := audit.NewService(cfg.AuditEnabled, auditRepo, logger)
	auditSvc.CleanupExpired(context.Background(), cfg.AuditRetentionDays)

	authH := handlers.NewAuthHandler(userRepo, cfg.JWTSecret)
	roundH := handlers.NewRoundHandler(roundRepo)
	guessH := handlers.NewGuessHandler(guessRepo, matchRepo)
	rankH := handlers.NewRankingHandler(userRepo)
	adminH := handlers.NewAdminHandler(database, guessRepo, matchRepo, auditSvc)

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
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
	}))

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

	admin := api.Group("/admin")
	admin.Use(jwtmw.JWTAuth(cfg.JWTSecret))
	admin.Use(jwtmw.AdminOnly)
	admin.POST("/matches/:id/score", adminH.UpdateMatchScore)

	if err := e.Start(":1323"); err != nil {
		logger.Error("servidor encerrado", "error", err)
	}
}
