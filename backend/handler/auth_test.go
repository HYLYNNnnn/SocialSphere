package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/form3tech-oss/jwt-go"
)

const testSigningKey = "0123456789abcdef0123456789abcdef"

func testAuthenticator(t *testing.T) *Authenticator {
	t.Helper()
	auth, err := NewAuthenticator(testSigningKey)
	if err != nil {
		t.Fatal(err)
	}
	auth.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	return auth
}

func TestAuthenticatorRejectsUnsafeConfiguration(t *testing.T) {
	for _, key := range []string{"", "short", "REPLACE_WITH_SECURE_JWT_SIGNING_KEY", "YOUR_JWT_SIGNING_KEY_MUST_BE_REPLACED"} {
		if _, err := NewAuthenticator(key); err == nil {
			t.Fatalf("accepted unsafe key %q", key)
		}
	}
}

func TestAuthenticatorRequiresValidSignatureIssuerAndTimes(t *testing.T) {
	auth := testAuthenticator(t)
	valid, err := auth.Issue("alice_123")
	if err != nil {
		t.Fatal(err)
	}
	if username, err := auth.Verify(valid); err != nil || username != "alice_123" {
		t.Fatalf("verify: %q %v", username, err)
	}
	now := auth.now().Unix()
	cases := []struct {
		name   string
		method jwt.SigningMethod
		claims authClaims
		key    interface{}
	}{
		{"expired", jwt.SigningMethodHS256, authClaims{Username: "alice", StandardClaims: jwt.StandardClaims{Subject: "alice", Issuer: tokenIssuer, IssuedAt: now - 100, ExpiresAt: now}}, []byte(testSigningKey)},
		{"missing expiry", jwt.SigningMethodHS256, authClaims{Username: "alice", StandardClaims: jwt.StandardClaims{Subject: "alice", Issuer: tokenIssuer, IssuedAt: now}}, []byte(testSigningKey)},
		{"wrong issuer", jwt.SigningMethodHS256, authClaims{Username: "alice", StandardClaims: jwt.StandardClaims{Subject: "alice", Issuer: "other-app", IssuedAt: now, ExpiresAt: now + 100}}, []byte(testSigningKey)},
		{"missing subject", jwt.SigningMethodHS256, authClaims{Username: "alice", StandardClaims: jwt.StandardClaims{Issuer: tokenIssuer, IssuedAt: now, ExpiresAt: now + 100}}, []byte(testSigningKey)},
		{"future issued-at", jwt.SigningMethodHS256, authClaims{Username: "alice", StandardClaims: jwt.StandardClaims{Subject: "alice", Issuer: tokenIssuer, IssuedAt: now + 50, ExpiresAt: now + 100}}, []byte(testSigningKey)},
		{"wrong algorithm", jwt.SigningMethodHS384, authClaims{Username: "alice", StandardClaims: jwt.StandardClaims{Subject: "alice", Issuer: tokenIssuer, IssuedAt: now, ExpiresAt: now + 100}}, []byte(testSigningKey)},
		{"wrong key", jwt.SigningMethodHS256, authClaims{Username: "alice", StandardClaims: jwt.StandardClaims{Subject: "alice", Issuer: tokenIssuer, IssuedAt: now, ExpiresAt: now + 100}}, []byte("different-private-key-0123456789abcdef")},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			token, err := jwt.NewWithClaims(test.method, test.claims).SignedString(test.key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := auth.Verify(token); err == nil {
				t.Fatal("accepted invalid token")
			}
		})
	}
	if _, err := auth.Verify("not-a-token"); err == nil {
		t.Fatal("accepted malformed token")
	}
}

func TestAuthenticatorMiddlewareRejectsAnonymousAndSetsIdentity(t *testing.T) {
	auth := testAuthenticator(t)
	token, err := auth.Issue("alice_123")
	if err != nil {
		t.Fatal(err)
	}
	wrapped := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, ok := AuthenticatedUsername(r)
		if !ok || username != "alice_123" {
			t.Fatalf("identity: %q %v", username, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, test := range []struct {
		header string
		status int
	}{{"", 401}, {"Basic value", 401}, {"Bearer invalid", 401}, {"Bearer " + token, 204}} {
		request := httptest.NewRequest(http.MethodGet, "/protected", nil)
		request.Header.Set("Authorization", test.header)
		response := httptest.NewRecorder()
		wrapped.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("header %q status=%d", test.header, response.Code)
		}
	}
}
