package github

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/nacl/box"
)

func encryptSecret(plaintext, publicKeyB64 string) (string, error) {
	pubKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return "", err
	}
	if len(pubKeyBytes) != 32 {
		return "", fmt.Errorf("github public key length invalid")
	}
	var pubkey [32]byte
	copy(pubkey[:], pubKeyBytes)
	encrypted, err := box.SealAnonymous(nil, []byte(plaintext), &pubkey, rand.Reader)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}
