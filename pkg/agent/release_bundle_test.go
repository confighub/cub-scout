// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/internal/releasetest"
	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry/remote"
)

func TestReleaseBundle(t *testing.T) {
	data, err := os.ReadFile("../../examples/oci-release-check/objects.yaml")
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		files   map[string][]byte
		limit   int
		wantErr string
	}{
		{"valid", map[string][]byte{"objects.yaml": data}, 100, ""},
		{"nested", map[string][]byte{"nested/objects.yaml": data}, 100, ""},
		{"too many", map[string][]byte{"objects.yaml": data}, 1, "object limit"},
		{"duplicate", map[string][]byte{"a.yaml": data, "b.yaml": data}, 100, "duplicate desired"},
		{"chart", map[string][]byte{"Chart.yaml": []byte("name: chart")}, 100, "rendering"},
		{"kustomize", map[string][]byte{"kustomization.yaml": []byte("resources: []")}, 100, "rendering"},
		{"template", map[string][]byte{"templates/object.yaml": data}, 100, "rendering"},
		{"jsonnet", map[string][]byte{"objects.yaml": data, "extra.jsonnet": []byte("{}")}, 100, "rendering"},
		{"source ignore", map[string][]byte{"objects.yaml": data, ".sourceignore": []byte("objects.yaml")}, 100, "rendering"},
		{"non literal", map[string][]byte{"values.yaml": []byte("foo: bar")}, 100, "literal"},
		{"duplicate field", map[string][]byte{"objects.yaml": []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n  name: b\n")}, 100, "ambiguous"},
		{"empty", map[string][]byte{"README.md": []byte("test")}, 100, "no literal"},
		{"traversal", map[string][]byte{"../../escape.yaml": data}, 100, "unsafe"},
		{"absolute", map[string][]byte{"/escape.yaml": data}, 100, "unsafe"},
		{"oversized file", map[string][]byte{"big.yaml": bytes.Repeat([]byte("x"), releaseManifestBytes+1)}, 100, "1 MiB"},
		{"expansion limit", map[string][]byte{"big.yaml": bytes.Repeat([]byte("x"), releaseExpandedBytes+1)}, 100, "16 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			ref, err := releasetest.WriteLayout(dir, tc.files)
			require.NoError(t, err)
			b, err := LoadReleaseBundle(context.Background(), ref, dir, tc.limit)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.False(t, b.Evidence.Verified)
				return
			}
			require.NoError(t, err)
			require.True(t, b.Evidence.Verified)
			require.Equal(t, 2, b.Evidence.Objects)
			require.Zero(t, b.Evidence.RegistryRequests)
			require.Equal(t, tc.name == "nested", b.NestedFiles)
			second, err := LoadReleaseBundle(context.Background(), ref, dir, tc.limit)
			require.NoError(t, err)
			require.Equal(t, b, second)
		})
	}
	dir := t.TempDir()
	ref, err := releasetest.WriteLayout(dir, map[string][]byte{"objects.yaml": data})
	require.NoError(t, err)
	_, err = LoadReleaseBundle(context.Background(), "oci://example.invalid/config:latest", dir, 100)
	require.ErrorContains(t, err, "tags")
	_, err = LoadReleaseBundle(context.Background(), ref, dir, 0)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = LoadReleaseBundle(ctx, ref, dir, 100)
	require.ErrorIs(t, err, context.Canceled)
	manifestPath := filepath.Join(dir, "blobs", "sha256", strings.Split(ref, "sha256:")[1])
	manifestData, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(manifestData, &manifest))
	layerPath := filepath.Join(dir, "blobs", "sha256", manifest.Layers[0].Digest.Encoded())
	require.NoError(t, os.WriteFile(layerPath, []byte("tampered"), 0600))
	_, err = LoadReleaseBundle(context.Background(), ref, dir, 100)
	require.ErrorContains(t, err, "digest mismatch")
	require.NoError(t, os.WriteFile(manifestPath, []byte("tampered manifest"), 0600))
	_, err = LoadReleaseBundle(context.Background(), ref, dir, 100)
	require.ErrorContains(t, err, "digest mismatch")
}

func TestReleaseArchiveSpecialEntries(t *testing.T) {
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeChar} {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: "x.yaml", Typeflag: kind, Linkname: "/tmp/x"}))
		require.NoError(t, tw.Close())
		_, _, err := decodeReleaseArchive(context.Background(), buf.Bytes(), 100)
		require.ErrorContains(t, err, "special files")
	}
}

func TestReleaseManifestShapes(t *testing.T) {
	data, err := os.ReadFile("../../examples/oci-release-check/objects.yaml")
	require.NoError(t, err)
	for _, tc := range []struct {
		name, wantErr string
		change        func(*ocispec.Manifest)
	}{
		{"index", "one OCI image manifest", func(m *ocispec.Manifest) { m.MediaType = ocispec.MediaTypeImageIndex }},
		{"multiple layers", "one OCI image manifest", func(m *ocispec.Manifest) { m.Layers = append(m.Layers, m.Layers[0]) }},
		{"no layers", "one OCI image manifest", func(m *ocispec.Manifest) { m.Layers = nil }},
		{"external layer", "external", func(m *ocispec.Manifest) { m.Layers[0].URLs = []string{"https://example.invalid/layer"} }},
		{"helm layer", "unsupported layer", func(m *ocispec.Manifest) {
			m.Layers[0].MediaType = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
		}},
		{"oversized layer", "oversized", func(m *ocispec.Manifest) { m.Layers[0].Size = releaseLayerBytes + 1 }},
		{"wrong layer size", "size does not match", func(m *ocispec.Manifest) { m.Layers[0].Size++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			ref, err := releasetest.WriteLayout(dir, map[string][]byte{"objects.yaml": data})
			require.NoError(t, err)
			original, err := os.ReadFile(filepath.Join(dir, "blobs", "sha256", strings.Split(ref, "sha256:")[1]))
			require.NoError(t, err)
			var manifest ocispec.Manifest
			require.NoError(t, json.Unmarshal(original, &manifest))
			tc.change(&manifest)
			modified, err := json.Marshal(manifest)
			require.NoError(t, err)
			d := digest.FromBytes(modified)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "blobs", "sha256", d.Encoded()), modified, 0600))
			b, err := LoadReleaseBundle(context.Background(), "oci://example.invalid/config@"+d.String(), dir, 100)
			require.ErrorContains(t, err, tc.wantErr)
			require.False(t, b.Evidence.Verified)
		})
	}
}

func TestReleaseRegistryRedirect(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://blob.invalid/content", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer private-registry-token")
	require.NoError(t, releaseRegistryRedirect(req, []*http.Request{req}))
	require.Empty(t, req.Header.Get("Authorization"))
	require.Error(t, releaseRegistryRedirect(req, []*http.Request{req, req, req}))
	req.URL.Scheme = "http"
	require.Error(t, releaseRegistryRedirect(req, nil))
}

func TestReleaseRegistryReadOnly(t *testing.T) {
	data, err := os.ReadFile("../../examples/oci-release-check/objects.yaml")
	require.NoError(t, err)
	dir := t.TempDir()
	ref, err := releasetest.WriteLayout(dir, map[string][]byte{"objects.yaml": data})
	require.NoError(t, err)
	parsed, err := ParseReleaseBundleReference(ref)
	require.NoError(t, err)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		if strings.Contains(r.URL.Path, "/manifests/") {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
		}
		parts := strings.Split(r.URL.Path, "sha256:")
		require.Len(t, parts, 2)
		b, err := os.ReadFile(filepath.Join(dir, "blobs", "sha256", parts[1]))
		require.NoError(t, err)
		w.Header().Set("Docker-Content-Digest", digest.FromBytes(b).String())
		_, _ = w.Write(b)
	}))
	defer server.Close()
	transport := &releaseRegistryTransport{base: server.Client().Transport}
	repo, err := remote.NewRepository(strings.TrimPrefix(server.URL, "https://") + "/config")
	require.NoError(t, err)
	repo.Client = &http.Client{Transport: transport}
	_, reader, err := repo.FetchReference(context.Background(), parsed.Reference)
	require.NoError(t, err)
	b, err := readReleaseBundle(context.Background(), ReleaseBundle{Evidence: ReleaseBundleEvidence{Reference: ref, Digest: parsed.Reference}}, reader, repo, 100)
	require.NoError(t, err)
	require.True(t, b.Evidence.Verified)
	require.EqualValues(t, 2, transport.requests.Load())
	require.Positive(t, transport.bytes.Load())
	transport.requests.Store(releaseRegistryRequests)
	_, err = repo.Fetch(context.Background(), ocispec.Descriptor{Digest: digest.Digest(parsed.Reference)})
	require.ErrorContains(t, err, "budget")
	_, err = readReleaseBlob(io.NopCloser(strings.NewReader("oversized")), "sha256:"+strings.Repeat("a", 64), 2)
	require.ErrorContains(t, err, "limit")
}
