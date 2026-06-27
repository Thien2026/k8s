package auth

import (
	"errors"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

var (
	ErrWeakPassword = errors.New("mật khẩu phải ≥ 12 ký tự, có chữ và số")
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

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func ValidatePassword(plain string) error {
	if len(plain) < 12 {
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
