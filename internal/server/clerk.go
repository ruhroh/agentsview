package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	clerkapi "github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	clerkjwt "github.com/clerk/clerk-sdk-go/v2/jwt"
)

var errMissingClerkToken = errors.New("missing Clerk session token")

// ClerkAuth stores the stable Clerk identity fields extracted from a
// verified session token.
type ClerkAuth struct {
	UserID          string
	SessionID       string
	AuthorizedParty string
}

// ClerkVerifier validates Clerk session tokens carried by same-origin
// cookies or fallback bearer tokens.
type ClerkVerifier struct {
	jwksClient *jwks.Client

	mu   sync.RWMutex
	keys map[string]*clerkapi.JSONWebKey
	azp  map[string]struct{}
}

func NewClerkVerifier(
	secretKey string,
	authorizedParties []string,
) (*ClerkVerifier, error) {
	return newClerkVerifier(secretKey, authorizedParties, nil)
}

func newClerkVerifier(
	secretKey string,
	authorizedParties []string,
	client *jwks.Client,
) (*ClerkVerifier, error) {
	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return nil, fmt.Errorf("missing Clerk secret key")
	}
	if len(authorizedParties) == 0 {
		return nil, fmt.Errorf("missing Clerk authorized parties")
	}

	azp := make(map[string]struct{}, len(authorizedParties))
	for _, party := range authorizedParties {
		party = strings.TrimSpace(party)
		if party == "" {
			continue
		}
		azp[party] = struct{}{}
	}
	if len(azp) == 0 {
		return nil, fmt.Errorf("missing Clerk authorized parties")
	}
	if client == nil {
		client = jwks.NewClient(&clerkapi.ClientConfig{
			BackendConfig: clerkapi.BackendConfig{
				Key: clerkapi.String(secretKey),
			},
		})
	}

	return &ClerkVerifier{
		jwksClient: client,
		keys:       make(map[string]*clerkapi.JSONWebKey),
		azp:        azp,
	}, nil
}

func (v *ClerkVerifier) VerifyRequest(
	r *http.Request,
) (*clerkapi.SessionClaims, error) {
	token := clerkTokenFromRequest(r)
	if token == "" {
		return nil, errMissingClerkToken
	}

	unverified, err := clerkjwt.Decode(r.Context(), &clerkjwt.DecodeParams{
		Token: token,
	})
	if err != nil {
		return nil, err
	}
	if unverified.KeyID == "" {
		return nil, fmt.Errorf("missing jwt kid header claim")
	}

	key, err := v.keyForToken(r.Context(), unverified.KeyID)
	if err != nil {
		return nil, err
	}

	return clerkjwt.Verify(r.Context(), &clerkjwt.VerifyParams{
		Token: token,
		JWK:   key,
		AuthorizedPartyHandler: func(azp string) bool {
			_, ok := v.azp[azp]
			return ok
		},
	})
}

func clerkTokenFromRequest(r *http.Request) string {
	if cookie, err := r.Cookie("__session"); err == nil {
		if value := strings.TrimSpace(cookie.Value); value != "" {
			return value
		}
	}

	auth := r.Header.Get("Authorization")
	if token, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return strings.TrimSpace(token)
	}
	return ""
}

func (v *ClerkVerifier) keyForToken(
	ctx context.Context,
	keyID string,
) (*clerkapi.JSONWebKey, error) {
	if key := v.cachedKey(keyID); key != nil {
		return key, nil
	}

	key, err := clerkjwt.GetJSONWebKey(ctx, &clerkjwt.GetJSONWebKeyParams{
		KeyID:      keyID,
		JWKSClient: v.jwksClient,
	})
	if err != nil {
		return nil, err
	}
	v.storeKey(keyID, key)
	return key, nil
}

func (v *ClerkVerifier) cachedKey(
	keyID string,
) *clerkapi.JSONWebKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.keys[keyID]
}

func (v *ClerkVerifier) storeKey(
	keyID string,
	key *clerkapi.JSONWebKey,
) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys[keyID] = key
}
