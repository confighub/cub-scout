// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// These are the public decoder limits documented in examples/helm-expt/README.md.
const (
	testHelmReleaseEncodedLimit    = 9 << 20
	testHelmReleaseCompressedLimit = 6 << 20
	testHelmReleaseJSONLimit       = 32 << 20
)

func gzipAndBase64(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return []byte(base64.StdEncoding.EncodeToString(buf.Bytes()))
}

func validReleaseJSON() []byte {
	return releaseJSON("web", "default", 2, "deployed")
}

func releaseJSON(name, namespace string, version int, status string) []byte {
	return []byte(fmt.Sprintf(`{"name":%q,"namespace":%q,"version":%d,"info":{"status":%q}}`, name, namespace, version, status))
}

func helmSecret(data []byte, name string, version int) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("sh.helm.release.v1.%s.v%d", name, version),
			Namespace: "default",
			Labels:    map[string]string{"owner": "helm", "name": name, "version": fmt.Sprint(version)},
		},
		Type: corev1.SecretType("helm.sh/release.v1"),
		Data: map[string][]byte{"release": data},
	}
}

func TestHelmDecodeRejectsMalformedAndUnidentifiedReleaseRecords(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"malformed base64", []byte("private-sentinel-!!!")},
		{"malformed gzip", []byte(base64.StdEncoding.EncodeToString([]byte("private-sentinel-not-gzip")))},
		{"malformed JSON", gzipAndBase64(t, []byte(`{"name":`))},
		{"invalid typed field", gzipAndBase64(t, []byte(`{"name":"private-sentinel","namespace":"default","version":"bad"}`))},
		{"empty object", gzipAndBase64(t, []byte(`{}`))},
		{"missing name", gzipAndBase64(t, []byte(`{"namespace":"default","version":1}`))},
		{"missing namespace", gzipAndBase64(t, []byte(`{"name":"web","version":1}`))},
		{"empty name", gzipAndBase64(t, []byte(`{"name":"","namespace":"default","version":1}`))},
		{"empty namespace", gzipAndBase64(t, []byte(`{"name":"web","namespace":"","version":1}`))},
		{"zero version", gzipAndBase64(t, []byte(`{"name":"web","namespace":"default","version":0}`))},
		{"negative version", gzipAndBase64(t, []byte(`{"name":"web","namespace":"default","version":-1}`))},
	}
	tracer := NewHelmTracer(fake.NewSimpleClientset())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			release, err := tracer.decodeRelease(tc.data)
			if err == nil || release != nil {
				t.Fatalf("decodeRelease() = (%+v, %v), want error and no release", release, err)
			}
			if strings.Contains(err.Error(), "private-sentinel") {
				t.Fatalf("decoder error leaked payload: %v", err)
			}
		})
	}
	got, err := tracer.decodeRelease(gzipAndBase64(t, validReleaseJSON()))
	if err != nil || got == nil || got.Name != "web" || got.Namespace != "default" || got.Version != 2 {
		t.Fatalf("valid release decode = (%+v, %v)", got, err)
	}
}

func TestHelmDecodeEnforcesEncodedCompressedAndExpandedLimits(t *testing.T) {
	if maxHelmReleaseEncodedBytes != testHelmReleaseEncodedLimit || maxHelmReleaseCompressedBytes != testHelmReleaseCompressedLimit || maxHelmReleaseJSONBytes != testHelmReleaseJSONLimit {
		t.Fatalf("decoder limits = (%d, %d, %d), documented limits = (%d, %d, %d)", maxHelmReleaseEncodedBytes, maxHelmReleaseCompressedBytes, maxHelmReleaseJSONBytes, testHelmReleaseEncodedLimit, testHelmReleaseCompressedLimit, testHelmReleaseJSONLimit)
	}
	tracer := NewHelmTracer(fake.NewSimpleClientset())
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"encoded", []byte(strings.Repeat("A", testHelmReleaseEncodedLimit+1)), "encoded Helm release exceeds size limit"},
		{"compressed", []byte(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x5a}, testHelmReleaseCompressedLimit+1))), "compressed Helm release exceeds size limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tracer.decodeRelease(tc.data); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("decodeRelease error = %v, want size-limit error %q", err, tc.want)
			}
		})
	}
	// A small gzip stream expands to valid JSON larger than the JSON budget.
	largeJSON := []byte(`{"ignored":"` + strings.Repeat("x", testHelmReleaseJSONLimit) + `"}`)
	if _, err := tracer.decodeRelease(gzipAndBase64(t, largeJSON)); err == nil || !strings.Contains(err.Error(), "expanded Helm release exceeds size limit") {
		t.Fatalf("expanded payload error = %v, want explicit expansion limit", err)
	}
}

func TestHelmCandidateDecodeFailureNeverFallsBackToOlderRelease(t *testing.T) {
	valid := helmSecret(gzipAndBase64(t, validReleaseJSON()), "web", 2)
	// Deliberately make the Secret object version look newer while its payload is
	// unreadable. The secret name/labels are enough to identify it as a candidate.
	bad := helmSecret([]byte(base64.StdEncoding.EncodeToString([]byte("private-sentinel-not-gzip"))), "web", 3)
	orders := [][]*corev1.Secret{{valid, bad}, {bad, valid}}
	for order, objects := range orders {
		t.Run(fmt.Sprintf("order-%d", order), func(t *testing.T) {
			client := fake.NewSimpleClientset()
			// The fake tracker may normalize object order. Return each requested
			// order directly so both traversal orders are actually exercised.
			client.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, &corev1.SecretList{Items: []corev1.Secret{*objects[0], *objects[1]}}, nil
			})
			tracer := NewHelmTracer(client)
			if releases, err := tracer.listReleases(context.Background(), "default"); err == nil || len(releases) != 0 || strings.Contains(err.Error(), "private-sentinel") {
				t.Errorf("listReleases returned releases=%v err=%v; want explicit safe incomplete error", releases, err)
			}
			if result, err := tracer.TraceRelease(context.Background(), "web", "default"); err == nil || result != nil || strings.Contains(err.Error(), "private-sentinel") {
				t.Errorf("TraceRelease returned result=%v err=%v; want explicit safe incomplete error", result, err)
			}
			if result, err := tracer.Trace(context.Background(), "Deployment", "web", "default"); err == nil || result != nil || strings.Contains(err.Error(), "private-sentinel") {
				t.Errorf("Trace returned result=%v err=%v; want explicit safe incomplete error", result, err)
			}
			if release, err := tracer.getRelease(context.Background(), "web", "default"); err == nil || release != nil || strings.Contains(err.Error(), "private-sentinel") {
				t.Errorf("getRelease returned release=%v err=%v; want explicit safe incomplete error", release, err)
			}
			if history, err := tracer.GetReleaseHistory(context.Background(), "web", "default"); err == nil || len(history) != 0 || strings.Contains(err.Error(), "private-sentinel") {
				t.Errorf("GetReleaseHistory returned history=%v err=%v; want explicit safe incomplete error", history, err)
			}
		})
	}
}

func TestHelmReleaseSecretListDeniedAndNoRecordsRemainDistinct(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	tracer := NewHelmTracer(client)
	if _, err := tracer.listReleases(ctx, "default"); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("denied list error = %v", err)
	}
	if _, err := tracer.getRelease(ctx, "web", "default"); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("denied get error = %v", err)
	}
	if _, err := tracer.GetReleaseHistory(ctx, "web", "default"); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("denied history error = %v", err)
	}

	tracer = NewHelmTracer(fake.NewSimpleClientset())
	if releases, err := tracer.listReleases(ctx, "default"); err != nil || len(releases) != 0 {
		t.Fatalf("empty release list = %v, %v; want empty nil", releases, err)
	}
	if release, err := tracer.getRelease(ctx, "web", "default"); err != nil || release != nil {
		t.Fatalf("empty direct lookup = %v, %v; want nil nil", release, err)
	}
	if history, err := tracer.GetReleaseHistory(ctx, "web", "default"); err != nil || len(history) != 0 {
		t.Fatalf("empty history = %v, %v; want empty nil", history, err)
	}
}

func TestHelmDecodeReleaseIdentityValidationUsesJSON(t *testing.T) {
	payload := map[string]any{"name": "valid", "namespace": "default", "version": 1}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	tracer := NewHelmTracer(fake.NewSimpleClientset())
	if release, err := tracer.decodeRelease(gzipAndBase64(t, b)); err != nil || release.Name != "valid" {
		t.Fatalf("decode valid metadata = %+v, %v", release, err)
	}
}

// These fixtures mirror the storage metadata written by Helm v3.17.3 and
// v4.0.0 Secret drivers: canonical storage key, owner/name/version labels,
// Secret namespace, and helm.sh/release.v1 type. Payloads are synthetic and
// stay offline; they do not claim a live Helm-version compatibility proof.
func TestHelmReleaseSecretIdentityMetadataMustMatchPayload(t *testing.T) {
	base := helmSecret(gzipAndBase64(t, validReleaseJSON()), "web", 2)
	mutations := []struct {
		name string
		edit func(*corev1.Secret)
	}{
		{"secret namespace mismatch", func(s *corev1.Secret) { s.Namespace = "other" }},
		{"payload namespace mismatch", func(s *corev1.Secret) {
			s.Data["release"] = gzipAndBase64(t, releaseJSON("web", "other", 2, "deployed"))
		}},
		{"payload name mismatch", func(s *corev1.Secret) {
			s.Data["release"] = gzipAndBase64(t, releaseJSON("other", "default", 2, "deployed"))
		}},
		{"payload version mismatch", func(s *corev1.Secret) {
			s.Data["release"] = gzipAndBase64(t, releaseJSON("web", "default", 3, "deployed"))
		}},
		{"storage key mismatch", func(s *corev1.Secret) { s.Name = "sh.helm.release.v1.web.v3" }},
		{"missing owner label", func(s *corev1.Secret) { delete(s.Labels, "owner") }},
		{"release name label mismatch", func(s *corev1.Secret) { s.Labels["name"] = "other" }},
		{"release version label mismatch", func(s *corev1.Secret) { s.Labels["version"] = "3" }},
		{"missing version label", func(s *corev1.Secret) { delete(s.Labels, "version") }},
		{"secret type mismatch", func(s *corev1.Secret) { s.Type = corev1.SecretTypeOpaque }},
	}
	tracer := NewHelmTracer(fake.NewSimpleClientset())
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			secret := base.DeepCopy()
			tc.edit(secret)
			release, err := tracer.decodeReleaseSecret(*secret, "default")
			if err == nil || release != nil {
				t.Fatalf("decodeReleaseSecret = (%+v, %v), want incomplete identity error", release, err)
			}
			if strings.Contains(err.Error(), "deployed") || strings.Contains(err.Error(), "other") {
				t.Fatalf("identity error exposed payload/foreign metadata: %v", err)
			}
		})
	}
}

func TestHelmReleaseEqualRevisionCandidatesAreOrderIndependent(t *testing.T) {
	ctx := context.Background()
	older := helmSecret(gzipAndBase64(t, releaseJSON("web", "default", 1, "superseded")), "web", 1)
	valid := helmSecret(gzipAndBase64(t, validReleaseJSON()), "web", 2)
	conflictingPayload := helmSecret(gzipAndBase64(t, releaseJSON("web", "default", 2, "failed")), "web", 2)
	conflictingIdentity := valid.DeepCopy()
	conflictingIdentity.Name = "custom-secret"

	for _, tc := range []struct {
		name       string
		candidates []corev1.Secret
		wantErr    bool
	}{
		{"identical duplicate evidence", []corev1.Secret{*valid, *valid.DeepCopy()}, false},
		{"equal revision conflicting payload", []corev1.Secret{*valid, *conflictingPayload}, true},
		{"valid plus conflicting same revision identity", []corev1.Secret{*valid, *conflictingIdentity}, true},
		{"valid older plus conflicting latest identity", []corev1.Secret{*older, *conflictingIdentity}, true},
	} {
		for order := 0; order < 2; order++ {
			t.Run(fmt.Sprintf("%s/order-%d", tc.name, order), func(t *testing.T) {
				candidates := append([]corev1.Secret(nil), tc.candidates...)
				if order == 1 {
					candidates[0], candidates[1] = candidates[1], candidates[0]
				}
				client := fake.NewSimpleClientset()
				client.PrependReactor("list", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
					listAction, ok := action.(k8stesting.ListAction)
					if !ok || listAction.GetListRestrictions().Labels.String() != "owner=helm" {
						t.Errorf("release candidate query selector = %v, want owner=helm without a name restriction", action)
					}
					return true, &corev1.SecretList{Items: candidates}, nil
				})
				tracer := NewHelmTracer(client)
				list, listErr := tracer.listReleases(ctx, "default")
				got, getErr := tracer.getRelease(ctx, "web", "default")
				history, historyErr := tracer.GetReleaseHistory(ctx, "web", "default")
				trace, traceErr := tracer.TraceRelease(ctx, "web", "default")
				resourceTrace, resourceTraceErr := tracer.Trace(ctx, "Deployment", "web", "default")
				errs := []error{listErr, getErr, historyErr, traceErr, resourceTraceErr}
				if tc.wantErr {
					for _, err := range errs {
						if err == nil || !strings.Contains(err.Error(), "identity") && !strings.Contains(err.Error(), "ambiguous") {
							t.Errorf("candidate result missing safe ambiguity/identity error: %v", err)
						}
					}
					if list != nil || got != nil || history != nil || trace != nil || resourceTrace != nil {
						t.Errorf("conflicting candidates returned partial results: list=%v get=%v history=%v trace=%v resourceTrace=%v", list, got, history, trace, resourceTrace)
					}
					return
				}
				if listErr != nil || getErr != nil || historyErr != nil || traceErr != nil || resourceTraceErr != nil {
					t.Fatalf("identical duplicate errors: list=%v get=%v history=%v trace=%v resourceTrace=%v", listErr, getErr, historyErr, traceErr, resourceTraceErr)
				}
				if len(list) != 1 || got == nil || got.Version != 2 || len(history) != 1 || len(trace.Chain) < 2 || resourceTrace.FullyManaged {
					t.Fatalf("identical duplicate selection changed valid behavior: list=%v get=%+v history=%v trace=%+v resourceTrace=%+v", list, got, history, trace, resourceTrace)
				}
			})
		}
	}
}
