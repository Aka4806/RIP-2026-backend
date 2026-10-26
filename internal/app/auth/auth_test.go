package auth

import (
	"context"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"strings"
	"testing"
	"time"
)

type memoryBlacklist struct {
	values map[string]bool
	fail   bool
	ttl    time.Duration
	user   int
}

func (b *memoryBlacklist) Contains(_ context.Context, id string) (bool, error) {
	if b.fail {
		return false, errors.New("offline")
	}
	return b.values[id], nil
}
func (b *memoryBlacklist) Add(_ context.Context, id string, u int, ttl time.Duration) error {
	b.values[id] = true
	b.ttl = ttl
	b.user = u
	return nil
}
func TestTokenValidationAndRevocation(t *testing.T) {
	ctx := context.Background()
	b := &memoryBlacklist{values: map[string]bool{}}
	s, err := New(strings.Repeat("k", 32), time.Hour, b)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := s.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Validate(ctx, raw)
	if err != nil || c.Subject != "42" {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, c); err != nil {
		t.Fatal(err)
	}
	if b.ttl <= 0 || b.ttl > time.Hour || b.user != 42 {
		t.Fatal("invalid blacklist TTL or user")
	}
	if _, err := s.Validate(ctx, raw); !errors.Is(err, ErrInvalid) {
		t.Fatal("revoked token accepted")
	}
	fresh, _, _ := s.Issue(42)
	if fresh == raw {
		t.Fatal("token reused")
	}
	b.fail = true
	if _, err := s.Validate(ctx, fresh); !errors.Is(err, ErrUnavailable) {
		t.Fatal("redis failure must fail closed")
	}
	b.fail = false
	for _, kind := range []string{"expired", "missing-exp", "wrong-issuer", "wrong-audience", "wrong-alg", "wrong-key", "wrong-role", "bad-subject"} {
		t.Run(kind, func(t *testing.T) {
			claims := Claims{Role: "creator", RegisteredClaims: jwt.RegisteredClaims{Issuer: Issuer, Audience: jwt.ClaimStrings{"sql-index-api"}, Subject: "42", ID: "test", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
			var alg jwt.SigningMethod = jwt.SigningMethodHS256
			key := s.key
			switch kind {
			case "expired":
				claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
			case "missing-exp":
				claims.ExpiresAt = nil
			case "wrong-issuer":
				claims.Issuer = "other"
			case "wrong-audience":
				claims.Audience = jwt.ClaimStrings{"other"}
			case "wrong-alg":
				alg = jwt.SigningMethodHS384
			case "wrong-key":
				key = []byte(strings.Repeat("z", 32))
			case "wrong-role":
				claims.Role = "guest"
			case "bad-subject":
				claims.Subject = "0"
			}
			token, _ := jwt.NewWithClaims(alg, claims).SignedString(key)
			if _, err := s.Validate(ctx, token); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid token accepted")
			}
		})
	}
}
