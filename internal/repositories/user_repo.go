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
		 RETURNING id, name, email, role, total_points, avatar_url, created_at`,
		name, email, passwordHash,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.TotalPoints, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, name, email, password_hash, role, total_points, avatar_url, created_at
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.TotalPoints, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id int) (*models.User, error) {
	var u models.User
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, name, email, role, total_points, avatar_url, created_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.TotalPoints, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) UpdateAvatarURL(ctx context.Context, id int, avatarURL string) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE users SET avatar_url = $1 WHERE id = $2`,
		avatarURL, id,
	)
	return err
}

func (r *UserRepository) UpdateAvatarData(ctx context.Context, id int, data []byte, contentType, avatarURL string) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE users SET avatar_data = $1, avatar_content_type = $2, avatar_url = $3 WHERE id = $4`,
		data, contentType, avatarURL, id,
	)
	return err
}

func (r *UserRepository) GetAvatarData(ctx context.Context, id int) ([]byte, string, error) {
	var data []byte
	var ct sql.NullString
	err := r.DB.QueryRowContext(ctx,
		`SELECT avatar_data, avatar_content_type FROM users WHERE id = $1`,
		id,
	).Scan(&data, &ct)
	if err != nil {
		return nil, "", err
	}
	contentType := "image/jpeg"
	if ct.Valid {
		contentType = ct.String
	}
	return data, contentType, nil
}

func (r *UserRepository) ListAll(ctx context.Context) ([]models.User, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, name, email, role, total_points, avatar_url, created_at
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
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.TotalPoints, &u.AvatarURL, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *UserRepository) GetRanking(ctx context.Context) ([]models.Ranking, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, name, email, total_points, avatar_url
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
		if err := rows.Scan(&r.UserID, &r.Name, &r.Email, &r.TotalPoints, &r.AvatarURL); err != nil {
			return nil, err
		}
		r.Position = pos
		pos++
		ranking = append(ranking, r)
	}
	return ranking, rows.Err()
}
