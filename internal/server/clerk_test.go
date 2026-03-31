package server

import (
	"context"
	"crypto"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	clerkapi "github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/clerktest"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/go-jose/go-jose/v3"
	"github.com/wesm/agentsview/internal/config"
)

func TestClerkVerifierVerifyRequest_PrefersCookie(t *testing.T) {
	goodToken, goodKey := signedSessionToken(
		t,
		"kid-good",
		"https://app.example.test",
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Hour),
	)
	badToken, _ := signedSessionToken(
		t,
		"kid-bad",
		"https://app.example.test",
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Hour),
	)
	verifier, _ := testClerkVerifier(
		t,
		map[string]crypto.PublicKey{"kid-good": goodKey},
		"https://app.example.test",
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req.AddCookie(&http.Cookie{Name: "__session", Value: goodToken})
	req.Header.Set("Authorization", "Bearer "+badToken)

	claims, err := verifier.VerifyRequest(req)
	if err != nil {
		t.Fatalf("VerifyRequest returned error: %v", err)
	}
	if claims.SessionID != "sess_123" {
		t.Fatalf("SessionID = %q, want %q", claims.SessionID, "sess_123")
	}
}

func TestClerkVerifierVerifyRequest_UsesBearerFallback(t *testing.T) {
	token, pubKey := signedSessionToken(
		t,
		"kid-1",
		"https://app.example.test",
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Hour),
	)
	verifier, _ := testClerkVerifier(
		t,
		map[string]crypto.PublicKey{"kid-1": pubKey},
		"https://app.example.test",
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	claims, err := verifier.VerifyRequest(req)
	if err != nil {
		t.Fatalf("VerifyRequest returned error: %v", err)
	}
	if claims.Subject != "user_123" {
		t.Fatalf("Subject = %q, want %q", claims.Subject, "user_123")
	}
}

func TestClerkVerifierVerifyRequest_RejectsExpiredToken(t *testing.T) {
	token, pubKey := signedSessionToken(
		t,
		"kid-1",
		"https://app.example.test",
		time.Now().Add(-2*time.Hour),
		time.Now().Add(-time.Hour),
	)
	verifier, _ := testClerkVerifier(
		t,
		map[string]crypto.PublicKey{"kid-1": pubKey},
		"https://app.example.test",
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req.AddCookie(&http.Cookie{Name: "__session", Value: token})

	if _, err := verifier.VerifyRequest(req); err == nil {
		t.Fatal("expected expired token error")
	}
}

func TestClerkVerifierVerifyRequest_RejectsInvalidSignature(t *testing.T) {
	token, _ := signedSessionToken(
		t,
		"kid-1",
		"https://app.example.test",
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Hour),
	)
	_, wrongKey := signedSessionToken(
		t,
		"kid-1",
		"https://app.example.test",
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Hour),
	)
	verifier, _ := testClerkVerifier(
		t,
		map[string]crypto.PublicKey{"kid-1": wrongKey},
		"https://app.example.test",
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req.AddCookie(&http.Cookie{Name: "__session", Value: token})

	if _, err := verifier.VerifyRequest(req); err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestClerkVerifierVerifyRequest_RefreshesUnknownKID(t *testing.T) {
	token, pubKey := signedSessionToken(
		t,
		"kid-refresh",
		"https://app.example.test",
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Hour),
	)
	verifier, requests := testClerkVerifier(
		t,
		map[string]crypto.PublicKey{"kid-refresh": pubKey},
		"https://app.example.test",
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req.AddCookie(&http.Cookie{Name: "__session", Value: token})

	if _, err := verifier.VerifyRequest(req); err != nil {
		t.Fatalf("VerifyRequest returned error: %v", err)
	}
	if _, err := verifier.VerifyRequest(req); err != nil {
		t.Fatalf("second VerifyRequest returned error: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("JWKS requests = %d, want %d", got, 1)
	}
}

func TestClerkVerifierVerifyRequest_RejectsUnauthorizedAZP(t *testing.T) {
	token, pubKey := signedSessionToken(
		t,
		"kid-1",
		"https://evil.example.test",
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Hour),
	)
	verifier, _ := testClerkVerifier(
		t,
		map[string]crypto.PublicKey{"kid-1": pubKey},
		"https://app.example.test",
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req.AddCookie(&http.Cookie{Name: "__session", Value: token})

	if _, err := verifier.VerifyRequest(req); err == nil {
		t.Fatal("expected unauthorized azp error")
	}
}

func TestAuthMiddleware_StaticTokenFallback(t *testing.T) {
	s := &Server{
		cfg: config.Config{
			RemoteAccess: true,
			AuthToken:    "legacy-token",
		},
	}
	handler := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req.RemoteAddr = "192.168.1.5:1234"
	req.Header.Set("Authorization", "Bearer legacy-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthMiddleware_LegacySSEQueryTokenStillWorks(t *testing.T) {
	s := &Server{
		cfg: config.Config{
			RemoteAccess: true,
			AuthToken:    "legacy-token",
		},
	}
	handler := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/sessions/session-1/watch?token=legacy-token",
		nil,
	)
	req.RemoteAddr = "192.168.1.5:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthMiddleware_ClerkSSEUsesCookieWithoutQueryToken(t *testing.T) {
	token, pubKey := signedSessionToken(
		t,
		"kid-1",
		"https://app.example.test",
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Hour),
	)
	verifier, _ := testClerkVerifier(
		t,
		map[string]crypto.PublicKey{"kid-1": pubKey},
		"https://app.example.test",
	)

	s := &Server{
		cfg: config.Config{
			RemoteAccess:           true,
			ClerkSecretKey:         "sk_test_123",
			ClerkAuthorizedParties: []string{"https://app.example.test"},
		},
		clerkVerifier: verifier,
	}
	handler := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, ok := clerkAuthFromRequest(r)
		if !ok {
			t.Fatal("expected Clerk auth in request context")
		}
		if auth.UserID != "user_123" {
			t.Fatalf("UserID = %q, want %q", auth.UserID, "user_123")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/sessions/session-1/watch",
		nil,
	)
	req.RemoteAddr = "192.168.1.5:1234"
	req.AddCookie(&http.Cookie{Name: "__session", Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func signedSessionToken(
	t *testing.T,
	kid string,
	azp string,
	notBefore time.Time,
	expiry time.Time,
) (string, crypto.PublicKey) {
	t.Helper()
	claims := clerkapi.SessionClaims{
		RegisteredClaims: clerkapi.RegisteredClaims{
			Issuer:    "https://clerk.accounts.dev",
			Subject:   "user_123",
			NotBefore: clerkapi.Int64(notBefore.Unix()),
			IssuedAt:  clerkapi.Int64(notBefore.Unix()),
			Expiry:    clerkapi.Int64(expiry.Unix()),
		},
		Claims: clerkapi.Claims{
			SessionID:       "sess_123",
			AuthorizedParty: azp,
		},
	}
	return clerktest.GenerateJWT(t, claims, kid)
}

func testClerkVerifier(
	t *testing.T,
	keys map[string]crypto.PublicKey,
	authorizedParties ...string,
) (*ClerkVerifier, *atomic.Int32) {
	t.Helper()

	var requests atomic.Int32
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/jwks" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		type response struct {
			Keys []jose.JSONWebKey `json:"keys"`
		}
		body := response{Keys: make([]jose.JSONWebKey, 0, len(keys))}
		for kid, key := range keys {
			body.Keys = append(body.Keys, jose.JSONWebKey{
				Key:       key,
				KeyID:     kid,
				Algorithm: "RS256",
				Use:       "sig",
			})
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Fatalf("encoding jwks response: %v", err)
		}
	}))
	t.Cleanup(jwksServer.Close)

	client := jwks.NewClient(&clerkapi.ClientConfig{
		BackendConfig: clerkapi.BackendConfig{
			Key: clerkapi.String("sk_test_123"),
			URL: clerkapi.String(jwksServer.URL + "/v1"),
		},
	})
	verifier, err := newClerkVerifier(
		"sk_test_123",
		authorizedParties,
		client,
	)
	if err != nil {
		t.Fatalf("newClerkVerifier: %v", err)
	}
	return verifier, &requests
}

func TestClerkAuthStoredInContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKeyClerkAuth, &ClerkAuth{
		UserID: "user_123",
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil).WithContext(ctx)

	auth, ok := clerkAuthFromRequest(req)
	if !ok {
		t.Fatal("expected Clerk auth in context")
	}
	if auth.UserID != "user_123" {
		t.Fatalf("UserID = %q, want %q", auth.UserID, "user_123")
	}
}
