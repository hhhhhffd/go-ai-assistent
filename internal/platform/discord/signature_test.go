package discord

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestSignatureVerifierValid(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate keys: %v", err)
	}

	verifier, err := NewSignatureVerifier(hex.EncodeToString(publicKey))
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}

	body := []byte(`{"type":1}`)
	timestamp := "1720000000"
	signature := ed25519.Sign(privateKey, append([]byte(timestamp), body...))

	if err := verifier.Verify(timestamp, hex.EncodeToString(signature), body); err != nil {
		t.Fatalf("expected valid signature, got error: %v", err)
	}
}

func TestSignatureVerifierRejectsInvalidSignature(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate keys: %v", err)
	}

	verifier, err := NewSignatureVerifier(hex.EncodeToString(publicKey))
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}

	body := []byte(`{"type":1}`)
	timestamp := "1720000000"
	invalidSignature := make([]byte, ed25519.SignatureSize)
	if err := verifier.Verify(timestamp, hex.EncodeToString(invalidSignature), body); err == nil {
		t.Fatalf("expected invalid signature error")
	}
}
