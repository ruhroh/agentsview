package server

import (
	"context"
	"net"
	"net/http"
	"strings"

	clerkapi "github.com/clerk/clerk-sdk-go/v2"
)

// contextKey is an unexported type for context keys in this package.
type contextKey int

const (
	// ctxKeyRemoteAuth indicates the request is from an authenticated
	// remote client. When set to true, host-check and CORS middleware
	// skip their restrictions.
	ctxKeyRemoteAuth contextKey = iota
	ctxKeyClerkAuth
)

// isRemoteAuth returns true if the request was authenticated as a
// remote client by the auth middleware.
func isRemoteAuth(r *http.Request) bool {
	v, _ := r.Context().Value(ctxKeyRemoteAuth).(bool)
	return v
}

func clerkAuthFromRequest(r *http.Request) (*ClerkAuth, bool) {
	v, ok := r.Context().Value(ctxKeyClerkAuth).(*ClerkAuth)
	return v, ok
}

// isLocalhostRequest returns true when the request originates from
// a loopback address (127.0.0.0/8, ::1). It checks RemoteAddr,
// which is set by net/http to the client's IP.
func isLocalhostRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// authMiddleware enforces remote API authentication using either
// Clerk sessions or the legacy static bearer token flow. Localhost
// connections always bypass auth when remote access is disabled.
// Non-API routes (static assets) are never gated.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only gate /api/ routes — static assets are always served.
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Read config once for all checks below.
		s.mu.RLock()
		token := s.cfg.AuthToken
		clerkEnabled := s.cfg.ClerkSecretKey != ""
		remoteEnabled := s.cfg.RemoteAccess
		clerkVerifier := s.clerkVerifier
		s.mu.RUnlock()

		// CORS preflight requests (OPTIONS) never include credentials.
		// Let them through so the browser can negotiate CORS before
		// sending the authenticated request. When remote access is
		// enabled with a token, mark OPTIONS as remote-auth so the
		// CORS middleware allows the preflight for cross-origin
		// remote clients.
		if r.Method == http.MethodOptions {
			if remoteEnabled && (token != "" || clerkEnabled) {
				ctx := context.WithValue(r.Context(), ctxKeyRemoteAuth, true)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Localhost bypass: when remote access is disabled, local
		// connections skip auth for backward compatibility. When
		// remote access is enabled with a token, localhost must also
		// authenticate — this prevents bypass via reverse proxy or
		// SSH port-forward where remote clients appear as 127.0.0.1.
		if isLocalhostRequest(r) {
			if !remoteEnabled || token == "" {
				next.ServeHTTP(w, r)
				return
			}
			// Fall through to token check below.
		}

		// When remote access is not enabled, reject non-loopback
		// requests outright. This prevents unauthenticated LAN
		// access when the server is bound to 0.0.0.0. No CORS
		// headers — cross-origin requests are not expected when
		// remote access is off.
		if !remoteEnabled {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		// Remote access enabled but no auth configured yet — reject.
		// No CORS headers — this is a server misconfiguration, not
		// an auth challenge the client can resolve with a token.
		if !clerkEnabled && token == "" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if clerkEnabled {
			if clerkVerifier == nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			claims, err := clerkVerifier.VerifyRequest(r)
			if err != nil {
				setCORSOnAuthError(w, r)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			auth := &ClerkAuth{
				UserID:          claims.Subject,
				SessionID:       claims.SessionID,
				AuthorizedParty: claims.AuthorizedParty,
			}
			ctx := context.WithValue(r.Context(), ctxKeyRemoteAuth, true)
			ctx = context.WithValue(ctx, ctxKeyClerkAuth, auth)
			ctx = clerkapi.ContextWithSessionClaims(ctx, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Check Bearer token in Authorization header. The ?token=
		// query param fallback is restricted to the SSE watch
		// endpoint because EventSource cannot set custom headers.
		// All other endpoints must use the Authorization header.
		var provided string
		auth := r.Header.Get("Authorization")
		if t, ok := strings.CutPrefix(auth, "Bearer "); ok {
			provided = t
		} else if qt := r.URL.Query().Get("token"); qt != "" && strings.HasSuffix(r.URL.Path, "/watch") {
			provided = qt
		} else {
			setCORSOnAuthError(w, r)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if provided != token {
			setCORSOnAuthError(w, r)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Mark this request as authenticated remote so downstream
		// middleware (host-check, CORS) can relax restrictions.
		ctx := context.WithValue(r.Context(), ctxKeyRemoteAuth, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// setCORSOnAuthError adds CORS headers to 401 responses so
// cross-origin browsers can read the auth failure status. Without
// these headers, 401s from authMiddleware (which runs before
// corsMiddleware) become opaque network errors, preventing the
// frontend from detecting auth failures and prompting for a token.
//
// Only used for token-related 401s in remote mode, where the token
// is the access boundary and cross-origin requests are expected.
// Not used for 403s (remote access disabled / no token configured)
// which are not auth challenges the client can resolve.
func setCORSOnAuthError(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	ensureVaryHeader(w.Header(), "Origin")
}
