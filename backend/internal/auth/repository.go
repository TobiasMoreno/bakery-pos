package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash []byte
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Repository interface {
	FindUserByUsername(ctx context.Context, username string) (User, error)
	CreateSession(ctx context.Context, session Session, requestID string) error
	FindUserBySessionHash(ctx context.Context, tokenHash []byte, now time.Time) (User, error)
	RevokeSession(ctx context.Context, tokenHash []byte, actorID uuid.UUID, requestID string, now time.Time) error
}
