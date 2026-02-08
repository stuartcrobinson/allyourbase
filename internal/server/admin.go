package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

// adminAuth handles simple password-based admin dashboard authentication.
// Stateless: tokens are HMAC-derived from a per-boot secret, so no storage needed.
type adminAuth struct {
	password string
	secret   []byte // random 32 bytes, regenerated each server start
}

func newAdminAuth(password string) *adminAuth {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return &adminAuth{password: password, secret: secret}
}

func (a *adminAuth) token() string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte("ayb-admin"))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *adminAuth) validatePassword(password string) bool {
	return subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) == 1
}

func (a *adminAuth) validateToken(token string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.token())) == 1
}

// handleAdminStatus returns whether admin authentication is required.
func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{
		"auth": s.adminAuth != nil,
	})
}

// handleAdminLogin validates the admin password and returns a token.
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth == nil {
		httputil.WriteError(w, http.StatusNotFound, "admin auth not configured")
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if !httputil.DecodeJSON(w, r, &body) {
		return
	}

	if !s.adminAuth.validatePassword(body.Password) {
		httputil.WriteError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"token": s.adminAuth.token(),
	})
}

// requireAdminToken returns middleware that requires a valid admin token.
// When admin.password is not set, all requests pass through.
func (s *Server) requireAdminToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.adminAuth == nil {
			next.ServeHTTP(w, r)
			return
		}

		token, ok := httputil.ExtractBearerToken(r)
		if !ok || !s.adminAuth.validateToken(token) {
			httputil.WriteError(w, http.StatusUnauthorized, "admin authentication required")
			return
		}

		next.ServeHTTP(w, r)
	})
}
