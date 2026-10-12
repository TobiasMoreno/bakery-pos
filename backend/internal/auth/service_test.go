package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeRepository struct {
	user           User
	session        Session
	sessionUser    User
	revoked        bool
	createErr      error
	findSessionErr error
}

func (f *fakeRepository) FindUserByUsername(context.Context, string) (User, error) {
	if f.user.ID == uuid.Nil {
		return User{}, ErrNotFound
	}
	return f.user, nil
}

func (f *fakeRepository) CreateSession(_ context.Context, session Session, _ string) error {
	f.session = session
	return f.createErr
}

func (f *fakeRepository) FindUserBySessionHash(context.Context, []byte, time.Time) (User, error) {
	if f.findSessionErr != nil {
		return User{}, f.findSessionErr
	}
	return f.sessionUser, nil
}

func (f *fakeRepository) RevokeSession(context.Context, []byte, uuid.UUID, string, time.Time) error {
	f.revoked = true
	return nil
}

func TestLoginCreatesHashedSession(t *testing.T) {
	params := PasswordParams{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	hash, err := HashPassword("correct-horse", params)
	if err != nil {
		t.Fatal(err)
	}
	repository := &fakeRepository{user: User{
		ID: uuid.New(), Name: "Dueño", Username: "owner", PasswordHash: hash, Role: RoleOwner, Active: true,
	}}
	service := NewService(repository, time.Hour)
	service.now = func() time.Time { return time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC) }
	service.token = func() (string, error) { return "raw-session-token", nil }

	user, rawToken, expiresAt, err := service.Login(context.Background(), " OWNER ", "correct-horse", "request-1")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if user.Role != RoleOwner || rawToken != "raw-session-token" {
		t.Fatalf("unexpected login result: %+v %q", user, rawToken)
	}
	if expiresAt.Sub(service.now()) != time.Hour {
		t.Fatalf("expiresAt = %v", expiresAt)
	}
	if string(repository.session.TokenHash) == rawToken {
		t.Fatal("repository must not receive the raw token")
	}
}

func TestLoginHidesUnknownUser(t *testing.T) {
	service := NewService(&fakeRepository{}, time.Hour)
	_, _, _, err := service.Login(context.Background(), "missing", "long-enough", "request-1")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v", err)
	}
}

func TestAuthenticateRejectsMissingSession(t *testing.T) {
	repository := &fakeRepository{findSessionErr: ErrNotFound}
	service := NewService(repository, time.Hour)
	if _, err := service.Authenticate(context.Background(), "token"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Authenticate() error = %v", err)
	}
}
