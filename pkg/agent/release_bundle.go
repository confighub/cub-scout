// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	_ "crypto/sha256" // OCI content verification must not depend on indirect imports.
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	yamlv3 "gopkg.in/yaml.v3"

	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

const (
	ReleaseMaxObjects       = 100
	releaseManifestBytes    = 1 << 20
	releaseLayerBytes       = 8 << 20
	releaseExpandedBytes    = 16 << 20
	releaseRegistryRequests = 16
)

type ReleaseBundleEvidence struct {
	Reference        string `json:"reference"`
	Digest           string `json:"digest"`
	LayerDigest      string `json:"layerDigest,omitempty"`
	Source           string `json:"source"`
	Verified         bool   `json:"verified"`
	Objects          int    `json:"objects"`
	RegistryRequests int64  `json:"registryRequests"`
	RegistryBytes    int64  `json:"registryBytes"`
}

type ReleaseBundle struct {
	Evidence    ReleaseBundleEvidence
	Objects     []*unstructured.Unstructured
	NestedFiles bool
}

func ParseReleaseBundleReference(raw string) (registry.Reference, error) {
	if !strings.HasPrefix(raw, "oci://") || strings.Count(raw, "@") != 1 {
		return registry.Reference{}, fmt.Errorf("bundle requires oci://registry/repository@sha256:<64 lowercase hex digits>; tags are not supported")
	}
	ref, err := registry.ParseReference(strings.TrimPrefix(raw, "oci://"))
	if err != nil || !controllerOCIDigest.MatchString(ref.Reference) {
		return registry.Reference{}, fmt.Errorf("invalid digest-pinned OCI bundle reference")
	}
	return ref, nil
}

// LoadReleaseBundle verifies content in memory. It neither extracts to disk nor
// renders templates. Registry credentials are read using the same ORAS store as cub.
func LoadReleaseBundle(ctx context.Context, raw, layout string, maxObjects int) (ReleaseBundle, error) {
	ref, err := ParseReleaseBundleReference(raw)
	bundle := ReleaseBundle{Evidence: ReleaseBundleEvidence{Reference: raw, Digest: ref.Reference, Source: "registry"}}
	if err != nil {
		return bundle, err
	}
	if maxObjects < 1 || maxObjects > ReleaseMaxObjects {
		return bundle, fmt.Errorf("max objects must be between 1 and %d", ReleaseMaxObjects)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var fetcher content.Fetcher
	var manifestReader io.ReadCloser
	if layout != "" {
		bundle.Evidence.Source = "oci-layout"
		fetcher = oci.NewStorageFromFS(os.DirFS(layout))
		manifestReader, err = fetcher.Fetch(ctx, ocispec.Descriptor{Digest: digest.Digest(ref.Reference)})
	} else {
		repo, repoErr := remote.NewRepository(ref.Registry + "/" + ref.Repository)
		if repoErr != nil {
			return bundle, repoErr
		}
		transport := &releaseRegistryTransport{base: http.DefaultTransport}
		client := &auth.Client{Client: &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: releaseRegistryRedirect}, Cache: auth.NewCache()}
		store, storeErr := credentials.NewStoreFromDocker(credentials.StoreOptions{})
		if storeErr != nil {
			return bundle, fmt.Errorf("read registry credential configuration: %w", storeErr)
		}
		client.Credential = credentials.Credential(store)
		client.SetUserAgent("cub-scout")
		repo.Client = client
		fetcher = repo
		_, manifestReader, err = repo.FetchReference(ctx, ref.Reference)
		if err == nil {
			bundle, err = readReleaseBundle(ctx, bundle, manifestReader, fetcher, maxObjects)
		}
		bundle.Evidence.RegistryRequests = transport.requests.Load()
		bundle.Evidence.RegistryBytes = transport.bytes.Load()
		return bundle, err
	}
	if err != nil {
		return bundle, fmt.Errorf("read bundle manifest: %w", err)
	}
	return readReleaseBundle(ctx, bundle, manifestReader, fetcher, maxObjects)
}

type releaseRegistryTransport struct {
	base     http.RoundTripper
	requests atomic.Int64
	bytes    atomic.Int64
}

func releaseRegistryRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 || req.URL.Scheme != "https" {
		return fmt.Errorf("registry redirect limit or HTTPS boundary exceeded")
	}
	// Signed blob-download redirects do not need registry bearer credentials.
	req.Header.Del("Authorization")
	return nil
}

func (t *releaseRegistryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("release registry reads require HTTPS")
	}
	if t.requests.Load() >= releaseRegistryRequests {
		return nil, fmt.Errorf("registry request budget exhausted")
	}
	t.requests.Add(1)
	resp, err := t.base.RoundTrip(req)
	if err == nil {
		resp.Body = &releaseCountedBody{ReadCloser: resp.Body, total: &t.bytes, remaining: releaseLayerBytes + 1}
	}
	return resp, err
}

type releaseCountedBody struct {
	io.ReadCloser
	total     *atomic.Int64
	remaining int64
}

func (b *releaseCountedBody) Read(p []byte) (int, error) {
	remaining := min(b.remaining, (32<<20)-b.total.Load())
	if remaining <= 0 {
		return 0, fmt.Errorf("registry response exceeds byte limit")
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	b.total.Add(int64(n))
	return n, err
}

func readReleaseBundle(ctx context.Context, bundle ReleaseBundle, reader io.ReadCloser, fetcher content.Fetcher, maxObjects int) (ReleaseBundle, error) {
	manifestBytes, err := readReleaseBlob(reader, bundle.Evidence.Digest, releaseManifestBytes)
	if err != nil {
		return bundle, err
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return bundle, fmt.Errorf("invalid OCI manifest: %w", err)
	}
	if manifest.SchemaVersion != 2 || manifest.MediaType != ocispec.MediaTypeImageManifest || len(manifest.Layers) != 1 {
		return bundle, fmt.Errorf("expected one OCI image manifest with one literal configuration layer; indexes and multiple layers are not supported")
	}
	layer := manifest.Layers[0]
	if !controllerOCIDigest.MatchString(layer.Digest.String()) || layer.Size <= 0 || layer.Size > releaseLayerBytes || len(layer.URLs) > 0 {
		return bundle, fmt.Errorf("invalid, external or oversized configuration layer")
	}
	switch layer.MediaType {
	case ocispec.MediaTypeImageLayer, ocispec.MediaTypeImageLayerGzip, "application/vnd.cncf.flux.content.v1.tar+gzip":
	default:
		return bundle, fmt.Errorf("unsupported layer media type %q; a literal tar or tar+gzip configuration bundle is required", layer.MediaType)
	}
	if err := ctx.Err(); err != nil {
		return bundle, err
	}
	layerReader, err := fetcher.Fetch(ctx, layer)
	if err != nil {
		return bundle, fmt.Errorf("read configuration layer: %w", err)
	}
	blob, err := readReleaseBlob(layerReader, layer.Digest.String(), releaseLayerBytes)
	if err != nil {
		return bundle, err
	}
	if int64(len(blob)) != layer.Size {
		return bundle, fmt.Errorf("configuration layer size does not match descriptor")
	}
	if layer.MediaType != ocispec.MediaTypeImageLayer {
		gz, gzErr := gzip.NewReader(bytes.NewReader(blob))
		if gzErr != nil {
			return bundle, gzErr
		}
		blob, err = io.ReadAll(io.LimitReader(gz, releaseExpandedBytes+1))
		gz.Close()
		if err != nil {
			return bundle, fmt.Errorf("read gzip layer: %w", err)
		}
	}
	if len(blob) > releaseExpandedBytes {
		return bundle, fmt.Errorf("expanded configuration layer exceeds 16 MiB")
	}
	objects, nested, err := decodeReleaseArchive(ctx, blob, maxObjects)
	if err != nil {
		return bundle, err
	}
	bundle.Objects, bundle.NestedFiles = objects, nested
	bundle.Evidence.LayerDigest = layer.Digest.String()
	bundle.Evidence.Objects, bundle.Evidence.Verified = len(objects), true
	return bundle, nil
}

func readReleaseBlob(reader io.ReadCloser, expected string, limit int64) ([]byte, error) {
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("OCI content exceeds byte limit")
	}
	if digest.FromBytes(data).String() != expected {
		return nil, fmt.Errorf("OCI content digest mismatch")
	}
	return data, nil
}

func decodeReleaseArchive(ctx context.Context, data []byte, maxObjects int) ([]*unstructured.Unstructured, bool, error) {
	tr := tar.NewReader(bytes.NewReader(data))
	var objects []*unstructured.Unstructured
	files, identities := map[string]bool{}, map[string]bool{}
	nested := false
	for count := 0; ; count++ {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if count > 1024 {
			return nil, false, fmt.Errorf("configuration archive exceeds 1024 entries")
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, err
		}
		name := strings.TrimPrefix(header.Name, "./")
		clean := path.Clean(name)
		if path.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(name, "\\") || strings.ContainsAny(name, "\x00\r\n\x1b") {
			return nil, false, fmt.Errorf("unsafe configuration archive path")
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg || files[clean] {
			return nil, false, fmt.Errorf("links, special files and duplicate paths are not supported")
		}
		files[clean] = true
		base := strings.ToLower(path.Base(clean))
		if base == "chart.yaml" || base == "kustomization.yaml" || base == "kustomization.yml" || base == "kustomization" || base == ".sourceignore" || strings.HasSuffix(base, ".jsonnet") || strings.HasSuffix(base, ".libsonnet") || strings.Contains(clean, "templates/") {
			return nil, false, fmt.Errorf("bundle contains rendering inputs; supply literal configuration")
		}
		if ext := strings.ToLower(path.Ext(clean)); ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		nested = nested || strings.Contains(clean, "/")
		if header.Size > releaseManifestBytes {
			return nil, false, fmt.Errorf("configuration file exceeds 1 MiB")
		}
		file, err := io.ReadAll(tr)
		if err != nil {
			return nil, false, err
		}
		// Reject duplicate keys before using the existing Kubernetes YAML scalar
		// conversion. A last-key-wins parser could compare different config.
		strict := yamlv3.NewDecoder(bytes.NewReader(file))
		for {
			var node yamlv3.Node
			if err := strict.Decode(&node); err == io.EOF {
				break
			} else if err != nil {
				return nil, false, fmt.Errorf("invalid configuration YAML in %q", clean)
			}
			var value interface{}
			if err := node.Decode(&value); err != nil {
				return nil, false, fmt.Errorf("ambiguous configuration YAML in %q", clean)
			}
		}
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(file), 4096)
		for {
			var raw map[string]interface{}
			if err := decoder.Decode(&raw); err == io.EOF {
				break
			} else if err != nil {
				return nil, false, fmt.Errorf("invalid literal configuration in %q", clean)
			}
			if len(raw) == 0 {
				continue
			}
			obj := &unstructured.Unstructured{Object: raw}
			ref := BoundedResourceRef{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Namespace: obj.GetNamespace(), Name: obj.GetName()}
			// Secrets are retained as desired identities but never read from Kubernetes.
			check := ref
			if check.Kind == "Secret" {
				check.Kind = "ConfigMap"
			}
			if err := check.Validate(); err != nil || obj.GetKind() == "List" {
				return nil, false, fmt.Errorf("bundle needs individually named literal Kubernetes objects in %q", clean)
			}
			id := NewObjectSetObjectID(obj).Key()
			if identities[id] {
				return nil, false, fmt.Errorf("duplicate desired object identity %s", id)
			}
			identities[id] = true
			objects = append(objects, obj)
			if len(objects) > maxObjects {
				return nil, false, fmt.Errorf("bundle exceeds requested object limit %d", maxObjects)
			}
		}
	}
	if len(objects) == 0 {
		return nil, false, fmt.Errorf("bundle contains no literal Kubernetes objects")
	}
	sort.Slice(objects, func(i, j int) bool {
		return NewObjectSetObjectID(objects[i]).Key() < NewObjectSetObjectID(objects[j]).Key()
	})
	return objects, nested, nil
}
