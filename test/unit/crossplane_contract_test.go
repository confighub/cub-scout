package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// loadFixtureUnstructured loads a single YAML document into an Unstructured object.
func loadFixtureUnstructured(t *testing.T, relPath string) *unstructured.Unstructured {
	t.Helper()
	path := filepath.Join(FixturesDir, relPath)
	b, err := os.ReadFile(path)
	require.NoError(t, err)

	var obj map[string]interface{}
	require.NoError(t, yaml.Unmarshal(b, &obj))

	u := &unstructured.Unstructured{Object: obj}
	return u
}

// TestCrossplaneDetectionContract codifies the XR-first ownership rules:
// - Composite label or XR ownerRef is sufficient for Crossplane ownership.
// - Claim labels enrich but are not required.
// - Claim takes precedence over composite when both are present.
func TestCrossplaneDetectionContract(t *testing.T) {
	t.Run("XR-first: composite label implies Crossplane ownership even without claim", func(t *testing.T) {
		u := loadFixtureUnstructured(t, "crossplane/managed-xr-only.yaml")
		own := agent.DetectOwnership(u)
		assert.Equal(t, agent.OwnerCrossplane, own.Type)
		assert.Equal(t, "composite", own.SubType)
		assert.Equal(t, "xpostgresqlinstance-abc123", own.Name)
	})

	t.Run("Claim labels enrich: claim takes precedence over composite", func(t *testing.T) {
		u := loadFixtureUnstructured(t, "crossplane/managed-with-claim.yaml")
		own := agent.DetectOwnership(u)
		assert.Equal(t, agent.OwnerCrossplane, own.Type)
		assert.Equal(t, "claim", own.SubType)
		assert.Equal(t, "ecommerce-cache", own.Name)
		assert.Equal(t, "ecommerce", own.Namespace)
	})

	t.Run("OwnerRef: upbound.io group implies Crossplane ownership", func(t *testing.T) {
		u := loadFixtureUnstructured(t, "crossplane/managed-ownerref.yaml")
		own := agent.DetectOwnership(u)
		assert.Equal(t, agent.OwnerCrossplane, own.Type)
		assert.Equal(t, "instance", own.SubType)
		assert.Equal(t, "staging-db", own.Name)
	})
}
