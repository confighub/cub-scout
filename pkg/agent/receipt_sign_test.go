// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestSignedReceipt_ReceiptV1_SignAndVerify(t *testing.T) {
	privPEM, pubPEM, keyID, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}
	if len(keyID) != 16 {
		t.Fatalf("expected 16-char hex keyID, got %q (len=%d)", keyID, len(keyID))
	}

	stmt := Statement{
		Type:          StatementType,
		PredicateType: PredicateTypeReceiptV1,
		Subject: []Subject{
			{
				Name:   SubjectSchemeK8sLive + "apps/v1/Deployment/prod/api",
				Digest: map[string]string{"sha256": strings.Repeat("a", 64)},
			},
		},
		Predicate: Predicate{
			Version:       PredicateVersion,
			Claim:         "applied matches spec at apps/prod/api/deployment.yaml",
			Scope:         Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
			Verifier:      Verifier{Tool: "cub-scout", Version: "v3.0.0"},
			VerifiedAt:    "2026-11-25T10:00:00Z",
			PredicateName: "applied-matches-spec",
			Verdict:       VerdictPASS,
			Omissions:     []Omission{},
			InputAttestations: []AttestationRef{},
		},
	}
	if err := StampFingerprint(&stmt); err != nil {
		t.Fatalf("StampFingerprint: %v", err)
	}
	payloadBytes, err := json.Marshal(stmt)
	if err != nil {
		t.Fatalf("marshal statement: %v", err)
	}

	signedAt := time.Date(2026, 11, 25, 10, 5, 0, 0, time.UTC)
	env, err := SignReceiptAt(payloadBytes, privPEM, signedAt)
	if err != nil {
		t.Fatalf("SignReceiptAt: %v", err)
	}
	if env.SchemaVersion != SignedReceiptSchemaVersion {
		t.Errorf("schemaVersion = %q, want %q", env.SchemaVersion, SignedReceiptSchemaVersion)
	}
	if env.Algorithm != SignedReceiptAlgorithm {
		t.Errorf("algorithm = %q, want %q", env.Algorithm, SignedReceiptAlgorithm)
	}
	if env.KeyID != keyID {
		t.Errorf("keyId = %q, want %q", env.KeyID, keyID)
	}
	if !strings.HasPrefix(env.PayloadDigest, "sha256:") || len(env.PayloadDigest) != 7+64 {
		t.Errorf("unexpected payloadDigest: %q", env.PayloadDigest)
	}
	if env.SignedAt != "2026-11-25T10:05:00Z" {
		t.Errorf("signedAt = %q, want 2026-11-25T10:05:00Z", env.SignedAt)
	}

	envBytes, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	ver, err := VerifySignedReceipt(envBytes, pubPEM)
	if err != nil {
		t.Fatalf("VerifySignedReceipt failed on valid receipt/v1: %v", err)
	}
	if !ver.Valid {
		t.Fatalf("expected Valid=true, got false (error=%q)", ver.Error)
	}
	if ver.KeyID != keyID {
		t.Errorf("verified keyId = %q, want %q", ver.KeyID, keyID)
	}
	if ver.PayloadDigest != env.PayloadDigest {
		t.Errorf("verified payloadDigest = %q, want %q", ver.PayloadDigest, env.PayloadDigest)
	}
}

func TestSignedReceipt_LandedJSON_SignAndVerify(t *testing.T) {
	privPEM, pubPEM, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}

	landedJSON := []byte(`{
  "command": "landed",
  "answer": {
    "state": "landed",
    "summary": "Deployment/api in prod landed revision main@sha1:abc1234",
    "observedAt": "2026-11-25T10:00:00Z"
  },
  "target": {"kind": "Deployment", "name": "api", "namespace": "prod"},
  "revision": "main@sha1:abc1234",
  "replicas": 3
}`)

	env, err := SignReceipt(landedJSON, privPEM)
	if err != nil {
		t.Fatalf("SignReceipt landed JSON: %v", err)
	}
	envBytes, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	ver, err := VerifySignedReceipt(envBytes, pubPEM)
	if err != nil {
		t.Fatalf("VerifySignedReceipt landed JSON: %v", err)
	}
	if !ver.Valid {
		t.Fatalf("expected Valid=true for landed JSON")
	}
}

func TestSignedReceipt_TamperedPayload_Rejected(t *testing.T) {
	privPEM, pubPEM, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}

	payload := []byte(`{"verdict":"PASS","resource":"Deployment/api"}`)
	env, err := SignReceipt(payload, privPEM)
	if err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}

	// Case 1: Tamper payload without updating payloadDigest.
	tamperedEnv := *env
	tamperedEnv.Payload = json.RawMessage(`{"verdict":"BLOCK","resource":"Deployment/api"}`)
	tamperedBytes, _ := json.Marshal(tamperedEnv)

	ver, err := VerifySignedReceipt(tamperedBytes, pubPEM)
	if err == nil || ver.Valid {
		t.Fatalf("expected tampered payload to be rejected; ver=%+v err=%v", ver, err)
	}
	if !strings.Contains(err.Error(), "digest mismatch") {
		t.Errorf("expected digest mismatch error, got: %v", err)
	}

	// Case 2: Attacker tampers payload AND updates payloadDigest (without private key).
	newDigest, _, err := ComputePayloadDigest(tamperedEnv.Payload)
	if err != nil {
		t.Fatalf("ComputePayloadDigest: %v", err)
	}
	tamperedEnv.PayloadDigest = newDigest
	tamperedBytes2, _ := json.Marshal(tamperedEnv)

	ver2, err := VerifySignedReceipt(tamperedBytes2, pubPEM)
	if err == nil || ver2.Valid {
		t.Fatalf("expected forged digest + tampered payload to be rejected; ver=%+v err=%v", ver2, err)
	}
	if !strings.Contains(err.Error(), "signature verification failed") {
		t.Errorf("expected signature verification failed error, got: %v", err)
	}
}

func TestSignedReceipt_WrongPublicKey_Rejected(t *testing.T) {
	privPEM1, _, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair #1: %v", err)
	}
	_, pubPEM2, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair #2: %v", err)
	}

	env, err := SignReceipt([]byte(`{"state":"landed"}`), privPEM1)
	if err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}
	envBytes, _ := json.Marshal(env)

	ver, err := VerifySignedReceipt(envBytes, pubPEM2)
	if err == nil || ver.Valid {
		t.Fatalf("expected wrong public key to be rejected; ver=%+v err=%v", ver, err)
	}
	if !strings.Contains(err.Error(), "signature verification failed") {
		t.Errorf("expected signature verification failed error, got: %v", err)
	}
}

func TestSignedReceipt_InvalidPEM_Rejected(t *testing.T) {
	privPEM, pubPEM, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}

	// Malformed private key PEM on sign
	if _, err := SignReceipt([]byte(`{"a":1}`), []byte("not-a-pem")); err == nil {
		t.Fatal("expected SignReceipt to reject garbage PEM")
	}

	// Non-Ed25519 key (ECDSA) rejected on sign and verify
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	ecPrivDER, _ := x509.MarshalPKCS8PrivateKey(ecKey)
	ecPrivPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecPrivDER})
	if _, err := SignReceipt([]byte(`{"a":1}`), ecPrivPEM); err == nil {
		t.Fatal("expected SignReceipt to reject non-Ed25519 private key")
	}

	env, err := SignReceipt([]byte(`{"a":1}`), privPEM)
	if err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}
	envBytes, _ := json.Marshal(env)

	// Garbage public key PEM on verify
	if ver, err := VerifySignedReceipt(envBytes, []byte("not-a-pem")); err == nil || ver.Valid {
		t.Fatalf("expected VerifySignedReceipt to reject garbage pubkey PEM")
	}

	// ECDSA public key PEM on verify
	ecPubDER, _ := x509.MarshalPKIXPublicKey(&ecKey.PublicKey)
	ecPubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: ecPubDER})
	if ver, err := VerifySignedReceipt(envBytes, ecPubPEM); err == nil || ver.Valid {
		t.Fatalf("expected VerifySignedReceipt to reject non-Ed25519 public key")
	}

	// Sanity: real pubkey still passes
	if ver, err := VerifySignedReceipt(envBytes, pubPEM); err != nil || !ver.Valid {
		t.Fatalf("expected valid pubkey to pass; err=%v", err)
	}
}

func TestSignedReceipt_UnsupportedAlgorithmAndSchema_Rejected(t *testing.T) {
	privPEM, pubPEM, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}
	env, err := SignReceipt([]byte(`{"k":"v"}`), privPEM)
	if err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}

	badAlg := *env
	badAlg.Algorithm = "rsa-pss"
	b1, _ := json.Marshal(badAlg)
	if ver, err := VerifySignedReceipt(b1, pubPEM); err == nil || ver.Valid || !strings.Contains(err.Error(), "unsupported signature algorithm") {
		t.Fatalf("expected unsupported algorithm error, got ver=%+v err=%v", ver, err)
	}

	badSchema := *env
	badSchema.SchemaVersion = "signed-receipt/v99"
	b2, _ := json.Marshal(badSchema)
	if ver, err := VerifySignedReceipt(b2, pubPEM); err == nil || ver.Valid || !strings.Contains(err.Error(), "unsupported schemaVersion") {
		t.Fatalf("expected unsupported schemaVersion error, got ver=%+v err=%v", ver, err)
	}
}

func TestSignedReceipt_DeterministicDigest_ReorderedJSON(t *testing.T) {
	privPEM, pubPEM, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}

	jsonA := []byte(`{
		"z": 1,
		"a": { "d": true, "b": [3, 2, 1], "c": "hello" },
		"m": 42
	}`)
	jsonB := []byte(`{"m":42,"a":{"c":"hello","b":[3,2,1],"d":true},"z":1}`)

	dA, canonA, err := ComputePayloadDigest(jsonA)
	if err != nil {
		t.Fatalf("ComputePayloadDigest(A): %v", err)
	}
	dB, canonB, err := ComputePayloadDigest(jsonB)
	if err != nil {
		t.Fatalf("ComputePayloadDigest(B): %v", err)
	}
	if dA != dB {
		t.Fatalf("expected identical digests for key-reordered JSON: %s vs %s", dA, dB)
	}
	if string(canonA) != string(canonB) {
		t.Fatalf("expected identical canonical bytes: %s vs %s", canonA, canonB)
	}

	ts := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	envA, err := SignReceiptAt(jsonA, privPEM, ts)
	if err != nil {
		t.Fatalf("SignReceiptAt(A): %v", err)
	}
	envB, err := SignReceiptAt(jsonB, privPEM, ts)
	if err != nil {
		t.Fatalf("SignReceiptAt(B): %v", err)
	}
	if envA.PayloadDigest != envB.PayloadDigest {
		t.Errorf("envelope digests differ: %s vs %s", envA.PayloadDigest, envB.PayloadDigest)
	}
	if envA.Signature != envB.Signature {
		t.Errorf("Ed25519 signatures must be deterministic and identical: %s vs %s", envA.Signature, envB.Signature)
	}

	// Re-formatting the envelope payload with different key order must still verify.
	reorderedEnv := *envA
	reorderedEnv.Payload = json.RawMessage(jsonA)
	reorderedBytes, _ := json.Marshal(reorderedEnv)
	ver, err := VerifySignedReceipt(reorderedBytes, pubPEM)
	if err != nil || !ver.Valid {
		t.Fatalf("reordered payload in envelope should still verify: ver=%+v err=%v", ver, err)
	}
}
