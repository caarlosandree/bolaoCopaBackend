package repositories

import (
	"context"
	"database/sql"

	"backend/internal/models"
)

type UserRepository struct {
	DB *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{DB: db}
}

func (r *UserRepository) Create(ctx context.Context, name, email, passwordHash string) (*models.User, error) {
	var u models.User
	err := r.DB.QueryRowContext(ctx,
		`INSERT INTO users (name, email, password_hash, role, total_points)
		 VALUES ($1, $2, $3, 'user', 0)
		 RETURNING id, name, email, role, total_points, created_at`,
		name, email, passwordHash,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.TotalPoints, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, name, email, password_hash, role, total_points, created_at
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.TotalPoints, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) ListAll(ctx context.Context) ([]models.User, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, name, email, role, total_points, created_at
		 FROM users
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.TotalPoints, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *UserRepository) GetRanking(ctx context.Context) ([]models.Ranking, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, name, email, total_points
		 FROM users
		 ORDER BY total_points DESC, name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ranking []models.Ranking
	pos := 1
	for rows.Next() {
		var r models.Ranking
		if err := rows.Scan(&r.UserID, &r.Name, &r.Email, &r.TotalPoints); err != nil {
			return nil, err
		}
		r.Position = pos
		pos++
		ranking = append(ranking, r)
	}
	return ranking, rows.Err()
}
