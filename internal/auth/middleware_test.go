package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func newTestService() *Service {
	return &Service{
		jwtSecret:  []byte(testSecret),
		tokenDur:   time.Hour,
		refreshDur: 7 * 24 * time.Hour,
	}
}

func generateTestToken(svc *Service, userID, email string) string {
	user := &User{ID: userID, Email: email}
	token, _ := svc.generateToken(user)
	return token
}

func TestRequireAuthValidToken(t *testing.T) {
	svc := newTestService()
	token := generateTestToken(svc, "user-1", "test@example.com")

	var gotClaims *Claims
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusOK)
	testutil.NotNil(t, gotClaims)
	testutil.Equal(t, gotClaims.Subject, "user-1")
	testutil.Equal(t, gotClaims.Email, "test@example.com")
}

func TestRequireAuthMissingHeader(t *testing.T) {
	svc := newTestService()
	called := false
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusUnauthorized)
	testutil.False(t, called, "next handler should not be called")
}

func TestRequireAuthMalformedHeader(t *testing.T) {
	svc := newTestService()
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No "Bearer " prefix.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Token abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusUnauthorized)
}

func TestRequireAuthExpiredToken(t *testing.T) {
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  -time.Hour,
	}
	token := generateTestToken(svc, "user-1", "test@example.com")

	validSvc := newTestService()
	handler := RequireAuth(validSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusUnauthorized)
}

func TestOptionalAuthNoHeader(t *testing.T) {
	svc := newTestService()
	var gotClaims *Claims
	handler := OptionalAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusOK)
	testutil.True(t, gotClaims == nil, "claims should be nil")
}

func TestOptionalAuthValidToken(t *testing.T) {
	svc := newTestService()
	token := generateTestToken(svc, "user-2", "other@example.com")

	var gotClaims *Claims
	handler := OptionalAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusOK)
	testutil.NotNil(t, gotClaims)
	testutil.Equal(t, gotClaims.Subject, "user-2")
}

func TestClaimsFromContextNil(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	claims := ClaimsFromContext(req.Context())
	testutil.True(t, claims == nil, "claims should be nil when not set")
}
