package respond

import (
	"net/http"

	"backend/internal/requestctx"

	"github.com/labstack/echo/v5"
)

type ErrorResponse struct {
	Error     string `json:"error"`
	RequestID string `json:"request_id"`
}

func Error(c *echo.Context, status int, message string) error {
	requestID := requestctx.RequestID(c)
	if requestID == "" {
		requestID = requestctx.NormalizeOrNew("")
	}
	return c.JSON(status, ErrorResponse{
		Error:     message,
		RequestID: requestID,
	})
}

func InternalError(c *echo.Context, message string) error {
	return Error(c, http.StatusInternalServerError, message)
}
