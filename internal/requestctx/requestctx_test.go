package requestctx

import "testing"

func TestNormalizeOrNewKeepsValidRequestID(t *testing.T) {
	requestID := "support-req_123"

	got := NormalizeOrNew(requestID)

	if got != requestID {
		t.Fatalf("expected request ID %q, got %q", requestID, got)
	}
}

func TestNormalizeOrNewGeneratesRequestIDWhenInvalid(t *testing.T) {
	got := NormalizeOrNew("bad request id with spaces")

	if got == "" {
		t.Fatal("expected generated request ID")
	}
	if got == "bad request id with spaces" {
		t.Fatal("expected invalid request ID to be replaced")
	}
}
