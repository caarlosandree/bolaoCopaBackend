package audit

import "testing"

func TestSanitizeMetadataRemovesSensitiveFields(t *testing.T) {
	metadata := map[string]any{
		"match_id":      10,
		"password":      "secret",
		"Authorization": "Bearer token",
		"email":         "user@example.com",
		"duration_ms":   15,
	}

	got := SanitizeMetadata(metadata)

	if got["match_id"] != 10 {
		t.Fatalf("expected safe metadata to remain")
	}
	if got["duration_ms"] != 15 {
		t.Fatalf("expected duration metadata to remain")
	}
	if _, ok := got["password"]; ok {
		t.Fatalf("expected password to be removed")
	}
	if _, ok := got["Authorization"]; ok {
		t.Fatalf("expected Authorization to be removed")
	}
	if _, ok := got["email"]; ok {
		t.Fatalf("expected email to be removed")
	}
}

func TestOutcomeFromStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{httpStatusOK, "success"},
		{httpStatusBadRequest, "client_error"},
		{httpStatusInternalServerError, "server_error"},
	}

	for _, tt := range tests {
		if got := OutcomeFromStatus(tt.status); got != tt.want {
			t.Fatalf("status %d: expected %q, got %q", tt.status, tt.want, got)
		}
	}
}

const (
	httpStatusOK                  = 200
	httpStatusBadRequest          = 400
	httpStatusInternalServerError = 500
)
