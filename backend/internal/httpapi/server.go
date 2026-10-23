package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"bakery-pos/backend/internal/auth"
	"bakery-pos/backend/internal/catalog"
	"bakery-pos/backend/internal/inventory"
	"bakery-pos/backend/internal/pos"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const sessionCookieName = "bakery_pos_session"

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	userKey      contextKey = "user"
)

type Pinger interface {
	Ping(context.Context) error
}

type Options struct {
	AllowedOrigin string
	CookieSecure  bool
	Logger        *slog.Logger
	Catalog       *catalog.Service
	Inventory     *inventory.Service
	POS           *pos.Service
}

type Server struct {
	authService  *auth.Service
	pinger       Pinger
	options      Options
	loginLimiter *LoginLimiter
	catalog      *catalog.Service
	inventory    *inventory.Service
	pos          *pos.Service
	now          func() time.Time
}

func NewServer(authService *auth.Service, pinger Pinger, options Options) *Server {
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &Server{
		authService:  authService,
		pinger:       pinger,
		options:      options,
		loginLimiter: NewLoginLimiter(5, 15*time.Minute),
		catalog:      options.Catalog,
		inventory:    options.Inventory,
		pos:          options.POS,
		now:          time.Now,
	}
}

func (s *Server) Routes() http.Handler {
	router := chi.NewRouter()
	router.Use(s.requestID)
	router.Use(s.recoverer)
	router.Use(securityHeaders)
	router.Use(s.validateOrigin)

	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Get("/readyz", s.ready)

	router.Route("/api/v1", func(api chi.Router) {
		api.Post("/auth/login", s.login)
		api.Group(func(protected chi.Router) {
			protected.Use(s.authenticate)
			protected.Get("/me", s.me)
			protected.Post("/auth/logout", s.logout)
			if s.catalog != nil {
				protected.With(RequirePermission(auth.PermissionCatalogRead)).Get("/categories", s.listCategories)
				protected.With(RequirePermission(auth.PermissionPricingWrite)).Post("/categories", s.createCategory)
				protected.With(RequirePermission(auth.PermissionCatalogRead)).Get("/products", s.listProducts)
				protected.With(RequirePermission(auth.PermissionPricingWrite)).Post("/products", s.createProduct)
				protected.With(RequirePermission(auth.PermissionPricingWrite)).Post("/products/import/preview", s.previewProductImport)
				protected.With(RequirePermission(auth.PermissionPricingWrite)).Post("/products/import/commit", s.commitProductImport)
				protected.With(RequirePermission(auth.PermissionCatalogRead)).Get("/products/{id}", s.getProduct)
				protected.With(RequirePermission(auth.PermissionPricingWrite)).Patch("/products/{id}", s.updateProduct)
				protected.With(RequirePermission(auth.PermissionCatalogRead)).Get("/products/{id}/prices", s.priceHistory)
				protected.With(RequirePermission(auth.PermissionPricingWrite)).Post("/prices/bulk-preview", s.previewBulkPrices)
				protected.With(RequirePermission(auth.PermissionPricingWrite)).Post("/prices/bulk-commit", s.commitBulkPrices)
				protected.With(RequirePermission(auth.PermissionCatalogRead)).Get("/pos/catalog-snapshot", s.catalogSnapshot)
			}
			if s.inventory != nil {
				protected.With(RequirePermission(auth.PermissionInventoryRead)).Get("/inventory/balances", s.listInventoryBalances)
				protected.With(RequirePermission(auth.PermissionInventoryRead)).Get("/inventory/movements", s.listInventoryMovements)
			}
			if s.pos != nil {
				protected.With(RequirePermission(auth.PermissionSalesWrite)).Post("/pos/device/acquire", s.acquirePOSDevice)
				protected.With(RequirePermission(auth.PermissionSalesWrite)).Post("/pos/device/release", s.releasePOSDevice)
				protected.With(RequirePermission(auth.PermissionCashOperate)).Post("/cash/shifts", s.openCashShift)
				protected.With(RequirePermission(auth.PermissionCashOperate)).Get("/cash/shifts/current", s.currentCashShift)
				protected.With(RequirePermission(auth.PermissionCashOperate)).Post("/cash/shifts/{id}/movements", s.recordCashMovement)
				protected.With(RequirePermission(auth.PermissionCashOperate)).Post("/cash/shifts/{id}/close", s.closeCashShift)
				protected.With(RequirePermission(auth.PermissionSalesWrite)).Post("/sales", s.createSale)
				protected.With(RequirePermission(auth.PermissionSalesWrite)).Get("/sales", s.listSales)
				protected.With(RequirePermission(auth.PermissionSalesWrite)).Get("/sales/{id}", s.getSale)
				protected.With(RequirePermission(auth.PermissionSalesWrite)).Post("/pos/offline-authorizations", s.issueOfflineAuthorization)
				protected.With(RequirePermission(auth.PermissionSalesWrite)).Post("/sync/operations", s.syncOperations)
				protected.With(RequirePermission(auth.PermissionSalesWrite)).Get("/sync/status", s.syncStatus)
				protected.With(RequirePermission(auth.PermissionRecoveryForce)).Post("/pos/recovery/force", s.forcePOSRecovery)
				protected.With(RequirePermission(auth.PermissionRecoveryForce)).Get("/pos/recovery/current", s.currentPOSRecovery)
				protected.With(RequirePermission(auth.PermissionRecoveryForce)).Post("/pos/recovery/{id}/differences", s.addRecoveryDifference)
				protected.With(RequirePermission(auth.PermissionRecoveryForce)).Post("/pos/recovery/{id}/differences/{difference_id}/resolve", s.resolveRecoveryDifference)
				protected.With(RequirePermission(auth.PermissionRecoveryForce)).Put("/pos/recovery/{id}/controls", s.updateRecoveryControls)
				protected.With(RequirePermission(auth.PermissionRecoveryForce)).Post("/pos/recovery/{id}/reconstructions", s.reconstructRecoverySale)
				protected.With(RequirePermission(auth.PermissionRecoveryForce)).Post("/pos/recovery/{id}/close", s.closePOSRecovery)
			}
		})
	})
	return router
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.pinger.Ping(ctx); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "service_unavailable", "La base de datos no está disponible.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	input.Username = auth.NormalizeUsername(input.Username)
	if input.Username == "" || len(input.Username) > 80 || len(input.Password) < 8 || len(input.Password) > 256 {
		writeError(w, r, http.StatusBadRequest, "validation_error", "Revisá usuario y contraseña.", nil)
		return
	}

	limiterKey := remoteIP(r) + ":" + input.Username
	now := s.now().UTC()
	if s.loginLimiter.Blocked(limiterKey, now) {
		writeError(w, r, http.StatusTooManyRequests, "login_rate_limited", "Demasiados intentos. Esperá unos minutos.", nil)
		return
	}

	user, token, expiresAt, err := s.authService.Login(r.Context(), input.Username, input.Password, RequestIDFromContext(r.Context()))
	if errors.Is(err, auth.ErrInvalidCredentials) || errors.Is(err, auth.ErrInactiveUser) {
		s.loginLimiter.RecordFailure(limiterKey, now)
		writeError(w, r, http.StatusUnauthorized, "invalid_credentials", "Usuario o contraseña incorrectos.", nil)
		return
	}
	if err != nil {
		s.options.Logger.ErrorContext(r.Context(), "login failed", "request_id", RequestIDFromContext(r.Context()), "error", err)
		writeError(w, r, http.StatusInternalServerError, "internal_error", "No pudimos iniciar la sesión.", nil)
		return
	}
	s.loginLimiter.Reset(limiterKey)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(expiresAt.Sub(s.now()).Seconds()),
		HttpOnly: true,
		Secure:   s.options.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "La sesión no es válida.", nil)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "La sesión no es válida.", nil)
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "La sesión no es válida.", nil)
		return
	}
	if err := s.authService.Logout(r.Context(), cookie.Value, user.ID, RequestIDFromContext(r.Context())); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "La sesión no es válida.", nil)
			return
		}
		s.options.Logger.ErrorContext(r.Context(), "logout failed", "request_id", RequestIDFromContext(r.Context()), "error", err)
		writeError(w, r, http.StatusInternalServerError, "internal_error", "No pudimos cerrar la sesión.", nil)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.options.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "Iniciá sesión para continuar.", nil)
			return
		}
		user, err := s.authService.Authenticate(r.Context(), cookie.Value)
		if errors.Is(err, auth.ErrInvalidCredentials) || errors.Is(err, auth.ErrInactiveUser) {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "La sesión venció o fue revocada.", nil)
			return
		}
		if err != nil {
			s.options.Logger.ErrorContext(r.Context(), "session lookup failed", "request_id", RequestIDFromContext(r.Context()), "error", err)
			writeError(w, r, http.StatusInternalServerError, "internal_error", "No pudimos validar la sesión.", nil)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	})
}

func RequirePermission(permission auth.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := UserFromContext(r.Context())
			if !ok {
				writeError(w, r, http.StatusUnauthorized, "unauthorized", "Iniciá sesión para continuar.", nil)
				return
			}
			if !user.Has(permission) {
				writeError(w, r, http.StatusForbidden, "forbidden", "No tenés permiso para realizar esta acción.", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func UserFromContext(ctx context.Context) (auth.PublicUser, bool) {
	user, ok := ctx.Value(userKey).(auth.PublicUser)
	return user, ok
}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return requestID
}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID)))
	})
}

func (s *Server) validateOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && origin != s.options.AllowedOrigin {
			writeError(w, r, http.StatusForbidden, "invalid_origin", "El origen de la solicitud no está permitido.", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.options.Logger.ErrorContext(r.Context(), "panic recovered", "request_id", RequestIDFromContext(r.Context()), "panic", recovered)
				writeError(w, r, http.StatusInternalServerError, "internal_error", "Ocurrió un error inesperado.", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	return decodeJSONLimit(w, r, destination, 8<<10)
}

func decodeJSONLimit(w http.ResponseWriter, r *http.Request, destination any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("el cuerpo debe contener un único objeto JSON")
	}
	return nil
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (s *Server) String() string {
	return fmt.Sprintf("httpapi.Server(origin=%s)", s.options.AllowedOrigin)
}
