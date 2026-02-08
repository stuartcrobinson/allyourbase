package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
)

// Handler tests that don't require a database test the HTTP layer only:
// decoding, error mapping, response format. DB-dependent tests (register,
// login with real users) are in the integration test file.

func TestHandleRegisterValidation(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "invalid email",
			body:       `{"email":"notanemail","password":"12345678"}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid email format",
		},
		{
			name:       "empty email",
			body:       `{"email":"","password":"12345678"}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "email is required",
		},
		{
			name:       "short password",
			body:       `{"email":"user@example.com","password":"short"}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "at least 8 characters",
		},
		{
			name:       "empty body",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "email is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			testutil.Equal(t, w.Code, tt.wantStatus)
			testutil.Contains(t, w.Body.String(), tt.wantMsg)
		})
	}
}

func TestHandleLoginValidation(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	// Login with no DB pool will fail at query level — we get an internal error.
	// But we can still test the malformed JSON path.
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandleMeWithoutToken(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusUnauthorized)
}

func TestHandleRegisterMalformedJSON(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandleRegisterBodyTooLarge(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	// Create a body larger than 1MB.
	largeBody := bytes.Repeat([]byte("x"), httputil.MaxBodySize+1)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
}

func TestAuthResponseFormat(t *testing.T) {
	// Verify the JSON structure of an auth response.
	resp := authResponse{
		Token:        "test-token",
		RefreshToken: "test-refresh-token",
		User: &User{
			ID:        "test-id",
			Email:     "test@example.com",
			CreatedAt: time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC),
		},
	}

	data, err := json.Marshal(resp)
	testutil.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	testutil.NoError(t, err)

	testutil.Equal(t, parsed["token"].(string), "test-token")
	testutil.Equal(t, parsed["refreshToken"].(string), "test-refresh-token")
	user := parsed["user"].(map[string]any)
	testutil.Equal(t, user["id"].(string), "test-id")
	testutil.Equal(t, user["email"].(string), "test@example.com")
	// Verify camelCase field names.
	testutil.NotNil(t, user["createdAt"])
	testutil.NotNil(t, user["updatedAt"])
}

func TestHandleRefreshMalformedJSON(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandleRefreshMissingToken(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "refreshToken is required")
}

func TestHandleLogoutMissingToken(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "refreshToken is required")
}

func TestHandlePasswordResetMissingEmail(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/password-reset", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "email is required")
}

func TestHandlePasswordResetMalformedJSON(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/password-reset", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandlePasswordResetConfirmMissingToken(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/password-reset/confirm",
		strings.NewReader(`{"password":"newpassword123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "token is required")
}

func TestHandlePasswordResetConfirmMissingPassword(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/password-reset/confirm",
		strings.NewReader(`{"token":"sometoken"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "password is required")
}

func TestHandleVerifyEmailMissingToken(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "token is required")
}

func TestHandleResendVerificationNoAuth(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/verify/resend", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusUnauthorized)
}

func TestHandlePasswordResetAlwaysReturns200(t *testing.T) {
	// Even with no DB pool (will fail internally), password-reset
	// should always return 200 to prevent email enumeration.
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/password-reset",
		strings.NewReader(`{"email":"nonexistent@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusOK)
	testutil.Contains(t, w.Body.String(), "if that email exists")
}
