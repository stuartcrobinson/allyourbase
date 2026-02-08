package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestOAuthStateStoreGenerateAndValidate(t *testing.T) {
	store := NewOAuthStateStore(time.Minute)

	token, err := store.Generate()
	testutil.NoError(t, err)
	testutil.True(t, len(token) > 0, "token should not be empty")

	// First validation succeeds.
	testutil.True(t, store.Validate(token), "first validate should succeed")

	// Second validation fails (one-time use).
	testutil.False(t, store.Validate(token), "second validate should fail (consumed)")
}

func TestOAuthStateStoreExpiry(t *testing.T) {
	store := NewOAuthStateStore(1 * time.Millisecond)

	token, err := store.Generate()
	testutil.NoError(t, err)

	time.Sleep(5 * time.Millisecond)
	testutil.False(t, store.Validate(token), "expired token should fail")
}

func TestOAuthStateStoreInvalid(t *testing.T) {
	store := NewOAuthStateStore(time.Minute)
	testutil.False(t, store.Validate("nonexistent"), "unknown token should fail")
}

func TestAuthorizationURLGoogle(t *testing.T) {
	client := OAuthClientConfig{ClientID: "my-id", ClientSecret: "my-secret"}
	u, err := AuthorizationURL("google", client, "http://localhost/callback", "test-state")
	testutil.NoError(t, err)
	testutil.Contains(t, u, "accounts.google.com")
	testutil.Contains(t, u, "client_id=my-id")
	testutil.Contains(t, u, "state=test-state")
	testutil.Contains(t, u, "redirect_uri=")
	testutil.Contains(t, u, "scope=")
	testutil.Contains(t, u, "access_type=offline")
}

func TestAuthorizationURLGitHub(t *testing.T) {
	client := OAuthClientConfig{ClientID: "gh-id", ClientSecret: "gh-secret"}
	u, err := AuthorizationURL("github", client, "http://localhost/callback", "test-state")
	testutil.NoError(t, err)
	testutil.Contains(t, u, "github.com/login/oauth/authorize")
	testutil.Contains(t, u, "client_id=gh-id")
	testutil.Contains(t, u, "scope=user")
}

func TestAuthorizationURLUnsupported(t *testing.T) {
	client := OAuthClientConfig{ClientID: "id", ClientSecret: "secret"}
	_, err := AuthorizationURL("twitter", client, "http://localhost/callback", "state")
	testutil.ErrorContains(t, err, "not configured")
}

func TestParseGoogleUser(t *testing.T) {
	body := `{"id":"12345","email":"user@gmail.com","name":"Test User"}`
	info, err := parseGoogleUser([]byte(body))
	testutil.NoError(t, err)
	testutil.Equal(t, info.ProviderUserID, "12345")
	testutil.Equal(t, info.Email, "user@gmail.com")
	testutil.Equal(t, info.Name, "Test User")
}

func TestParseGoogleUserMissingID(t *testing.T) {
	body := `{"email":"user@gmail.com"}`
	_, err := parseGoogleUser([]byte(body))
	testutil.ErrorContains(t, err, "missing user ID")
}

func TestParseGitHubUser(t *testing.T) {
	// Set up a mock emails endpoint for the case where email is empty.
	body := `{"id":42,"login":"octocat","email":"octocat@github.com","name":"The Octocat"}`
	info, err := parseGitHubUser(nil, []byte(body), "unused-token")
	testutil.NoError(t, err)
	testutil.Equal(t, info.ProviderUserID, "42")
	testutil.Equal(t, info.Email, "octocat@github.com")
	testutil.Equal(t, info.Name, "The Octocat")
}

func TestParseGitHubUserFallbackLoginAsName(t *testing.T) {
	body := `{"id":42,"login":"octocat","email":"octocat@github.com","name":""}`
	info, err := parseGitHubUser(nil, []byte(body), "unused-token")
	testutil.NoError(t, err)
	testutil.Equal(t, info.Name, "octocat")
}

func TestParseGitHubUserMissingID(t *testing.T) {
	body := `{"login":"octocat"}`
	_, err := parseGitHubUser(nil, []byte(body), "token")
	testutil.ErrorContains(t, err, "missing user ID")
}

func TestHandleOAuthRedirectUnknownProvider(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/oauth/twitter", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusNotFound)
	testutil.Contains(t, w.Body.String(), "not configured")
}

func TestHandleOAuthRedirectConfiguredProvider(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "test-id", ClientSecret: "test-secret"})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/oauth/google", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusTemporaryRedirect)
	loc := w.Header().Get("Location")
	testutil.Contains(t, loc, "accounts.google.com")
	testutil.Contains(t, loc, "client_id=test-id")
	testutil.Contains(t, loc, "state=")
}

func TestHandleOAuthCallbackMissingState(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "invalid or expired OAuth state")
}

func TestHandleOAuthCallbackMissingCode(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})

	// Generate a valid state.
	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	router := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?state="+state, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "missing authorization code")
}

func TestHandleOAuthCallbackProviderError(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?error=access_denied&error_description=user+denied", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "denied or failed")
}

func TestOAuthCallbackURLDerivation(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		proto    string
		tls      bool
		provider string
		want     string
	}{
		{
			name:     "http",
			host:     "localhost:8090",
			provider: "google",
			want:     "http://localhost:8090/api/auth/oauth/google/callback",
		},
		{
			name:     "forwarded https",
			host:     "myapp.com",
			proto:    "https",
			provider: "github",
			want:     "https://myapp.com/api/auth/oauth/github/callback",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			got := oauthCallbackURL(req, tt.provider)
			testutil.Equal(t, got, tt.want)
		})
	}
}

func TestOAuthCallbackWithCodeExchangeFailure(t *testing.T) {
	// Start a fake token endpoint that returns an error.
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "bad_code"})
	}))
	defer fakeServer.Close()

	// Temporarily override Google's token URL.
	orig := oauthProviders["google"]
	modified := orig
	modified.TokenURL = fakeServer.URL
	oauthProviders["google"] = modified
	defer func() { oauthProviders["google"] = orig }()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})

	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	router := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=bad&state="+state, nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadGateway)
	testutil.Contains(t, w.Body.String(), "failed to authenticate")
}
