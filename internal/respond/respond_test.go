package respond_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/middleware"
	"backend/internal/requestctx"
	"backend/internal/respond"

	"github.com/labstack/echo/v5"
)

func TestErrorIncludesRequestID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set(requestctx.HeaderRequestID, "req-test-123")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := middleware.RequestID(func(c *echo.Context) error {
		return respond.Error(c, http.StatusBadRequest, "dados inválidos")
	})(c)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var body respond.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.RequestID != "req-test-123" {
		t.Fatalf("expected request ID %q, got %q", "req-test-123", body.RequestID)
	}
}
