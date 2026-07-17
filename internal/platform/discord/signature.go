package discord

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type SignatureVerifier struct {
	publicKey ed25519.PublicKey
}

func NewSignatureVerifier(publicKeyHex string) (*SignatureVerifier, error) {
	key := strings.TrimSpace(publicKeyHex)
	if key == "" {
		return nil, errors.New("discord public key is empty")
	}

	decoded, err := hex.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("decode discord public key: %w", err)
	}

	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("discord public key size must be %d bytes", ed25519.PublicKeySize)
	}

	return &SignatureVerifier{
		publicKey: ed25519.PublicKey(decoded),
	}, nil
}

func (v *SignatureVerifier) Verify(timestamp, signatureHex string, body []byte) error {
	if strings.TrimSpace(timestamp) == "" {
		return errors.New("missing signature timestamp")
	}
	if strings.TrimSpace(signatureHex) == "" {
		return errors.New("missing signature")
	}

	signature, err := hex.DecodeString(strings.TrimSpace(signatureHex))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("signature size must be %d bytes", ed25519.SignatureSize)
	}

	message := append([]byte(timestamp), body...)
	if !ed25519.Verify(v.publicKey, message, signature) {
		return errors.New("invalid signature")
	}

	return nil
}
