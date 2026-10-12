package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) FindUserByUsername(ctx context.Context, username string) (User, error) {
	const query = `
		SELECT id, name, username, password_hash, role, production_write, inventory_adjust, active
		FROM users
		WHERE username = $1`
	var user User
	err := r.pool.QueryRow(ctx, query, username).Scan(
		&user.ID,
		&user.Name,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&user.ProductionWrite,
		&user.InventoryAdjust,
		&user.Active,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by username: %w", err)
	}
	return user, nil
}

func (r *PostgresRepository) CreateSession(ctx context.Context, session Session, requestID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create session: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insertSession = `
		INSERT INTO sessions (id, user_id, token_hash, expires_at, last_seen_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $5)`
	if _, err := tx.Exec(ctx, insertSession, session.ID, session.UserID, session.TokenHash, session.ExpiresAt, session.CreatedAt); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	const insertAudit = `
		INSERT INTO audit_events (id, actor_id, action, entity_type, entity_id, request_id)
		VALUES ($1, $2, 'auth.login', 'session', $3, $4)`
	if _, err := tx.Exec(ctx, insertAudit, uuid.New(), session.UserID, session.ID, requestID); err != nil {
		return fmt.Errorf("audit session creation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create session: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindUserBySessionHash(ctx context.Context, tokenHash []byte, now time.Time) (User, error) {
	const query = `
		SELECT u.id, u.name, u.username, u.password_hash, u.role,
		       u.production_write, u.inventory_adjust, u.active
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > $2`
	var user User
	err := r.pool.QueryRow(ctx, query, tokenHash, now).Scan(
		&user.ID,
		&user.Name,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&user.ProductionWrite,
		&user.InventoryAdjust,
		&user.Active,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by session: %w", err)
	}
	return user, nil
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, tokenHash []byte, actorID uuid.UUID, requestID string, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin revoke session: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sessionID uuid.UUID
	const revoke = `
		UPDATE sessions
		SET revoked_at = $2
		WHERE token_hash = $1 AND revoked_at IS NULL
		RETURNING id`
	if err := tx.QueryRow(ctx, revoke, tokenHash, now).Scan(&sessionID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	const insertAudit = `
		INSERT INTO audit_events (id, actor_id, action, entity_type, entity_id, request_id)
		VALUES ($1, $2, 'auth.logout', 'session', $3, $4)`
	if _, err := tx.Exec(ctx, insertAudit, uuid.New(), actorID, sessionID, requestID); err != nil {
		return fmt.Errorf("audit session revocation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit revoke session: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpsertDevelopmentUser(ctx context.Context, user User) error {
	const query = `
		INSERT INTO users (
			id, name, username, password_hash, role,
			production_write, inventory_adjust, active
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE)
		ON CONFLICT (username) DO UPDATE
		SET name = EXCLUDED.name,
		    password_hash = EXCLUDED.password_hash,
		    role = EXCLUDED.role,
		    production_write = EXCLUDED.production_write,
		    inventory_adjust = EXCLUDED.inventory_adjust,
		    active = TRUE,
		    updated_at = now()`
	if _, err := r.pool.Exec(
		ctx,
		query,
		user.ID,
		user.Name,
		user.Username,
		user.PasswordHash,
		user.Role,
		user.ProductionWrite,
		user.InventoryAdjust,
	); err != nil {
		return fmt.Errorf("upsert development user: %w", err)
	}
	return nil
}
