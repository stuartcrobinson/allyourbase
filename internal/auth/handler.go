package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// Handler serves auth HTTP endpoints.
type Handler struct {
	auth             *Service
	logger           *slog.Logger
	oauthClients     map[string]OAuthClientConfig
	oauthStateStore  *OAuthStateStore
	oauthRedirectURL string
}

// NewHandler creates a new auth handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{
		auth:            svc,
		logger:          logger,
		oauthClients:    make(map[string]OAuthClientConfig),
		oauthStateStore: NewOAuthStateStore(10 * time.Minute),
	}
}

// SetOAuthProvider registers an OAuth provider with its client credentials.
func (h *Handler) SetOAuthProvider(provider string, client OAuthClientConfig) {
	h.oauthClients[provider] = client
}

// SetOAuthRedirectURL sets the URL to redirect to after OAuth login.
func (h *Handler) SetOAuthRedirectURL(u string) {
	h.oauthRedirectURL = u
}

// Routes returns a chi.Router with auth endpoints mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/register", h.handleRegister)
	r.Post("/login", h.handleLogin)
	r.Post("/refresh", h.handleRefresh)
	r.Post("/logout", h.handleLogout)
	r.With(RequireAuth(h.auth)).Get("/me", h.handleMe)
	r.Post("/password-reset", h.handlePasswordReset)
	r.Post("/password-reset/confirm", h.handlePasswordResetConfirm)
	r.Post("/verify", h.handleVerifyEmail)
	r.With(RequireAuth(h.auth)).Post("/verify/resend", h.handleResendVerification)
	r.Get("/oauth/{provider}", h.handleOAuthRedirect)
	r.Get("/oauth/{provider}/callback", h.handleOAuthCallback)
	return r
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type authResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	User         *User  `json:"user"`
}

func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if !decodeBody(w, r, &req) {
		return
	}

	user, token, refreshToken, err := h.auth.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			// Strip the "validation error: " sentinel prefix from user-facing message.
			msg := strings.TrimPrefix(err.Error(), ErrValidation.Error()+": ")
			httputil.WriteError(w, http.StatusBadRequest, msg)
		case errors.Is(err, ErrEmailTaken):
			httputil.WriteError(w, http.StatusConflict, "email already registered")
		default:
			h.logger.Error("register error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, authResponse{Token: token, RefreshToken: refreshToken, User: user})
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if !decodeBody(w, r, &req) {
		return
	}

	user, token, refreshToken, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			httputil.WriteError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		h.logger.Error("login error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{Token: token, RefreshToken: refreshToken, User: user})
}

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	user, err := h.auth.UserByID(r.Context(), claims.Subject)
	if err != nil {
		h.logger.Error("user lookup error", "error", err, "user_id", claims.Subject)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, user)
}

func (h *Handler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.RefreshToken == "" {
		httputil.WriteError(w, http.StatusBadRequest, "refreshToken is required")
		return
	}

	user, accessToken, refreshToken, err := h.auth.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) {
			httputil.WriteError(w, http.StatusUnauthorized, "invalid or expired refresh token")
			return
		}
		h.logger.Error("refresh error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{Token: accessToken, RefreshToken: refreshToken, User: user})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.RefreshToken == "" {
		httputil.WriteError(w, http.StatusBadRequest, "refreshToken is required")
		return
	}

	if err := h.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		h.logger.Error("logout error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type passwordResetRequest struct {
	Email string `json:"email"`
}

type passwordResetConfirmRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type tokenRequest struct {
	Token string `json:"token"`
}

func (h *Handler) handlePasswordReset(w http.ResponseWriter, r *http.Request) {
	var req passwordResetRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Email == "" {
		httputil.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Always return 200 to prevent email enumeration.
	if err := h.auth.RequestPasswordReset(r.Context(), req.Email); err != nil {
		h.logger.Error("password reset error", "error", err)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "if that email exists, a reset link has been sent"})
}

func (h *Handler) handlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	var req passwordResetConfirmRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Token == "" {
		httputil.WriteError(w, http.StatusBadRequest, "token is required")
		return
	}
	if req.Password == "" {
		httputil.WriteError(w, http.StatusBadRequest, "password is required")
		return
	}

	err := h.auth.ConfirmPasswordReset(r.Context(), req.Token, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidResetToken):
			httputil.WriteError(w, http.StatusBadRequest, "invalid or expired reset token")
		case errors.Is(err, ErrValidation):
			msg := strings.TrimPrefix(err.Error(), ErrValidation.Error()+": ")
			httputil.WriteError(w, http.StatusBadRequest, msg)
		default:
			h.logger.Error("password reset confirm error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "password has been reset"})
}

func (h *Handler) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Token == "" {
		httputil.WriteError(w, http.StatusBadRequest, "token is required")
		return
	}

	err := h.auth.ConfirmEmail(r.Context(), req.Token)
	if err != nil {
		if errors.Is(err, ErrInvalidVerifyToken) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid or expired verification token")
			return
		}
		h.logger.Error("email verification error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "email verified"})
}

func (h *Handler) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.auth.SendVerificationEmail(r.Context(), claims.Subject, claims.Email); err != nil {
		h.logger.Error("resend verification error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "verification email sent"})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	return httputil.DecodeJSON(w, r, v)
}

func (h *Handler) handleOAuthRedirect(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	client, ok := h.oauthClients[provider]
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, fmt.Sprintf("OAuth provider %q not configured", provider))
		return
	}

	state, err := h.oauthStateStore.Generate()
	if err != nil {
		h.logger.Error("OAuth state generation error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	callbackURL := oauthCallbackURL(r, provider)
	authURL, err := AuthorizationURL(provider, client, callbackURL, state)
	if err != nil {
		h.logger.Error("OAuth URL error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (h *Handler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	client, ok := h.oauthClients[provider]
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, fmt.Sprintf("OAuth provider %q not configured", provider))
		return
	}

	// Check for provider-side errors.
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		desc := r.URL.Query().Get("error_description")
		h.logger.Warn("OAuth provider error", "provider", provider, "error", errMsg, "description", desc)
		httputil.WriteError(w, http.StatusBadRequest, "OAuth authentication was denied or failed")
		return
	}

	// Validate CSRF state.
	state := r.URL.Query().Get("state")
	if !h.oauthStateStore.Validate(state) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid or expired OAuth state")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		httputil.WriteError(w, http.StatusBadRequest, "missing authorization code")
		return
	}

	// Exchange code for user info.
	callbackURL := oauthCallbackURL(r, provider)
	info, err := ExchangeCode(r.Context(), provider, client, code, callbackURL)
	if err != nil {
		h.logger.Error("OAuth code exchange error", "provider", provider, "error", err)
		httputil.WriteError(w, http.StatusBadGateway, "failed to authenticate with provider")
		return
	}

	// Find or create user + issue tokens.
	user, accessToken, refreshToken, err := h.auth.OAuthLogin(r.Context(), provider, info)
	if err != nil {
		h.logger.Error("OAuth login error", "provider", provider, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// If a redirect URL is configured, redirect with tokens in hash fragment.
	if h.oauthRedirectURL != "" {
		fragment := url.Values{
			"token":        {accessToken},
			"refreshToken": {refreshToken},
		}
		dest := h.oauthRedirectURL + "#" + fragment.Encode()
		http.Redirect(w, r, dest, http.StatusTemporaryRedirect)
		return
	}

	// No redirect URL — return JSON directly.
	httputil.WriteJSON(w, http.StatusOK, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

// oauthCallbackURL derives the OAuth callback URL from the current request.
func oauthCallbackURL(r *http.Request, provider string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return fmt.Sprintf("%s://%s/api/auth/oauth/%s/callback", scheme, r.Host, provider)
}
