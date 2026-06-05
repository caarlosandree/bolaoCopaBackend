package main

import (
	"context"
	"log"
	"net/http"

	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/handlers"
	jwtmw "backend/internal/middleware"
	"backend/internal/migrations"
	"backend/internal/repositories"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("erro ao carregar configuração: %v", err)
	}

	database, err := db.Connect(cfg.DSN())
	if err != nil {
		log.Fatalf("erro ao conectar no banco: %v", err)
	}
	defer database.Close()

	if err := migrations.Run(context.Background(), database); err != nil {
		log.Fatalf("erro ao aplicar migrations: %v", err)
	}

	userRepo := repositories.NewUserRepository(database)
	roundRepo := repositories.NewRoundRepository(database)
	guessRepo := repositories.NewGuessRepository(database)
	matchRepo := repositories.NewMatchRepository(database)

	authH := handlers.NewAuthHandler(userRepo, cfg.JWTSecret)
	roundH := handlers.NewRoundHandler(roundRepo)
	guessH := handlers.NewGuessHandler(guessRepo, matchRepo)
	rankH := handlers.NewRankingHandler(userRepo)
	adminH := handlers.NewAdminHandler(database, guessRepo, matchRepo)

	e := echo.New()
	e.Use(middleware.RequestLogger())
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
		e.Logger.Error("servidor encerrado", "error", err)
	}
}
