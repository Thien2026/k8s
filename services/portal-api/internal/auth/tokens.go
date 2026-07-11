package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	CookieAccess  = "platform_at"
	CookieRefresh = "platform_rt"
)

type TokenConfig struct {
	Secret     []byte
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Secure     bool
}

type Claims struct {
	UserID int64  `json:"uid"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func NewRefreshToken() (plain string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	hash = HashToken(plain)
	return plain, hash, nil
}

func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func (tc TokenConfig) SignAccess(u User) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: u.ID,
		Email:  u.Email,
		Role:   u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", u.ID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tc.AccessTTL)),
			Issuer:    "platform-console",
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(tc.Secret)
}

func (tc TokenConfig) ParseAccess(token string) (Claims, error) {
	var claims Claims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return tc.Secret, nil
	})
	if err != nil {
		return Claims{}, err
	}
	if !parsed.Valid {
		return Claims{}, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

const (
	StepUpPurposePlatformPolicy = "platform_policy"
	HeaderPlatformStepUp        = "X-Platform-Step-Up"
)

// StepUpClaims — token ngắn hạn sau khi xác thực 2 lớp (login pass + policy unlock).
type StepUpClaims struct {
	UserID  int64  `json:"uid"`
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

func (tc TokenConfig) SignStepUp(userID int64, purpose string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	now := time.Now()
	exp := now.Add(ttl)
	claims := StepUpClaims{
		UserID:  userID,
		Purpose: purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			Issuer:    "platform-console-stepup",
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(tc.Secret)
	return signed, exp, err
}

func (tc TokenConfig) ParseStepUp(token, purpose string) (StepUpClaims, error) {
	var claims StepUpClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return tc.Secret, nil
	})
	if err != nil {
		return StepUpClaims{}, err
	}
	if !parsed.Valid {
		return StepUpClaims{}, jwt.ErrTokenInvalidClaims
	}
	if claims.Purpose != purpose {
		return StepUpClaims{}, fmt.Errorf("step-up purpose không khớp")
	}
	return claims, nil
}
