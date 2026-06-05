package middleware

import (
	"net/http"
	"strings"

	"backend/internal/respond"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
)

type Claims struct {
	UserID int    `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func JWTAuth(secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				return respond.Error(c, http.StatusUnauthorized, "token ausente")
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				return respond.Error(c, http.StatusUnauthorized, "token inválido")
			}

			c.Set("userID", claims.UserID)
			c.Set("role", claims.Role)
			return next(c)
		}
	}
}

func AdminOnly(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		role, _ := c.Get("role").(string)
		if role != "admin" {
			return respond.Error(c, http.StatusForbidden, "acesso restrito a administradores")
		}
		return next(c)
	}
}
