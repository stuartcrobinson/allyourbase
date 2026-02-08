package testutil

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// DiscardLogger returns a *slog.Logger that discards all output.
// Use this in tests instead of defining a local discardLogger() helper.
func DiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Equal fails the test if got != want.
func Equal[T comparable](t testing.TB, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// NotEqual fails the test if got == want.
func NotEqual[T comparable](t testing.TB, got, want T) {
	t.Helper()
	if got == want {
		t.Errorf("got %v, should not equal %v", got, want)
	}
}

// NoError fails the test immediately if err is not nil.
func NoError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ErrorContains fails the test if err is nil or doesn't contain substr.
func ErrorContains(t testing.TB, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// True fails the test if condition is false.
func True(t testing.TB, condition bool, msgAndArgs ...any) {
	t.Helper()
	if !condition {
		if len(msgAndArgs) > 0 {
			t.Errorf("expected true: "+fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...)
		} else {
			t.Error("expected true, got false")
		}
	}
}

// False fails the test if condition is true.
func False(t testing.TB, condition bool, msgAndArgs ...any) {
	t.Helper()
	if condition {
		if len(msgAndArgs) > 0 {
			t.Errorf("expected false: "+fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...)
		} else {
			t.Error("expected false, got true")
		}
	}
}

// Nil fails the test if val is not nil.
func Nil(t testing.TB, val any) {
	t.Helper()
	if val != nil {
		t.Errorf("expected nil, got %v", val)
	}
}

// NotNil fails the test immediately if val is nil.
func NotNil(t testing.TB, val any) {
	t.Helper()
	if val == nil {
		t.Fatal("expected non-nil, got nil")
	}
}

// SliceLen fails the test if the slice doesn't have the expected length.
func SliceLen[T any](t testing.TB, slice []T, wantLen int) {
	t.Helper()
	if len(slice) != wantLen {
		t.Errorf("slice length: got %d, want %d", len(slice), wantLen)
	}
}

// MapLen fails the test if the map doesn't have the expected length.
func MapLen[K comparable, V any](t testing.TB, m map[K]V, wantLen int) {
	t.Helper()
	if len(m) != wantLen {
		t.Errorf("map length: got %d, want %d", len(m), wantLen)
	}
}

// Contains fails the test if s does not contain substr.
func Contains(t testing.TB, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("%q does not contain %q", s, substr)
	}
}

