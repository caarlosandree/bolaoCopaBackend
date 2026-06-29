package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"backend/internal/audit"
	"backend/internal/requestctx"

	"github.com/labstack/echo/v5"
)

func RequestID(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		requestID := requestctx.NormalizeOrNew(c.Request().Header.Get(requestctx.HeaderRequestID))
		c.Set("requestID", requestID)
		c.Response().Header().Set(requestctx.HeaderRequestID, requestID)

		req := c.Request()
		ctx := requestctx.WithRequestID(req.Context(), requestID)
		c.SetRequest(req.WithContext(ctx))

		return next(c)
	}
}

type ObservabilityConfig struct {
	Logger *slog.Logger
	Audit  *audit.Service
}

func Observability(cfg ObservabilityConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			err := next(c)
			if err != nil {
				c.Echo().HTTPErrorHandler(c, err)
			}

			status := responseStatus(c)
			duration := time.Since(start)
			requestID := requestctx.RequestID(c)
			route := c.Path()
			if route == "" {
				route = c.Request().URL.Path
			}
			action := actionFor(c.Request().Method, route)

			attrs := []any{
				"request_id", requestID,
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"route", route,
				"status", status,
				"duration_ms", duration.Milliseconds(),
				"remote_ip", c.RealIP(),
				"user_agent", c.Request().UserAgent(),
				"operation", action,
			}

			if userID, ok := c.Get("userID").(int); ok {
				attrs = append(attrs, "user_id", userID)
			}
			if role, ok := c.Get("role").(string); ok && role != "" {
				attrs = append(attrs, "role", role)
			}

			if cfg.Logger != nil {
				logRequest(c.Request().Context(), cfg.Logger, status, attrs...)
			}

			if cfg.Audit != nil && shouldAudit(route) {
				cfg.Audit.Record(c.Request().Context(), audit.Event{
					RequestID:    requestID,
					ActorUserID:  actorUserID(c),
					ActorRole:    actorRole(c),
					Action:       action,
					ResourceType: resourceType(route),
					ResourceID:   resourceID(c, route),
					Method:       c.Request().Method,
					Path:         c.Request().URL.Path,
					StatusCode:   status,
					Outcome:      audit.OutcomeFromStatus(status),
					IP:           c.RealIP(),
					UserAgent:    c.Request().UserAgent(),
					Metadata: map[string]any{
						"duration_ms": duration.Milliseconds(),
						"route":       route,
					},
				})
			}

			return nil
		}
	}
}

func logRequest(ctx context.Context, logger *slog.Logger, status int, attrs ...any) {
	switch {
	case status >= 500:
		logger.ErrorContext(ctx, "request completed", attrs...)
	case status >= 400:
		logger.WarnContext(ctx, "request completed", attrs...)
	default:
		logger.InfoContext(ctx, "request completed", attrs...)
	}
}

func responseStatus(c *echo.Context) int {
	if response, ok := c.Response().(*echo.Response); ok && response.Status != 0 {
		return response.Status
	}
	return http.StatusOK
}

func shouldAudit(route string) bool {
	return route != "/health"
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

func actionFor(method, route string) string {
	switch method + " " + route {
	case http.MethodPost + " /api/auth/register":
		return "auth.register"
	case http.MethodPost + " /api/auth/login":
		return "auth.login"
	case http.MethodGet + " /api/ranking":
		return "ranking.list"
	case http.MethodPatch + " /api/admin/users/:id/hidden":
		return "admin.user.hidden.update"
	case http.MethodGet + " /api/rounds/active":
		return "round.active.get"
	case http.MethodPost + " /api/guesses":
		return "guess.save"
	case http.MethodPost + " /api/admin/matches/:id/score":
		return "admin.match_score.update"
	case http.MethodPost + " /api/admin/matches/:id/knockout":
		return "admin.match_knockout.update"
	default:
		return strings.ToLower(method) + "." + strings.Trim(strings.ReplaceAll(route, "/", "."), ".")
	}
}

func resourceType(route string) string {
	if strings.Contains(route, "/matches/") {
		return "match"
	}
	if strings.Contains(route, "/guesses") {
		return "guess"
	}
	if strings.Contains(route, "/rounds") {
		return "round"
	}
	if strings.Contains(route, "/ranking") {
		return "ranking"
	}
	if strings.Contains(route, "/auth") {
		return "auth"
	}
	return ""
}

func resourceID(c *echo.Context, route string) string {
	if strings.Contains(route, "/matches/:id") {
		return c.Param("id")
	}
	if strings.Contains(route, "/guesses") && c.Request().Method == http.MethodPost {
		return ""
	}
	return ""
}
