package storage

import (
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestValidateBucket(t *testing.T) {
	tests := []struct {
		name    string
		bucket  string
		wantErr bool
	}{
		{"valid simple", "images", false},
		{"valid with hyphens", "my-bucket", false},
		{"valid with underscores", "my_bucket", false},
		{"valid with digits", "bucket123", false},
		{"empty", "", true},
		{"too long", string(make([]byte, 64)), true},
		{"uppercase", "Images", true},
		{"spaces", "my bucket", true},
		{"dots", "my.bucket", true},
		{"slashes", "my/bucket", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBucket(tt.bucket)
			if tt.wantErr {
				testutil.True(t, err != nil, "expected error for %q", tt.bucket)
			} else {
				testutil.NoError(t, err)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		objName string
		wantErr bool
	}{
		{"valid simple", "photo.jpg", false},
		{"valid nested", "a/b/c/file.txt", false},
		{"empty", "", true},
		{"dot dot", "a/../b", true},
		{"leading slash", "/a/b", true},
		{"too long", string(make([]byte, 1025)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateName(tt.objName)
			if tt.wantErr {
				testutil.True(t, err != nil, "expected error for %q", tt.objName)
			} else {
				testutil.NoError(t, err)
			}
		})
	}
}

func TestSignAndValidateURL(t *testing.T) {
	svc := &Service{signKey: []byte("test-secret-key-for-signing-urls")}

	token := svc.SignURL("images", "photo.jpg", time.Hour)
	testutil.True(t, token != "", "token should not be empty")

	// Parse exp and sig from token.
	var exp, sig string
	for _, part := range splitParams(token) {
		if k, v, ok := splitKV(part); ok {
			switch k {
			case "exp":
				exp = v
			case "sig":
				sig = v
			}
		}
	}

	testutil.True(t, exp != "", "exp should be present")
	testutil.True(t, sig != "", "sig should be present")

	// Valid.
	testutil.True(t, svc.ValidateSignedURL("images", "photo.jpg", exp, sig), "should be valid")

	// Wrong bucket.
	testutil.False(t, svc.ValidateSignedURL("wrong", "photo.jpg", exp, sig), "wrong bucket should fail")

	// Wrong name.
	testutil.False(t, svc.ValidateSignedURL("images", "wrong.jpg", exp, sig), "wrong name should fail")

	// Wrong sig.
	testutil.False(t, svc.ValidateSignedURL("images", "photo.jpg", exp, "badsig"), "wrong sig should fail")

	// Invalid exp.
	testutil.False(t, svc.ValidateSignedURL("images", "photo.jpg", "notanumber", sig), "invalid exp should fail")
}

func TestSignURLExpired(t *testing.T) {
	svc := &Service{signKey: []byte("test-secret-key-for-signing-urls")}

	// Generate a token that expires immediately.
	token := svc.SignURL("b", "f", -time.Second)
	var exp, sig string
	for _, part := range splitParams(token) {
		if k, v, ok := splitKV(part); ok {
			switch k {
			case "exp":
				exp = v
			case "sig":
				sig = v
			}
		}
	}
	testutil.False(t, svc.ValidateSignedURL("b", "f", exp, sig), "expired token should fail")
}

// splitParams splits "k1=v1&k2=v2" into ["k1=v1", "k2=v2"].
func splitParams(s string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '&' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return parts
}

// splitKV splits "k=v" into (k, v, true).
func splitKV(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}
