package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type ctxKey struct{}

// UserCtxKey is the context key under which the authenticated user id is stored.
var UserCtxKey = ctxKey{}

// Issuer mints and verifies JWTs for chatdb users.
type Issuer struct {
	secret []byte
	ttl    time.Duration
}

func NewIssuer(secret string) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: 30 * 24 * time.Hour}
}

// HashPassword wraps bcrypt with sensible defaults.
func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword returns nil if password matches the bcrypt hash.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// Issue creates a signed JWT carrying the user id as the subject.
func (i *Issuer) Issue(userID int64) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(i.ttl)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(i.secret)
}

// Parse validates a token string and returns the user id.
func (i *Issuer) Parse(token string) (int64, error) {
	t, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return i.secret, nil
	})
	if err != nil {
		return 0, err
	}
	claims, ok := t.Claims.(*jwt.RegisteredClaims)
	if !ok || !t.Valid {
		return 0, errors.New("invalid token")
	}
	return strconv.ParseInt(claims.Subject, 10, 64)
}

// Middleware enforces a valid Bearer token and stashes the user id in context.
func (i *Issuer) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		if raw == "" {
			http.Error(w, `{"error":"missing token"}`, http.StatusUnauthorized)
			return
		}
		raw = strings.TrimPrefix(raw, "Bearer ")
		raw = strings.TrimSpace(raw)
		uid, err := i.Parse(raw)
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), UserCtxKey, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserID pulls the authenticated user id from request context.
func UserID(r *http.Request) (int64, bool) {
	v, ok := r.Context().Value(UserCtxKey).(int64)
	return v, ok
}
