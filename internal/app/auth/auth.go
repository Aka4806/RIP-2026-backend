package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

const Issuer = "sql-index-lr4"
const Prefix = "lr4:blacklist:"

var ErrInvalid = errors.New("invalid token")
var ErrUnavailable = errors.New("blacklist unavailable")

type Blacklist interface {
	Contains(context.Context, string) (bool, error)
	Add(context.Context, string, int, time.Duration) error
}
type RedisBlacklist struct{ Client *redis.Client }

func (b RedisBlacklist) Contains(ctx context.Context, id string) (bool, error) {
	n, err := b.Client.Exists(ctx, Prefix+id).Result()
	return n > 0, err
}
func (b RedisBlacklist) Add(ctx context.Context, id string, userID int, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	return b.Client.Set(ctx, Prefix+id, userID, ttl).Err()
}

type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}
type Service struct {
	key       []byte
	ttl       time.Duration
	blacklist Blacklist
}

func New(secret string, ttl time.Duration, blacklist Blacklist) (*Service, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT_SECRET must contain at least 32 bytes")
	}
	if ttl <= 0 || ttl > 24*time.Hour {
		return nil, errors.New("JWT_TTL must be positive and at most 24h")
	}
	if blacklist == nil {
		return nil, errors.New("blacklist is required")
	}
	return &Service{[]byte(secret), ttl, blacklist}, nil
}
func (s *Service) Issue(userID int) (string, time.Time, error) {
	if userID <= 0 {
		return "", time.Time{}, ErrInvalid
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	expires := now.Add(s.ttl).Truncate(time.Second)
	claims := Claims{Role: "creator", RegisteredClaims: jwt.RegisteredClaims{
		Issuer: Issuer, Subject: strconv.Itoa(userID), Audience: jwt.ClaimStrings{"sql-index-api"},
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires), ID: hex.EncodeToString(b),
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.key)
	return token, expires, err
}
func (s *Service) Validate(ctx context.Context, raw string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) { return s.key, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuer(Issuer), jwt.WithAudience("sql-index-api"), jwt.WithIssuedAt())
	if err != nil || !token.Valid {
		return nil, ErrInvalid
	}
	id, err := strconv.Atoi(claims.Subject)
	if err != nil || id <= 0 || claims.ID == "" || claims.Role != "creator" {
		return nil, ErrInvalid
	}
	blocked, err := s.blacklist.Contains(ctx, claims.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if blocked {
		return nil, ErrInvalid
	}
	return claims, nil
}
func (s *Service) Revoke(ctx context.Context, c *Claims) error {
	id, err := strconv.Atoi(c.Subject)
	if err != nil {
		return ErrInvalid
	}
	return s.blacklist.Add(ctx, c.ID, id, time.Until(c.ExpiresAt.Time))
}
func UserID(c *gin.Context) int { return c.GetInt("user_id") }

// Optional authentication never makes public reads depend on a supplied token.
func (s *Service) Middleware(required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		fields := strings.Fields(c.GetHeader("Authorization"))
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			if required {
				c.Header("WWW-Authenticate", "Bearer")
				c.AbortWithStatus(http.StatusUnauthorized)
			}
			return
		}
		claims, err := s.Validate(c.Request.Context(), fields[1])
		if err != nil {
			if required {
				if errors.Is(err, ErrUnavailable) {
					c.AbortWithStatus(http.StatusServiceUnavailable)
				} else {
					c.Header("WWW-Authenticate", "Bearer")
					c.AbortWithStatus(http.StatusUnauthorized)
				}
			}
			return
		}
		id, _ := strconv.Atoi(claims.Subject)
		c.Set("user_id", id)
		c.Set("claims", claims)
	}
}
