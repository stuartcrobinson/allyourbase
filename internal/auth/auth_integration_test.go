//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

const testJWTSecret = "integration-test-secret-that-is-at-least-32-chars!!"

func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	logger := testutil.DiscardLogger()
	runner := migrations.NewRunner(sharedPG.Pool, logger)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrapping migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("running migrations: %v", err)
	}
}

func newAuthService() *auth.Service {
	return auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, 7*24*time.Hour, testutil.DiscardLogger())
}

func setupAuthServer(t *testing.T, ctx context.Context) *server.Server {
	t.Helper()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret

	authSvc := newAuthService()
	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
}

func doJSON(t *testing.T, srv *server.Server, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

type authResp struct {
	Token        string         `json:"token"`
	RefreshToken string         `json:"refreshToken"`
	User         map[string]any `json:"user"`
}

func parseAuthResp(t *testing.T, w *httptest.ResponseRecorder) authResp {
	t.Helper()
	var resp authResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parsing auth response: %v (body: %s)", err, w.Body.String())
	}
	return resp
}

// --- Registration tests ---

func TestRegisterSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "alice@example.com", "password": "password123",
	}, "")

	testutil.Equal(t, w.Code, http.StatusCreated)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return a token")
	testutil.True(t, resp.RefreshToken != "", "should return a refresh token")
	testutil.Equal(t, resp.User["email"].(string), "alice@example.com")
	testutil.True(t, resp.User["id"].(string) != "", "should have user id")
}

func TestRegisterDuplicateEmail(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	body := map[string]string{"email": "dup@example.com", "password": "password123"}
	w := doJSON(t, srv, "POST", "/api/auth/register", body, "")
	testutil.Equal(t, w.Code, http.StatusCreated)

	// Same email again.
	w = doJSON(t, srv, "POST", "/api/auth/register", body, "")
	testutil.Equal(t, w.Code, http.StatusConflict)
}

func TestRegisterDuplicateEmailCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "User@Example.com", "password": "password123",
	}, "")
	testutil.Equal(t, w.Code, http.StatusCreated)

	// Same email, different case.
	w = doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "user@example.com", "password": "password123",
	}, "")
	testutil.Equal(t, w.Code, http.StatusConflict)
}

// --- Login tests ---

func TestLoginSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register first.
	doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "login@example.com", "password": "password123",
	}, "")

	// Login.
	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "login@example.com", "password": "password123",
	}, "")
	testutil.Equal(t, w.Code, http.StatusOK)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return a token")
	testutil.True(t, resp.RefreshToken != "", "should return a refresh token")
	testutil.Equal(t, resp.User["email"].(string), "login@example.com")
}

func TestLoginWrongPassword(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "wrong@example.com", "password": "password123",
	}, "")

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "wrong@example.com", "password": "wrongpassword",
	}, "")
	testutil.Equal(t, w.Code, http.StatusUnauthorized)
}

func TestLoginNonexistentEmail(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "noone@example.com", "password": "password123",
	}, "")
	// Same status as wrong password — no enumeration.
	testutil.Equal(t, w.Code, http.StatusUnauthorized)
}

// --- /me endpoint tests ---

func TestMeWithRegisterToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "me@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	w = doJSON(t, srv, "GET", "/api/auth/me", nil, resp.Token)
	testutil.Equal(t, w.Code, http.StatusOK)

	var user map[string]any
	json.Unmarshal(w.Body.Bytes(), &user)
	testutil.Equal(t, user["email"].(string), "me@example.com")
}

func TestMeWithLoginToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "melogin@example.com", "password": "password123",
	}, "")

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "melogin@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	w = doJSON(t, srv, "GET", "/api/auth/me", nil, resp.Token)
	testutil.Equal(t, w.Code, http.StatusOK)
}

func TestMeWithoutToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "GET", "/api/auth/me", nil, "")
	testutil.Equal(t, w.Code, http.StatusUnauthorized)
}

// --- Protected collection endpoints ---

func TestCollectionEndpointRequiresAuth(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Create a test table.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL
		)
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	authSvc := newAuthService()
	srv = server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	// Without token → 401.
	w := doJSON(t, srv, "GET", "/api/collections/posts/", nil, "")
	testutil.Equal(t, w.Code, http.StatusUnauthorized)

	// Register and get token.
	w = doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "authed@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// With token → 200.
	w = doJSON(t, srv, "GET", "/api/collections/posts/", nil, resp.Token)
	testutil.Equal(t, w.Code, http.StatusOK)
}

// --- RLS enforcement ---

func TestRLSEnforcement(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	// Create a table with RLS.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE notes (
			id SERIAL PRIMARY KEY,
			owner_id TEXT NOT NULL,
			content TEXT NOT NULL
		);
		ALTER TABLE notes ENABLE ROW LEVEL SECURITY;
		ALTER TABLE notes FORCE ROW LEVEL SECURITY;
		CREATE POLICY notes_owner ON notes
			USING (owner_id = current_setting('ayb.user_id', true));
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	// Register two users.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "user1@example.com", "password": "password123",
	}, "")
	user1 := parseAuthResp(t, w)

	w = doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "user2@example.com", "password": "password123",
	}, "")
	user2 := parseAuthResp(t, w)

	user1ID := user1.User["id"].(string)
	user2ID := user2.User["id"].(string)

	// Insert notes owned by each user (bypass RLS with superuser pool).
	_, err = sharedPG.Pool.Exec(ctx,
		"INSERT INTO notes (owner_id, content) VALUES ($1, 'user1 note'), ($2, 'user2 note')",
		user1ID, user2ID)
	testutil.NoError(t, err)

	// User 1 should only see their note.
	w = doJSON(t, srv, "GET", "/api/collections/notes/", nil, user1.Token)
	testutil.Equal(t, w.Code, http.StatusOK)

	var list1 struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &list1)
	testutil.Equal(t, len(list1.Items), 1)
	testutil.Equal(t, list1.Items[0]["content"], "user1 note")

	// User 2 should only see their note.
	w = doJSON(t, srv, "GET", "/api/collections/notes/", nil, user2.Token)
	testutil.Equal(t, w.Code, http.StatusOK)

	var list2 struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &list2)
	testutil.Equal(t, len(list2.Items), 1)
	testutil.Equal(t, list2.Items[0]["content"], "user2 note")
}

// --- Refresh token tests ---

func setupAuthServerWithRefreshDur(t *testing.T, ctx context.Context, refreshDur time.Duration) *server.Server {
	t.Helper()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret

	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, refreshDur, logger)
	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
}

func TestRefreshTokenFlow(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "refresh@example.com", "password": "password123",
	}, "")
	testutil.Equal(t, w.Code, http.StatusCreated)
	resp := parseAuthResp(t, w)
	testutil.True(t, resp.RefreshToken != "", "should return refresh token")

	// Use refresh token to get new tokens.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.Equal(t, w.Code, http.StatusOK)
	refreshResp := parseAuthResp(t, w)
	testutil.True(t, refreshResp.Token != "", "should return new access token")
	testutil.True(t, refreshResp.RefreshToken != "", "should return new refresh token")

	// Verify the new access token works on /me.
	w = doJSON(t, srv, "GET", "/api/auth/me", nil, refreshResp.Token)
	testutil.Equal(t, w.Code, http.StatusOK)
}

func TestRefreshTokenRotation(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "rotate@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)
	oldRefresh := resp.RefreshToken

	// Refresh once.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": oldRefresh,
	}, "")
	testutil.Equal(t, w.Code, http.StatusOK)
	newResp := parseAuthResp(t, w)

	// Old refresh token should now be invalid.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": oldRefresh,
	}, "")
	testutil.Equal(t, w.Code, http.StatusUnauthorized)

	// New refresh token should still work.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": newResp.RefreshToken,
	}, "")
	testutil.Equal(t, w.Code, http.StatusOK)
}

func TestRefreshTokenExpired(t *testing.T) {
	ctx := context.Background()
	// Use a 1ms refresh duration so it expires immediately.
	srv := setupAuthServerWithRefreshDur(t, ctx, time.Millisecond)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "expired@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// Wait for the refresh token to expire.
	time.Sleep(50 * time.Millisecond)

	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.Equal(t, w.Code, http.StatusUnauthorized)
}

func TestLogout(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "logout@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// Logout.
	w = doJSON(t, srv, "POST", "/api/auth/logout", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.Equal(t, w.Code, http.StatusNoContent)

	// Refresh with the logged-out token should fail.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.Equal(t, w.Code, http.StatusUnauthorized)
}

// --- OAuth integration tests ---

func TestOAuthLoginNewUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()
	info := &auth.OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "oauth@example.com",
		Name:           "OAuth User",
	}

	user, token, refreshToken, err := svc.OAuthLogin(ctx, "google", info)
	testutil.NoError(t, err)
	testutil.True(t, user.ID != "", "should create user")
	testutil.Equal(t, user.Email, "oauth@example.com")
	testutil.True(t, token != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")

	// Verify the access token works.
	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, claims.Subject, user.ID)
}

func TestOAuthLoginExistingIdentity(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()
	info := &auth.OAuthUserInfo{
		ProviderUserID: "google-456",
		Email:          "repeat@example.com",
		Name:           "Repeat User",
	}

	// First login creates user.
	user1, _, _, err := svc.OAuthLogin(ctx, "google", info)
	testutil.NoError(t, err)

	// Second login with same provider identity returns same user.
	user2, _, _, err := svc.OAuthLogin(ctx, "google", info)
	testutil.NoError(t, err)
	testutil.Equal(t, user2.ID, user1.ID)
}

func TestOAuthLoginLinksToExistingEmailUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()

	// Register a user with email/password first.
	emailUser, _, _, err := svc.Register(ctx, "linked@example.com", "password123")
	testutil.NoError(t, err)

	// Login via OAuth with the same email.
	info := &auth.OAuthUserInfo{
		ProviderUserID: "github-789",
		Email:          "linked@example.com",
		Name:           "Linked User",
	}
	oauthUser, _, _, err := svc.OAuthLogin(ctx, "github", info)
	testutil.NoError(t, err)

	// Should be the same user (linked, not a new account).
	testutil.Equal(t, oauthUser.ID, emailUser.ID)
}

func TestOAuthLoginMultipleProvidersSameUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()

	// Login via Google.
	googleInfo := &auth.OAuthUserInfo{
		ProviderUserID: "google-multi",
		Email:          "multi@example.com",
		Name:           "Multi User",
	}
	user1, _, _, err := svc.OAuthLogin(ctx, "google", googleInfo)
	testutil.NoError(t, err)

	// Login via GitHub with same email.
	githubInfo := &auth.OAuthUserInfo{
		ProviderUserID: "github-multi",
		Email:          "multi@example.com",
		Name:           "Multi User",
	}
	user2, _, _, err := svc.OAuthLogin(ctx, "github", githubInfo)
	testutil.NoError(t, err)

	// Should be the same user.
	testutil.Equal(t, user2.ID, user1.ID)
}

func TestOAuthLoginNoEmail(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()
	info := &auth.OAuthUserInfo{
		ProviderUserID: "github-noemail",
		Email:          "",
		Name:           "No Email User",
	}

	user, _, _, err := svc.OAuthLogin(ctx, "github", info)
	testutil.NoError(t, err)
	testutil.True(t, user.ID != "", "should create user even without email")
	// Should have a placeholder email.
	testutil.True(t, user.Email != "", "should have placeholder email")
}

func TestOAuthHandlerFullFlowMocked(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	// Set up fake OAuth provider endpoints.
	fakeProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "fake-access-token",
			})
		case "/userinfo":
			json.NewEncoder(w).Encode(map[string]any{
				"id":    "12345",
				"email": "fakeuser@example.com",
				"name":  "Fake User",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeProvider.Close()

	// Override Google's endpoints to point to our fake server.
	auth.SetProviderURLs("google", auth.OAuthProviderConfig{
		AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:    fakeProvider.URL + "/token",
		UserInfoURL: fakeProvider.URL + "/userinfo",
		Scopes:      []string{"openid", "email", "profile"},
	})
	defer auth.ResetProviderURLs("google")

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.OAuth = map[string]config.OAuthProvider{
		"google": {Enabled: true, ClientID: "test-id", ClientSecret: "test-secret"},
	}
	cfg.Auth.OAuthRedirectURL = "http://localhost:5173/callback"

	svc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, svc, nil)

	// Step 1: Initiate OAuth → should redirect to Google.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/google", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, w.Code, http.StatusTemporaryRedirect)
	loc := w.Header().Get("Location")
	testutil.True(t, loc != "", "should redirect")

	// Extract state from the redirect URL.
	var state string
	if idx := len("state="); true {
		for _, part := range splitQuery(loc) {
			if len(part) > idx && part[:idx] == "state=" {
				state = part[idx:]
				break
			}
		}
	}
	testutil.True(t, state != "", "redirect should include state")

	// Step 2: Simulate callback from provider.
	callbackURL := fmt.Sprintf("/api/auth/oauth/google/callback?code=test-code&state=%s", state)
	req = httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.Host = "localhost:8090"
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Should redirect to the configured redirect URL with tokens.
	testutil.Equal(t, w.Code, http.StatusTemporaryRedirect)
	redirectLoc := w.Header().Get("Location")
	testutil.True(t, redirectLoc != "", "should redirect with tokens")
	testutil.True(t, len(redirectLoc) > len("http://localhost:5173/callback#"), "redirect should have fragment")

	// Verify the user was created.
	var count int
	err := sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_users WHERE email = 'fakeuser@example.com'",
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, count, 1)

	// Verify the OAuth account was linked.
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_oauth_accounts WHERE provider = 'google' AND provider_user_id = '12345'",
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, count, 1)
}

// splitQuery splits a URL's query string into key=value pairs.
func splitQuery(rawURL string) []string {
	idx := 0
	for i, c := range rawURL {
		if c == '?' {
			idx = i + 1
			break
		}
	}
	if idx == 0 {
		return nil
	}
	query := rawURL[idx:]
	var parts []string
	for _, p := range splitOn(query, '&') {
		parts = append(parts, p)
	}
	return parts
}

func splitOn(s string, sep byte) []string {
	var result []string
	start := 0
	for i := range len(s) {
		if s[i] == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}
