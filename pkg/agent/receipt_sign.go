// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	// SignedReceiptSchemaVersion is the wire schema version for offline-verifiable
	// signed receipt envelopes (#889).
	SignedReceiptSchemaVersion = "signed-receipt/v1"

	// SignedReceiptAlgorithm is the supported signature algorithm for signed
	// receipt envelopes.
	SignedReceiptAlgorithm = "ed25519"
)

// SignedReceiptEnvelope wraps any canonicalizable JSON receipt payload (such as
// a receipt/v1 in-toto Statement or `cub-scout landed --format json` output)
// with a deterministic SHA-256 payload digest and an offline-verifiable Ed25519
// signature.
type SignedReceiptEnvelope struct {
	SchemaVersion string          `json:"schemaVersion"`
	Algorithm     string          `json:"algorithm"`
	KeyID         string          `json:"keyId"`
	PayloadDigest string          `json:"payloadDigest"`
	Signature     string          `json:"signature"`
	SignedAt      string          `json:"signedAt"`
	Payload       json.RawMessage `json:"payload"`
}

// SignedReceiptVerification is the structured result returned by
// VerifySignedReceipt. On verification failure, Valid is false and Error
// contains the human-readable reason alongside the returned error.
type SignedReceiptVerification struct {
	Valid         bool   `json:"valid"`
	SchemaVersion string `json:"schemaVersion,omitempty"`
	Algorithm     string `json:"algorithm,omitempty"`
	KeyID         string `json:"keyId,omitempty"`
	PayloadDigest string `json:"payloadDigest,omitempty"`
	SignedAt      string `json:"signedAt,omitempty"`
	Error         string `json:"error,omitempty"`
}

// GenerateEd25519KeyPair generates a new Ed25519 key pair encoded as PKCS#8
// private key PEM and PKIX (SubjectPublicKeyInfo) public key PEM, returning
// both PEM blocks and the 16-hex-character keyId derived from SHA-256(pubDER).
func GenerateEd25519KeyPair() (privPEM, pubPEM []byte, keyID string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, "", fmt.Errorf("generate ed25519 keypair: %w", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, "", fmt.Errorf("marshal pkcs8 private key: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, nil, "", fmt.Errorf("marshal pkix public key: %w", err)
	}
	privPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privDER,
	})
	pubPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	})
	return privPEM, pubPEM, ed25519KeyID(pubDER), nil
}

// CanonicalizeJSONPayload parses any valid JSON document and re-marshals it
// deterministically using Go's encoding/json (which sorts object keys
// lexicographically and removes insignificant whitespace while preserving exact
// numeric tokens via UseNumber).
func CanonicalizeJSONPayload(raw []byte) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("empty JSON payload")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("parse JSON payload: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing data after JSON payload")
	}
	canon, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical JSON payload: %w", err)
	}
	return canon, nil
}

// ComputePayloadDigest canonicalizes raw JSON and returns its "sha256:<hex>"
// digest alongside the canonical bytes.
func ComputePayloadDigest(raw []byte) (string, []byte, error) {
	canon, err := CanonicalizeJSONPayload(raw)
	if err != nil {
		return "", nil, err
	}
	return sha256DigestHex(canon), canon, nil
}

// SignReceipt canonicalizes payloadBytes, computes its SHA-256 digest, signs
// the canonical payload with the Ed25519 PKCS#8 PEM private key, and returns
// a SignedReceiptEnvelope stamped with the current UTC time.
func SignReceipt(payloadBytes, privKeyPEM []byte) (*SignedReceiptEnvelope, error) {
	return SignReceiptAt(payloadBytes, privKeyPEM, time.Now().UTC())
}

// SignReceiptAt is the deterministic-timestamp counterpart to SignReceipt.
func SignReceiptAt(payloadBytes, privKeyPEM []byte, signedAt time.Time) (*SignedReceiptEnvelope, error) {
	priv, err := parseEd25519PrivateKeyPEM(privKeyPEM)
	if err != nil {
		return nil, err
	}
	digest, canon, err := ComputePayloadDigest(payloadBytes)
	if err != nil {
		return nil, err
	}

	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("derive ed25519 public key: unexpected key type")
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("marshal ed25519 public key: %w", err)
	}

	sig := ed25519.Sign(priv, canon)
	return &SignedReceiptEnvelope{
		SchemaVersion: SignedReceiptSchemaVersion,
		Algorithm:     SignedReceiptAlgorithm,
		KeyID:         ed25519KeyID(pubDER),
		PayloadDigest: digest,
		Signature:     base64.StdEncoding.EncodeToString(sig),
		SignedAt:      signedAt.UTC().Format(time.RFC3339),
		Payload:       json.RawMessage(canon),
	}, nil
}

// VerifySignedReceipt verifies an offline SignedReceiptEnvelope against an
// Ed25519 PKIX public key PEM. It validates envelope structure, schema version,
// algorithm, payload canonical SHA-256 digest, keyId (when present), and the
// Ed25519 signature over the canonicalized payload.
func VerifySignedReceipt(envelopeBytes, pubKeyPEM []byte) (*SignedReceiptVerification, error) {
	res := &SignedReceiptVerification{}
	fail := func(err error) (*SignedReceiptVerification, error) {
		res.Valid = false
		res.Error = err.Error()
		return res, err
	}

	if len(bytes.TrimSpace(envelopeBytes)) == 0 {
		return fail(fmt.Errorf("signed receipt envelope is empty"))
	}

	var env SignedReceiptEnvelope
	if err := json.Unmarshal(envelopeBytes, &env); err != nil {
		return fail(fmt.Errorf("parse signed receipt envelope: %w", err))
	}
	res.SchemaVersion = env.SchemaVersion
	res.Algorithm = env.Algorithm
	res.KeyID = env.KeyID
	res.PayloadDigest = env.PayloadDigest
	res.SignedAt = env.SignedAt

	if env.SchemaVersion != SignedReceiptSchemaVersion {
		return fail(fmt.Errorf("unsupported schemaVersion %q (expected %q)", env.SchemaVersion, SignedReceiptSchemaVersion))
	}
	if env.Algorithm != SignedReceiptAlgorithm {
		return fail(fmt.Errorf("unsupported signature algorithm %q (expected %q)", env.Algorithm, SignedReceiptAlgorithm))
	}

	pub, pubDER, err := parseEd25519PublicKeyPEM(pubKeyPEM)
	if err != nil {
		return fail(err)
	}

	actualDigest, canon, err := ComputePayloadDigest(env.Payload)
	if err != nil {
		return fail(fmt.Errorf("canonicalize envelope payload: %w", err))
	}
	if strings.TrimSpace(env.PayloadDigest) == "" {
		return fail(fmt.Errorf("missing payloadDigest in signed receipt envelope"))
	}
	if subtle.ConstantTimeCompare([]byte(actualDigest), []byte(env.PayloadDigest)) != 1 {
		return fail(fmt.Errorf("payload digest mismatch: computed %s, envelope has %s", actualDigest, env.PayloadDigest))
	}

	if strings.TrimSpace(env.Signature) == "" {
		return fail(fmt.Errorf("missing signature in signed receipt envelope"))
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(env.Signature))
	if err != nil {
		return fail(fmt.Errorf("decode base64 signature: %w", err))
	}
	if len(sig) != ed25519.SignatureSize {
		return fail(fmt.Errorf("invalid ed25519 signature length %d (expected %d)", len(sig), ed25519.SignatureSize))
	}

	expectedKeyID := ed25519KeyID(pubDER)
	if !ed25519.Verify(pub, canon, sig) {
		if env.KeyID != "" && env.KeyID != expectedKeyID {
			return fail(fmt.Errorf("ed25519 signature verification failed (keyId mismatch: envelope=%s, pubkey=%s)", env.KeyID, expectedKeyID))
		}
		return fail(fmt.Errorf("ed25519 signature verification failed"))
	}

	if env.KeyID != "" && env.KeyID != expectedKeyID {
		return fail(fmt.Errorf("keyId mismatch: envelope=%s, pubkey=%s", env.KeyID, expectedKeyID))
	}

	res.Valid = true
	res.Error = ""
	return res, nil
}

func parseEd25519PrivateKeyPEM(privPEM []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(privPEM)
	if block == nil {
		return nil, fmt.Errorf("invalid private key PEM: no PEM block found")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#8 private key: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unsupported private key type %T (expected Ed25519)", parsed)
	}
	return priv, nil
}

func parseEd25519PublicKeyPEM(pubPEM []byte) (ed25519.PublicKey, []byte, error) {
	block, _ := pem.Decode(pubPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("invalid public key PEM: no PEM block found")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse PKIX public key: %w", err)
	}
	pub, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported public key type %T (expected Ed25519)", parsed)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal canonical PKIX public key: %w", err)
	}
	return pub, pubDER, nil
}

func ed25519KeyID(pubDER []byte) string {
	sum := sha256.Sum256(pubDER)
	return hex.EncodeToString(sum[:])[:16]
}

func sha256DigestHex(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
