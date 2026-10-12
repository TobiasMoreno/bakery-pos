package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"bakery-pos/backend/internal/auth"

	"github.com/google/uuid"
)

func TestRequirePermissionRejectsSeller(t *testing.T) {
	user := auth.PublicUser{
		ID:          uuid.New(),
		Role:        auth.RoleSeller,
		Permissions: []auth.Permission{auth.PermissionSalesWrite},
	}
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	ctx := context.WithValue(request.Context(), requestIDKey, "request-1")
	ctx = context.WithValue(ctx, userKey, user)
	response := httptest.NewRecorder()
	handler := RequirePermission(auth.PermissionUsersManage)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	handler.ServeHTTP(response, request.WithContext(ctx))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestRequirePermissionAllowsOwner(t *testing.T) {
	user := auth.User{ID: uuid.New(), Role: auth.RoleOwner}.Public()
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	ctx := context.WithValue(request.Context(), requestIDKey, "request-1")
	ctx = context.WithValue(ctx, userKey, user)
	response := httptest.NewRecorder()
	handler := RequirePermission(auth.PermissionUsersManage)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	handler.ServeHTTP(response, request.WithContext(ctx))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}
