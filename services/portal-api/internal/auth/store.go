package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

type DBUser struct {
	ID           int64
	Email        string
	DisplayName  string
	PasswordHash string
	Role         string
	Active       bool
	LockedUntil  *time.Time
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (DBUser, error) {
	var u DBUser
	var lockedUntil *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT id, email, COALESCE(display_name,''), password_hash, role, active, locked_until
		FROM users WHERE lower(email)=lower($1)`, email).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.Role, &u.Active, &lockedUntil)
	u.LockedUntil = lockedUntil
	return u, err
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (User, error) {
	var u User
	var active bool
	err := s.db.QueryRow(ctx, `
		SELECT id, email, COALESCE(display_name,''), role, active
		FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &active)
	if err != nil {
		return u, err
	}
	if !active {
		return u, fmt.Errorf("user inactive")
	}
	return u, nil
}

func (s *Store) GetUserRowByID(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT id, email, COALESCE(display_name,''), role, active
		FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Active)
	return u, err
}

func (s *Store) RecordFailedLogin(ctx context.Context, userID int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE users SET
			failed_login_attempts = failed_login_attempts + 1,
			locked_until = CASE
				WHEN failed_login_attempts + 1 >= 5 THEN now() + interval '15 minutes'
				ELSE locked_until
			END,
			updated_at = now()
		WHERE id=$1`, userID)
	return err
}

func (s *Store) RecordSuccessfulLogin(ctx context.Context, userID int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE users SET
			failed_login_attempts = 0,
			locked_until = NULL,
			last_login_at = now(),
			updated_at = now()
		WHERE id=$1`, userID)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash, ua, ip string, expires time.Time) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, user_agent, ip_address, expires_at)
		VALUES ($1,$2,$3,$4,$5)`,
		userID, tokenHash, ua, ip, expires)
	return err
}

func (s *Store) GetSessionUserID(ctx context.Context, tokenHash string) (int64, error) {
	var userID int64
	err := s.db.QueryRow(ctx, `
		SELECT user_id FROM sessions
		WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at > now()`, tokenHash).
		Scan(&userID)
	return userID, err
}

func (s *Store) RevokeSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE sessions SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL`, tokenHash)
	return err
}

func (s *Store) RevokeAllSessions(ctx context.Context, userID int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE sessions SET revoked_at=now()
		WHERE user_id=$1 AND revoked_at IS NULL`, userID)
	return err
}

func (s *Store) CreateUser(ctx context.Context, email, displayName, passwordHash, role string) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO users (email, display_name, password_hash, role)
		VALUES ($1,$2,$3,$4) RETURNING id`,
		email, displayName, passwordHash, role).Scan(&id)
	return id, err
}

func (s *Store) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE users SET password_hash=$2, failed_login_attempts=0, locked_until=NULL, updated_at=now()
		WHERE id=$1`, userID, passwordHash)
	return err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, email, COALESCE(display_name,''), role, active
		FROM users ORDER BY active DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Active); err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, id int64, displayName, role string, active *bool) error {
	if active != nil {
		_, err := s.db.Exec(ctx, `
			UPDATE users SET
				display_name = COALESCE(NULLIF($2,''), display_name),
				role = COALESCE(NULLIF($3,''), role),
				active = $4,
				updated_at = now()
			WHERE id = $1`, id, displayName, role, *active)
		return err
	}
	_, err := s.db.Exec(ctx, `
		UPDATE users SET
			display_name = COALESCE(NULLIF($2,''), display_name),
			role = COALESCE(NULLIF($3,''), role),
			updated_at = now()
		WHERE id = $1`, id, displayName, role)
	return err
}

func (s *Store) CountActiveAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM users WHERE active=true AND role=$1`, RoleAdmin).Scan(&n)
	return n, err
}

func (s *Store) Audit(ctx context.Context, userID *int64, action, resource string, detail any, ip string) {
	var detailJSON []byte
	if detail != nil {
		detailJSON, _ = json.Marshal(detail)
	}
	_, _ = s.db.Exec(ctx, `
		INSERT INTO audit_log (user_id, action, resource, detail, ip_address)
		VALUES ($1,$2,$3,$4,$5)`,
		userID, action, resource, detailJSON, ip)
}
