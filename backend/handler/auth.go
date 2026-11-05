package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/form3tech-oss/jwt-go"
)

const tokenIssuer = "socialsphere"
const tokenLifetime = 24 * time.Hour

type authContextKey struct{}

type Authenticator struct {
	key []byte
	now func() time.Time
}

type authClaims struct {
	Username string `json:"username"`
	jwt.StandardClaims
}

func NewAuthenticator(signingKey string) (*Authenticator, error) {
	key := strings.TrimSpace(signingKey)
	upper := strings.ToUpper(key)
	if len(key) < 32 || strings.Contains(upper, "REPLACE_") || strings.Contains(upper, "CHANGE_ME") || strings.Contains(upper, "YOUR_JWT") {
		return nil, errors.New("JWT_SIGNING_KEY must be a private random value of at least 32 bytes, not a placeholder")
	}
	return &Authenticator{key: []byte(key), now: time.Now}, nil
}

func (a *Authenticator) Issue(username string) (string, error) {
	now := a.now()
	claims := authClaims{Username: username, StandardClaims: jwt.StandardClaims{
		Subject: username, Issuer: tokenIssuer, IssuedAt: now.Unix(), ExpiresAt: now.Add(tokenLifetime).Unix(),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.key)
}

func (a *Authenticator) Verify(raw string) (string, error) {
	claims := &authClaims{}
	// Validate times explicitly below, requiring exp/iat/issuer/subject rather than
	// accepting optional zero values. ValidMethods also rejects algorithm substitution.
	parser := jwt.Parser{ValidMethods: []string{"HS256"}, SkipClaimsValidation: true}
	token, err := parser.ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unsupported signing method")
		}
		return a.key, nil
	})
	now := a.now().Unix()
	if err != nil || token == nil || !token.Valid || claims.Username == "" || claims.Subject != claims.Username ||
		claims.Issuer != tokenIssuer || claims.ExpiresAt <= now || claims.IssuedAt <= 0 || claims.IssuedAt > now ||
		claims.NotBefore > now || claims.ExpiresAt <= claims.IssuedAt || claims.ExpiresAt-claims.IssuedAt > int64(tokenLifetime/time.Second) {
		return "", errors.New("invalid or expired token")
	}
	return claims.Username, nil
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields := strings.Fields(r.Header.Get("Authorization"))
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			writeAuthError(w, http.StatusUnauthorized, "Please sign in to continue.")
			return
		}
		username, err := a.Verify(fields[1])
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "Please sign in again.")
			return
		}
		ctx := context.WithValue(r.Context(), authContextKey{}, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AuthenticatedUsername(r *http.Request) (string, bool) {
	username, ok := r.Context().Value(authContextKey{}).(string)
	return username, ok && username != ""
}

func writeAuthJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	writeAuthJSON(w, status, map[string]string{"message": message})
}
