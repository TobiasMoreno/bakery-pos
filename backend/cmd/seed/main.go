package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"bakery-pos/backend/internal/auth"
	"bakery-pos/backend/internal/database"

	"github.com/google/uuid"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	users := []struct {
		name     string
		username string
		password string
		role     auth.Role
	}{
		{
			name:     strings.TrimSpace(os.Getenv("DEMO_OWNER_NAME")),
			username: auth.NormalizeUsername(os.Getenv("DEMO_OWNER_USERNAME")),
			password: os.Getenv("DEMO_OWNER_PASSWORD"),
			role:     auth.RoleOwner,
		},
		{
			name:     strings.TrimSpace(os.Getenv("DEMO_EMPLOYEE_NAME")),
			username: auth.NormalizeUsername(os.Getenv("DEMO_EMPLOYEE_USERNAME")),
			password: os.Getenv("DEMO_EMPLOYEE_PASSWORD"),
			role:     auth.RoleSeller,
		},
	}
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	for _, user := range users {
		if user.name == "" || user.username == "" || len(user.password) < 8 {
			logger.Error("every demo user requires name, username and a password of at least 8 characters", "role", user.role)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	repository := auth.NewPostgresRepository(pool)
	for _, user := range users {
		passwordHash, err := auth.HashPassword(user.password, auth.DefaultPasswordParams())
		if err != nil {
			logger.Error("cannot hash demo credential", "role", user.role, "error", err)
			os.Exit(1)
		}
		if err := repository.UpsertDevelopmentUser(ctx, auth.User{
			ID:           uuid.New(),
			Name:         user.name,
			Username:     user.username,
			PasswordHash: passwordHash,
			Role:         user.role,
			Active:       true,
		}); err != nil {
			logger.Error("cannot seed demo user", "role", user.role, "error", err)
			os.Exit(1)
		}
		logger.Info("development user ready", "username", user.username, "role", user.role)
	}
}
