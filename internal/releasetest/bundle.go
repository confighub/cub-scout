// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package releasetest creates deterministic, unsigned OCI configuration fixtures.
package releasetest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "crypto/sha256" // Register SHA-256 for go-digest in the standalone generator.
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	digest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func WriteLayout(dir string, files map[string][]byte) (string, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := files[name]
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data))}); err != nil {
			return "", err
		}
		if _, err := tw.Write(data); err != nil {
			return "", err
		}
	}
	if err := tw.Close(); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	write := func(data []byte, media string) (ocispec.Descriptor, error) {
		d := digest.FromBytes(data)
		p := filepath.Join(dir, "blobs", "sha256")
		if err := os.MkdirAll(p, 0700); err != nil {
			return ocispec.Descriptor{}, err
		}
		err := os.WriteFile(filepath.Join(p, d.Encoded()), data, 0600)
		return ocispec.Descriptor{MediaType: media, Digest: d, Size: int64(len(data))}, err
	}
	layer, err := write(buf.Bytes(), ocispec.MediaTypeImageLayerGzip)
	if err != nil {
		return "", err
	}
	config, err := write([]byte("{}"), ocispec.MediaTypeImageConfig)
	if err != nil {
		return "", err
	}
	m := ocispec.Manifest{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageManifest, Config: config, Layers: []ocispec.Descriptor{layer}}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	manifest, err := write(data, ocispec.MediaTypeImageManifest)
	if err != nil {
		return "", err
	}
	index, err := json.Marshal(ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageIndex, Manifests: []ocispec.Descriptor{manifest}})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), index, 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0600); err != nil {
		return "", err
	}
	return "oci://example.invalid/config@" + manifest.Digest.String(), nil
}
