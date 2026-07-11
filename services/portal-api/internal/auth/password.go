package auth

import (
	"errors"
	"fmt"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

var (
	ErrWeakPassword = errors.New("mật khẩu phải ≥ 10 ký tự, có chữ và số")
)

func HashPassword(plain string) (string, error) {
	if err := ValidatePassword(plain); err != nil {
		return "", err
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// HashSecret — passphrase/unlock (không bắt chữ+số như mật khẩu login).
func HashSecret(plain string, minLen int) (string, error) {
	if minLen < 1 {
		minLen = 10
	}
	if len(plain) < minLen {
		return "", fmt.Errorf("passphrase tối thiểu %d ký tự", minLen)
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func ValidatePassword(plain string) error {
	if len(plain) < 10 {
		return ErrWeakPassword
	}
	var hasLetter, hasDigit bool
	for _, r := range plain {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}
