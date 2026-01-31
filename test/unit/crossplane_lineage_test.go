package unit

import (
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestResolveCrossplaneLineage(t *testing.T) {
	t.Run("managed->XR->claim when all objects are present", func(t *testing.T) {
		managed := LoadFixtureUnstructured(t, "crossplane/lineage-managed-with-xr-and-claim.yaml")
		xr := LoadFixtureUnstructured(t, "crossplane/lineage-xr.yaml")
		claim := LoadFixtureUnstructured(t, "crossplane/lineage-claim.yaml")

		lineage, ok := agent.ResolveCrossplaneLineage(managed, []*unstructured.Unstructured{xr, claim})
		require.True(t, ok)
		require.NotNil(t, lineage)

		assert.True(t, lineage.Managed.Present)
		assert.Equal(t, "Instance", lineage.Managed.Ref.Kind)
		assert.Equal(t, "staging-db", lineage.Managed.Ref.Name)

		assert.True(t, lineage.Composite.Present)
		assert.Equal(t, "XPostgreSQLInstance", lineage.Composite.Ref.Kind)
		assert.Equal(t, "xpostgresqlinstance-abc123", lineage.Composite.Ref.Name)

		require.NotNil(t, lineage.Claim)
		assert.True(t, lineage.Claim.Present)
		assert.Equal(t, "PostgreSQLInstance", lineage.Claim.Ref.Kind)
		assert.Equal(t, "ecommerce-db", lineage.Claim.Ref.Name)
		assert.Equal(t, "ecommerce", lineage.Claim.Ref.Namespace)

		assert.Contains(t, lineage.Evidence, "label:crossplane.io/composite")
		assert.Contains(t, lineage.Evidence, "label:crossplane.io/claim-*")
	})

	t.Run("managed->XR when claim is absent", func(t *testing.T) {
		managed := LoadFixtureUnstructured(t, "crossplane/lineage-managed-xr-only.yaml")
		xr := LoadFixtureUnstructured(t, "crossplane/lineage-xr.yaml")

		lineage, ok := agent.ResolveCrossplaneLineage(managed, []*unstructured.Unstructured{xr})
		require.True(t, ok)
		require.NotNil(t, lineage)

		assert.True(t, lineage.Composite.Present)
		assert.Equal(t, "XPostgreSQLInstance", lineage.Composite.Ref.Kind)
		assert.Equal(t, "xpostgresqlinstance-abc123", lineage.Composite.Ref.Name)
		// Claim metadata exists on XR, but Claim object isn't provided
		require.NotNil(t, lineage.Claim)
		assert.False(t, lineage.Claim.Present)
		assert.Equal(t, "Claim", lineage.Claim.Ref.Kind)
		assert.Equal(t, "ecommerce-db", lineage.Claim.Ref.Name)
		assert.Equal(t, "ecommerce", lineage.Claim.Ref.Namespace)
	})
}
