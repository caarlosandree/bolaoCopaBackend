package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net"
	"strings"
	"time"
)

type Event struct {
	RequestID    string
	OccurredAt   time.Time
	ActorUserID  *int
	ActorRole    string
	Action       string
	ResourceType string
	ResourceID   string
	Method       string
	Path         string
	StatusCode   int
	Outcome      string
	IP           string
	UserAgent    string
	Metadata     map[string]any
}

type Repository struct {
	DB *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{DB: db}
}

func (r *Repository) Insert(ctx context.Context, event Event) error {
	metadata, err := json.Marshal(SanitizeMetadata(event.Metadata))
	if err != nil {
		return err
	}

	_, err = r.DB.ExecContext(ctx, `
		INSERT INTO audit_logs (
			request_id,
			occurred_at,
			actor_user_id,
			actor_role,
			action,
			resource_type,
			resource_id,
			method,
			path,
			status_code,
			outcome,
			ip,
			user_agent,
			metadata
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10, $11, $12, NULLIF($13, ''), $14::jsonb)
	`,
		event.RequestID,
		event.OccurredAt,
		event.ActorUserID,
		event.ActorRole,
		event.Action,
		event.ResourceType,
		event.ResourceID,
		event.Method,
		event.Path,
		event.StatusCode,
		event.Outcome,
		nullableIP(event.IP),
		event.UserAgent,
		string(metadata),
	)
	return err
}

func (r *Repository) DeleteOlderThan(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	result, err := r.DB.ExecContext(ctx, `
		DELETE FROM audit_logs
		WHERE occurred_at < now() - make_interval(days => $1)
	`, retentionDays)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

type Service struct {
	Enabled bool
	Repo    *Repository
	Logger  *slog.Logger
}

func NewService(enabled bool, repo *Repository, logger *slog.Logger) *Service {
	return &Service{
		Enabled: enabled,
		Repo:    repo,
		Logger:  logger,
	}
}

func (s *Service) Record(ctx context.Context, event Event) {
	if s == nil || !s.Enabled || s.Repo == nil {
		return
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if event.Outcome == "" {
		event.Outcome = OutcomeFromStatus(event.StatusCode)
	}

	if err := s.Repo.Insert(ctx, event); err != nil && s.Logger != nil {
		s.Logger.ErrorContext(ctx, "audit write failed",
			"request_id", event.RequestID,
			"action", event.Action,
			"error", err,
		)
	}
}

func (s *Service) CleanupExpired(ctx context.Context, retentionDays int) {
	if s == nil || !s.Enabled || s.Repo == nil {
		return
	}

	deleted, err := s.Repo.DeleteOlderThan(ctx, retentionDays)
	if err != nil {
		if s.Logger != nil {
			s.Logger.ErrorContext(ctx, "audit retention cleanup failed",
				"retention_days", retentionDays,
				"error", err,
			)
		}
		return
	}

	if s.Logger != nil && deleted > 0 {
		s.Logger.InfoContext(ctx, "audit retention cleanup completed",
			"retention_days", retentionDays,
			"deleted", deleted,
		)
	}
}

func OutcomeFromStatus(status int) string {
	switch {
	case status >= 500:
		return "server_error"
	case status >= 400:
		return "client_error"
	default:
		return "success"
	}
}

func SanitizeMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}

	clean := make(map[string]any, len(metadata))
	for key, value := range metadata {
		normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		if isSensitiveKey(normalized) {
			continue
		}
		clean[key] = value
	}
	return clean
}

func isSensitiveKey(key string) bool {
	switch key {
	case "password", "password_hash", "token", "jwt", "authorization", "refresh_token", "body", "raw_body", "email", "name":
		return true
	default:
		return strings.Contains(key, "password") ||
			strings.Contains(key, "token") ||
			strings.Contains(key, "authorization")
	}
}

func nullableIP(ip string) any {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return nil
	}
	return parsed.String()
}
