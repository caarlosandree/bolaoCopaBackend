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
		`SELECT id, name, email, password_hash, role, total_points, avatar_url, is_hidden, created_at
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.TotalPoints, &u.AvatarURL, &u.IsHidden, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id int) (*models.User, error) {
	var u models.User
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, name, email, role, total_points, avatar_url, is_hidden, created_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.TotalPoints, &u.AvatarURL, &u.IsHidden, &u.CreatedAt)
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
		`SELECT id, name, email, role, total_points, avatar_url, is_hidden, created_at
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
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.TotalPoints, &u.AvatarURL, &u.IsHidden, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *UserRepository) UpdatePasswordHash(ctx context.Context, userID int, hash string) error {
	res, err := r.DB.ExecContext(ctx,
		`UPDATE users SET password_hash = $1 WHERE id = $2`,
		hash, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteUser remove um usuário e, por cascata configurada no banco,
// todos os seus palpites (guesses). Audit logs do ator têm actor_user_id
// setado para NULL (ON DELETE SET NULL). Retorna sql.ErrNoRows se o
// usuário não existir.
func (r *UserRepository) DeleteUser(ctx context.Context, id int) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (r *UserRepository) GetRanking(ctx context.Context, currentUserID int) ([]models.Ranking, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, name, email, total_points, avatar_url, is_hidden
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
		var entry models.Ranking
		var isHidden bool
		if err := rows.Scan(&entry.UserID, &entry.Name, &entry.Email, &entry.TotalPoints, &entry.AvatarURL, &isHidden); err != nil {
			return nil, err
		}
		if isHidden && entry.UserID != currentUserID {
			continue
		}
		entry.Position = pos
		pos++
		ranking = append(ranking, entry)
	}
	return ranking, rows.Err()
}

func (r *UserRepository) ListHiddenUserIDs(ctx context.Context) ([]int, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id FROM users WHERE is_hidden = TRUE`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *UserRepository) UpdateIsHidden(ctx context.Context, userID int, hidden bool) error {
	res, err := r.DB.ExecContext(ctx,
		`UPDATE users SET is_hidden = $1 WHERE id = $2`,
		hidden, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
