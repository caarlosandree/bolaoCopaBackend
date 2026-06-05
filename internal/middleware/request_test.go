package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/requestctx"

	"github.com/labstack/echo/v5"
)

func TestRequestIDGeneratesHeaderWhenAbsent(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := RequestID(func(c *echo.Context) error {
		if requestctx.RequestID(c) == "" {
			t.Fatal("expected request ID in echo context")
		}
		if requestctx.FromContext(c.Request().Context()) == "" {
			t.Fatal("expected request ID in request context")
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})(c)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Header().Get(requestctx.HeaderRequestID) == "" {
		t.Fatal("expected X-Request-Id response header")
	}
}

func TestRequestIDRespectsIncomingHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(requestctx.HeaderRequestID, "req-existing-123")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := RequestID(func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})(c)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rec.Header().Get(requestctx.HeaderRequestID); got != "req-existing-123" {
		t.Fatalf("expected incoming request ID, got %q", got)
	}
}
