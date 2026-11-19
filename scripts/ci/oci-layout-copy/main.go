// Command oci-layout-copy copies one manifest, by digest, from a plain-HTTP
// registry into a local OCI layout. The release-check CI lane uses it: the
// lane's registry is a local kind registry, which release check reads only
// through --oci-layout. It never writes to a registry or a cluster.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: oci-layout-copy <host:port/repository@sha256:digest> <layout-dir>")
		os.Exit(64)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "oci-layout-copy:", err)
		os.Exit(1)
	}
}

func run(ref, dir string) error {
	repository, digest, ok := strings.Cut(ref, "@")
	if !ok || !strings.HasPrefix(digest, "sha256:") {
		return fmt.Errorf("%q is not digest-pinned", ref)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, err := remote.NewRepository(repository)
	if err != nil {
		return err
	}
	repo.PlainHTTP = true
	store, err := oci.New(dir)
	if err != nil {
		return err
	}
	desc, err := oras.Copy(ctx, repo, digest, store, digest, oras.DefaultCopyOptions)
	if err != nil {
		return err
	}
	fmt.Println(desc.Digest)
	return nil
}
