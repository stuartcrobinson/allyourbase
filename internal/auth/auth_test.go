package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-that-is-at-least-32-characters-long!!"

func init() {
	// Use minimal argon2id params in unit tests for speed.
	// Production params (64 MiB, 3 iterations) take ~250ms per hash.
	argonMemory = 1024 // 1 MiB
	argonTime = 1
}

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := hashPassword("mypassword123")
	testutil.NoError(t, err)
	testutil.True(t, len(hash) > 0, "hash should not be empty")
	testutil.Contains(t, hash, "$argon2id$")

	ok, err := verifyPassword(hash, "mypassword123")
	testutil.NoError(t, err)
	testutil.True(t, ok, "correct password should verify")
}

func TestVerifyPasswordWrong(t *testing.T) {
	hash, err := hashPassword("mypassword123")
	testutil.NoError(t, err)

	ok, err := verifyPassword(hash, "wrongpassword")
	testutil.NoError(t, err)
	testutil.False(t, ok, "wrong password should not verify")
}

func TestVerifyPasswordInvalidFormat(t *testing.T) {
	_, err := verifyPassword("not-a-valid-hash", "password")
	testutil.ErrorContains(t, err, "invalid hash format")
}

func TestGenerateAndValidateToken(t *testing.T) {
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  time.Hour,
	}

	user := &User{
		ID:    "550e8400-e29b-41d4-a716-446655440000",
		Email: "test@example.com",
	}

	token, err := svc.generateToken(user)
	testutil.NoError(t, err)
	testutil.True(t, len(token) > 0, "token should not be empty")

	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, claims.Subject, user.ID)
	testutil.Equal(t, claims.Email, user.Email)
	testutil.NotNil(t, claims.ExpiresAt)
	testutil.NotNil(t, claims.IssuedAt)
}

func TestValidateTokenExpired(t *testing.T) {
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  -time.Hour, // expired immediately
	}

	user := &User{ID: "test-id", Email: "test@example.com"}
	token, err := svc.generateToken(user)
	testutil.NoError(t, err)

	_, err = svc.ValidateToken(token)
	testutil.ErrorContains(t, err, "token is expired")
}

func TestValidateTokenTampered(t *testing.T) {
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  time.Hour,
	}

	user := &User{ID: "test-id", Email: "test@example.com"}
	token, err := svc.generateToken(user)
	testutil.NoError(t, err)

	// Tamper with the token by replacing the signature.
	parts := strings.SplitN(token, ".", 3)
	tampered := parts[0] + "." + parts[1] + ".invalidsignature"
	_, err = svc.ValidateToken(tampered)
	testutil.ErrorContains(t, err, "invalid token")
}

func TestValidateTokenWrongSigningMethod(t *testing.T) {
	// Create a token signed with a different method (none).
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "test-id",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Email: "test@example.com",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	testutil.NoError(t, err)

	svc := &Service{jwtSecret: []byte(testSecret)}
	_, err = svc.ValidateToken(tokenString)
	testutil.ErrorContains(t, err, "unexpected signing method")
}

func TestValidateTokenWrongSecret(t *testing.T) {
	svc1 := &Service{jwtSecret: []byte(testSecret), tokenDur: time.Hour}
	svc2 := &Service{jwtSecret: []byte("different-secret-that-is-also-32-chars-long!!")}

	user := &User{ID: "test-id", Email: "test@example.com"}
	token, err := svc1.generateToken(user)
	testutil.NoError(t, err)

	_, err = svc2.ValidateToken(token)
	testutil.ErrorContains(t, err, "invalid token")
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr string
	}{
		{"valid", "user@example.com", ""},
		{"valid subdomain", "user@mail.example.com", ""},
		{"empty", "", "email is required"},
		{"no at", "userexample.com", "invalid email format"},
		{"no domain dot", "user@example", "invalid email format"},
		{"at at start", "@example.com", "invalid email format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEmail(tt.email)
			if tt.wantErr == "" {
				testutil.NoError(t, err)
			} else {
				testutil.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  string
	}{
		{"valid 8 chars", "12345678", ""},
		{"valid long", "a-very-long-secure-password", ""},
		{"too short", "1234567", "at least 8 characters"},
		{"empty", "", "at least 8 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if tt.wantErr == "" {
				testutil.NoError(t, err)
			} else {
				testutil.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestHashRefreshTokenDeterministic(t *testing.T) {
	h1 := hashRefreshToken("test-token-value")
	h2 := hashRefreshToken("test-token-value")
	testutil.Equal(t, h1, h2)
	testutil.Equal(t, len(h1), 64) // SHA-256 hex = 64 chars
}

func TestHashRefreshTokenDifferentInputs(t *testing.T) {
	h1 := hashRefreshToken("token-a")
	h2 := hashRefreshToken("token-b")
	testutil.NotEqual(t, h1, h2)
}
