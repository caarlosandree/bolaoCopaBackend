package requestctx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/labstack/echo/v5"
)

const (
	HeaderRequestID = "X-Request-Id"
	echoRequestID   = "requestID"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func RequestID(c *echo.Context) string {
	if requestID, ok := c.Get(echoRequestID).(string); ok {
		return requestID
	}
	return FromContext(c.Request().Context())
}

func FromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return ""
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func NormalizeOrNew(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if isValidRequestID(requestID) {
		return requestID
	}
	return newRequestID()
}

func isValidRequestID(requestID string) bool {
	if len(requestID) < 8 || len(requestID) > 128 {
		return false
	}
	for _, r := range requestID {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-" + randomFallback()
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

func randomFallback() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
