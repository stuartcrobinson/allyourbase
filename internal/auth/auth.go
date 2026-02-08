package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

// Sentinel errors returned by the auth service.
var (
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrEmailTaken          = errors.New("email already registered")
	ErrValidation          = errors.New("validation error")
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
	ErrInvalidResetToken   = errors.New("invalid or expired reset token")
	ErrInvalidVerifyToken  = errors.New("invalid or expired verification token")
)

// argon2id parameters. Vars (not consts) so tests can lower them for speed.
var (
	argonMemory  uint32 = 64 * 1024 // 64 MiB
	argonTime    uint32 = 3
	argonThreads uint8  = 2
)

const (
	argonSaltLen = 16
	argonKeyLen  = 32
)

// Service handles user registration, login, and JWT operations.
type Service struct {
	pool       *pgxpool.Pool
	jwtSecret  []byte
	tokenDur   time.Duration
	refreshDur time.Duration
	logger     *slog.Logger
	mailer     mailer.Mailer // nil = email features disabled
	appName    string        // used in email templates
	baseURL    string        // public base URL for action links
}

// User represents a registered user (without password hash).
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Claims are the JWT claims issued by AYB.
type Claims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
}

// NewService creates a new auth service.
func NewService(pool *pgxpool.Pool, jwtSecret string, tokenDuration, refreshDuration time.Duration, logger *slog.Logger) *Service {
	return &Service{
		pool:       pool,
		jwtSecret:  []byte(jwtSecret),
		tokenDur:   tokenDuration,
		refreshDur: refreshDuration,
		logger:     logger,
	}
}

// Register creates a new user and returns the user, an access token, and a refresh token.
func (s *Service) Register(ctx context.Context, email, password string) (*User, string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := validateEmail(email); err != nil {
		return nil, "", "", err
	}
	if err := validatePassword(password); err != nil {
		return nil, "", "", err
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, "", "", fmt.Errorf("hashing password: %w", err)
	}

	var user User
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ($1, $2)
		 RETURNING id, email, created_at, updated_at`,
		email, hash,
	).Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, "", "", ErrEmailTaken
		}
		return nil, "", "", fmt.Errorf("inserting user: %w", err)
	}

	s.logger.Info("user registered", "user_id", user.ID, "email", user.Email)

	// Send verification email (best-effort, don't block registration).
	if s.mailer != nil {
		if err := s.SendVerificationEmail(ctx, user.ID, user.Email); err != nil {
			s.logger.Error("failed to send verification email on register", "error", err)
		}
	}

	return s.issueTokens(ctx, &user)
}

// Login authenticates a user and returns the user, an access token, and a refresh token.
func (s *Service) Login(ctx context.Context, email, password string) (*User, string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var user User
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, created_at, updated_at
		 FROM _ayb_users WHERE LOWER(email) = $1`,
		email,
	).Scan(&user.ID, &user.Email, &hash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", ErrInvalidCredentials
		}
		return nil, "", "", fmt.Errorf("querying user: %w", err)
	}

	ok, err := verifyPassword(hash, password)
	if err != nil {
		return nil, "", "", fmt.Errorf("verifying password: %w", err)
	}
	if !ok {
		return nil, "", "", ErrInvalidCredentials
	}

	return s.issueTokens(ctx, &user)
}

// ValidateToken parses and validates a JWT token string.
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// UserByID fetches a user by ID.
func (s *Service) UserByID(ctx context.Context, id string) (*User, error) {
	var user User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, created_at, updated_at FROM _ayb_users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("querying user: %w", err)
	}
	return &user, nil
}

func (s *Service) generateToken(user *User) (string, error) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenDur)),
		},
		Email: user.Email,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// hashPassword hashes a password using argon2id and returns a PHC-format string.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// verifyPassword checks a password against a PHC-format argon2id hash.
func verifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid hash format")
	}

	var memory uint32
	var iterations uint32
	var threads uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads)
	if err != nil {
		return false, fmt.Errorf("parsing hash params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decoding salt: %w", err)
	}

	expectedKey, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decoding key: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(expectedKey)))
	return subtle.ConstantTimeCompare(key, expectedKey) == 1, nil
}

func validateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("%w: email is required", ErrValidation)
	}
	atIdx := strings.Index(email, "@")
	if atIdx < 1 {
		return fmt.Errorf("%w: invalid email format", ErrValidation)
	}
	domain := email[atIdx+1:]
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("%w: invalid email format", ErrValidation)
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", ErrValidation)
	}
	return nil
}

// RefreshToken validates a refresh token, rotates it, and returns the user
// with a new access token and refresh token.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*User, string, string, error) {
	hash := hashRefreshToken(refreshToken)

	var sessionID, userID string
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id FROM _ayb_sessions
		 WHERE token_hash = $1 AND expires_at > NOW()`,
		hash,
	).Scan(&sessionID, &userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", ErrInvalidRefreshToken
		}
		return nil, "", "", fmt.Errorf("querying session: %w", err)
	}

	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	// Rotate: generate new refresh token and update the session row.
	raw := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", "", fmt.Errorf("generating refresh token: %w", err)
	}
	newPlaintext := base64.RawURLEncoding.EncodeToString(raw)
	newHash := hashRefreshToken(newPlaintext)

	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_sessions SET token_hash = $1, expires_at = $2 WHERE id = $3`,
		newHash, time.Now().Add(s.refreshDur), sessionID,
	)
	if err != nil {
		return nil, "", "", fmt.Errorf("rotating session: %w", err)
	}

	accessToken, err := s.generateToken(user)
	if err != nil {
		return nil, "", "", fmt.Errorf("generating token: %w", err)
	}

	return user, accessToken, newPlaintext, nil
}

// Logout revokes a refresh token by deleting its session.
// Idempotent — returns nil even if the token doesn't match any session.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	hash := hashRefreshToken(refreshToken)
	_, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_sessions WHERE token_hash = $1`, hash,
	)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

const refreshTokenBytes = 32

func hashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// CreateUser creates a user without issuing tokens.
// Used by CLI commands that need to bootstrap users before the server starts.
func CreateUser(ctx context.Context, pool *pgxpool.Pool, email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	var user User
	err = pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ($1, $2)
		 RETURNING id, email, created_at, updated_at`,
		email, hash,
	).Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("inserting user: %w", err)
	}
	return &user, nil
}

// SetMailer configures the mailer for email-based auth flows.
func (s *Service) SetMailer(m mailer.Mailer, appName, baseURL string) {
	s.mailer = m
	s.appName = appName
	if appName == "" {
		s.appName = "AllYourBase"
	}
	s.baseURL = strings.TrimRight(baseURL, "/")
}

const (
	resetTokenBytes  = 32
	resetTokenExpiry = 1 * time.Hour
	verifyTokenBytes = 32
	verifyTokenExpiry = 24 * time.Hour
)

// RequestPasswordReset generates a reset token and emails it to the user.
// Always returns nil to prevent email enumeration — caller should always return 200.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	if s.mailer == nil {
		return nil
	}
	email = strings.ToLower(strings.TrimSpace(email))

	var userID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM _ayb_users WHERE LOWER(email) = $1`, email,
	).Scan(&userID)
	if err != nil {
		// User not found — return nil to prevent enumeration.
		return nil
	}

	// Delete any existing reset tokens for this user.
	_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_password_resets WHERE user_id = $1`, userID)

	// Generate token.
	raw := make([]byte, resetTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("generating reset token: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(plaintext)

	_, err = s.pool.Exec(ctx,
		`INSERT INTO _ayb_password_resets (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, hash, time.Now().Add(resetTokenExpiry),
	)
	if err != nil {
		return fmt.Errorf("inserting reset token: %w", err)
	}

	actionURL := s.baseURL + "/auth/password-reset/confirm?token=" + plaintext
	html, text, err := mailer.RenderPasswordReset(mailer.TemplateData{
		AppName:   s.appName,
		ActionURL: actionURL,
	})
	if err != nil {
		return fmt.Errorf("rendering reset email: %w", err)
	}

	if err := s.mailer.Send(ctx, &mailer.Message{
		To:      email,
		Subject: "Reset your password",
		HTML:    html,
		Text:    text,
	}); err != nil {
		s.logger.Error("failed to send password reset email", "error", err, "email", email)
	}
	return nil
}

// ConfirmPasswordReset validates the token and sets a new password.
func (s *Service) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	hash := hashToken(token)

	var userID string
	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM _ayb_password_resets
		 WHERE token_hash = $1 AND expires_at > NOW()`,
		hash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidResetToken
		}
		return fmt.Errorf("querying reset token: %w", err)
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing new password: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		newHash, userID,
	)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}

	// Delete all reset tokens for this user.
	_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_password_resets WHERE user_id = $1`, userID)

	// Invalidate all existing sessions (force re-login).
	_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_sessions WHERE user_id = $1`, userID)

	s.logger.Info("password reset completed", "user_id", userID)
	return nil
}

// SendVerificationEmail generates a verification token and emails it.
func (s *Service) SendVerificationEmail(ctx context.Context, userID, email string) error {
	if s.mailer == nil {
		return nil
	}

	// Delete any existing verification tokens for this user.
	_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_email_verifications WHERE user_id = $1`, userID)

	raw := make([]byte, verifyTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("generating verification token: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(plaintext)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_email_verifications (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, hash, time.Now().Add(verifyTokenExpiry),
	)
	if err != nil {
		return fmt.Errorf("inserting verification token: %w", err)
	}

	actionURL := s.baseURL + "/auth/verify?token=" + plaintext
	html, text, err := mailer.RenderVerification(mailer.TemplateData{
		AppName:   s.appName,
		ActionURL: actionURL,
	})
	if err != nil {
		return fmt.Errorf("rendering verification email: %w", err)
	}

	if err := s.mailer.Send(ctx, &mailer.Message{
		To:      email,
		Subject: "Verify your email",
		HTML:    html,
		Text:    text,
	}); err != nil {
		s.logger.Error("failed to send verification email", "error", err, "email", email)
	}
	return nil
}

// ConfirmEmail validates the verification token and marks the user's email as verified.
func (s *Service) ConfirmEmail(ctx context.Context, token string) error {
	hash := hashToken(token)

	var userID string
	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM _ayb_email_verifications
		 WHERE token_hash = $1 AND expires_at > NOW()`,
		hash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidVerifyToken
		}
		return fmt.Errorf("querying verification token: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_users SET email_verified = true, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("updating email_verified: %w", err)
	}

	// Delete all verification tokens for this user.
	_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_email_verifications WHERE user_id = $1`, userID)

	s.logger.Info("email verified", "user_id", userID)
	return nil
}

// hashToken hashes a plaintext token with SHA-256 for storage.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Service) createSession(ctx context.Context, userID string) (string, error) {
	raw := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashRefreshToken(plaintext)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_sessions (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, hash, time.Now().Add(s.refreshDur),
	)
	if err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return plaintext, nil
}
