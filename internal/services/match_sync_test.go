package services

import (
	"testing"
	"time"
)

func TestParseTheSportsDBTimestamp(t *testing.T) {
	got, err := parseTheSportsDBTimestamp("2026-06-11T19:00:00")
	if err != nil {
		t.Fatalf("parseTheSportsDBTimestamp returned error: %v", err)
	}

	want := time.Date(2026, 6, 11, 19, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
