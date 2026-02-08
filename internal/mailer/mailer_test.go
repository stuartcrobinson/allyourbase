package mailer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestLogMailerSend(t *testing.T) {
	logger := testutil.DiscardLogger()
	m := NewLogMailer(logger)

	err := m.Send(context.Background(), &Message{
		To:      "user@example.com",
		Subject: "Test Subject",
		HTML:    "<p>Hello</p>",
		Text:    "Hello",
	})
	testutil.NoError(t, err)
}

func TestWebhookMailerSend(t *testing.T) {
	var received webhookPayload
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-AYB-Signature")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	secret := "test-webhook-secret"
	m := NewWebhookMailer(WebhookConfig{
		URL:    srv.URL,
		Secret: secret,
	})

	msg := &Message{
		To:      "user@example.com",
		Subject: "Test",
		HTML:    "<p>Hi</p>",
		Text:    "Hi",
	}
	err := m.Send(context.Background(), msg)
	testutil.NoError(t, err)

	testutil.Equal(t, received.To, "user@example.com")
	testutil.Equal(t, received.Subject, "Test")
	testutil.Equal(t, received.HTML, "<p>Hi</p>")
	testutil.Equal(t, received.Text, "Hi")

	// Verify HMAC signature.
	testutil.True(t, gotSig != "", "signature header should be set")
	payload, _ := json.Marshal(received)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	testutil.Equal(t, gotSig, expectedSig)
}

func TestWebhookMailerNoSecret(t *testing.T) {
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-AYB-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewWebhookMailer(WebhookConfig{URL: srv.URL})
	err := m.Send(context.Background(), &Message{To: "a@b.com", Subject: "x"})
	testutil.NoError(t, err)
	testutil.Equal(t, gotSig, "")
}

func TestWebhookMailerNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := NewWebhookMailer(WebhookConfig{URL: srv.URL})
	err := m.Send(context.Background(), &Message{To: "a@b.com", Subject: "x"})
	testutil.ErrorContains(t, err, "status 500")
}

func TestRenderPasswordReset(t *testing.T) {
	html, text, err := RenderPasswordReset(TemplateData{
		AppName:   "MyApp",
		ActionURL: "https://example.com/reset?token=abc123",
	})
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(html, "Reset your password"), "HTML should contain title")
	testutil.True(t, strings.Contains(html, "MyApp"), "HTML should contain app name")
	testutil.True(t, strings.Contains(html, "https://example.com/reset?token=abc123"), "HTML should contain action URL")
	testutil.True(t, strings.Contains(text, "Reset your password"), "text should contain title")
	testutil.True(t, len(text) > 0, "text fallback should not be empty")
}

func TestRenderVerification(t *testing.T) {
	html, text, err := RenderVerification(TemplateData{
		AppName:   "MyApp",
		ActionURL: "https://example.com/verify?token=xyz",
	})
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(html, "Verify your email"), "HTML should contain title")
	testutil.True(t, strings.Contains(html, "MyApp"), "HTML should contain app name")
	testutil.True(t, strings.Contains(html, "https://example.com/verify?token=xyz"), "HTML should contain action URL")
	testutil.True(t, strings.Contains(text, "Verify your email"), "text should contain title")
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello world", "hello world"},
		{"simple tag", "<p>hello</p>", "hello"},
		{"nested tags", "<div><p>hello</p></div>", "hello"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTML(tt.in)
			testutil.Equal(t, got, tt.want)
		})
	}
}

func TestSMTPMailerConfigValidation(t *testing.T) {
	// Verify NewSMTPMailer creates without panic.
	m := NewSMTPMailer(SMTPConfig{
		Host: "smtp.example.com",
		Port: 587,
		From: "noreply@example.com",
	})
	testutil.True(t, m != nil, "SMTPMailer should not be nil")
}

func TestSMTPMailerAuthTypes(t *testing.T) {
	tests := []struct {
		method string
	}{
		{"PLAIN"},
		{"LOGIN"},
		{"CRAM-MD5"},
		{""},
	}
	for _, tt := range tests {
		m := &SMTPMailer{cfg: SMTPConfig{AuthMethod: tt.method}}
		// Just verify authType() doesn't panic.
		_ = m.authType()
	}
}
