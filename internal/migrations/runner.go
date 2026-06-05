package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

//go:embed sql/*.up.sql
var migrationFiles embed.FS

// Run aplica todas as migrations *.up.sql ainda não aplicadas, em ordem crescente de versão.
// Cada migration é executada dentro de uma transação individual.
func Run(ctx context.Context, db *sql.DB) error {
	if err := ensureMigrationsTable(ctx, db); err != nil {
		return fmt.Errorf("criar tabela schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return fmt.Errorf("ler versões aplicadas: %w", err)
	}

	files, err := pendingFiles(applied)
	if err != nil {
		return fmt.Errorf("listar migrations: %w", err)
	}

	if len(files) == 0 {
		slog.Info("migrations: banco atualizado, nenhuma migration pendente")
		return nil
	}

	for _, mf := range files {
		if err := applyMigration(ctx, db, mf); err != nil {
			return fmt.Errorf("aplicar migration %s: %w", mf.name, err)
		}
	}

	return nil
}

// --- tipos internos ---

type migration struct {
	version int64
	name    string
	path    string
}

// --- helpers ---

func ensureMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    BIGINT PRIMARY KEY,
			name       TEXT        NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[int64]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int64]bool)
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func pendingFiles(applied map[int64]bool) ([]migration, error) {
	entries, err := fs.Glob(migrationFiles, "sql/*.up.sql")
	if err != nil {
		return nil, err
	}

	var pending []migration
	for _, path := range entries {
		base := strings.TrimSuffix(strings.TrimPrefix(path, "sql/"), ".up.sql")
		// formato esperado: 000001_nome_da_migration
		parts := strings.SplitN(base, "_", 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("nome de migration inválido: %s (esperado: NNNNNN_nome)", path)
		}
		version, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("versão inválida em %s: %w", path, err)
		}
		if !applied[version] {
			pending = append(pending, migration{version: version, name: base, path: path})
		}
	}

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].version < pending[j].version
	})
	return pending, nil
}

func applyMigration(ctx context.Context, db *sql.DB, mf migration) error {
	content, err := migrationFiles.ReadFile(mf.path)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("executar SQL: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
		mf.version, mf.name,
	); err != nil {
		return fmt.Errorf("registrar versão: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	slog.Info("migrations: aplicada", "version", mf.version, "name", mf.name)
	return nil
}
