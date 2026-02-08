package httputil

import (
	"encoding/json"
	"net/http"
	"strings"
)

// MaxBodySize is the maximum allowed request body size (1MB).
const MaxBodySize = 1 << 20

// DecodeJSON reads and decodes a JSON request body with size limiting.
// Writes a 400 error and returns false on failure.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// ExtractBearerToken extracts a Bearer token from the Authorization header.
// Returns the token and true if found, or empty string and false otherwise.
func ExtractBearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" || !strings.HasPrefix(header, "Bearer ") {
		return "", false
	}
	token := header[7:]
	if token == "" {
		return "", false
	}
	return token, true
}

// ErrorResponse is the standard error envelope for all AYB API errors.
type ErrorResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a standard error response.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{
		Code:    status,
		Message: message,
	})
}

// WriteFieldError writes an error response with field-level validation detail.
func WriteFieldError(w http.ResponseWriter, status int, message string, field, fieldCode, fieldMsg string) {
	WriteJSON(w, status, ErrorResponse{
		Code:    status,
		Message: message,
		Data: map[string]any{
			field: map[string]string{
				"code":    fieldCode,
				"message": fieldMsg,
			},
		},
	})
}
