package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrInactiveUser = errors.New("inactive user")

type Service struct {
	repository     Repository
	passwordParams PasswordParams
	sessionTTL     time.Duration
	now            func() time.Time
	token          func() (string, error)
}

func NewService(repository Repository, sessionTTL time.Duration) *Service {
	return &Service{
		repository:     repository,
		passwordParams: DefaultPasswordParams(),
		sessionTTL:     sessionTTL,
		now:            time.Now,
		token:          generateToken,
	}
}

func (s *Service) Login(ctx context.Context, username, password, requestID string) (PublicUser, string, time.Time, error) {
	user, err := s.repository.FindUserByUsername(ctx, NormalizeUsername(username))
	if errors.Is(err, ErrNotFound) {
		return PublicUser{}, "", time.Time{}, ErrInvalidCredentials
	}
	if err != nil {
		return PublicUser{}, "", time.Time{}, err
	}
	valid, err := VerifyPassword(password, user.PasswordHash)
	if err != nil || !valid {
		return PublicUser{}, "", time.Time{}, ErrInvalidCredentials
	}
	if !user.Active {
		return PublicUser{}, "", time.Time{}, ErrInactiveUser
	}

	rawToken, err := s.token()
	if err != nil {
		return PublicUser{}, "", time.Time{}, fmt.Errorf("generate session token: %w", err)
	}
	now := s.now().UTC()
	expiresAt := now.Add(s.sessionTTL)
	if err := s.repository.CreateSession(ctx, Session{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: HashToken(rawToken),
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, requestID); err != nil {
		return PublicUser{}, "", time.Time{}, err
	}
	return user.Public(), rawToken, expiresAt, nil
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (PublicUser, error) {
	if rawToken == "" {
		return PublicUser{}, ErrInvalidCredentials
	}
	user, err := s.repository.FindUserBySessionHash(ctx, HashToken(rawToken), s.now().UTC())
	if errors.Is(err, ErrNotFound) {
		return PublicUser{}, ErrInvalidCredentials
	}
	if err != nil {
		return PublicUser{}, err
	}
	if !user.Active {
		return PublicUser{}, ErrInactiveUser
	}
	return user.Public(), nil
}

func (s *Service) Logout(ctx context.Context, rawToken string, actorID uuid.UUID, requestID string) error {
	err := s.repository.RevokeSession(ctx, HashToken(rawToken), actorID, requestID, s.now().UTC())
	if errors.Is(err, ErrNotFound) {
		return ErrInvalidCredentials
	}
	return err
}

func HashToken(rawToken string) []byte {
	hash := sha256.Sum256([]byte(rawToken))
	return hash[:]
}

func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
