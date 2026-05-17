package main

import "testing"

func TestParseCubUnitListJSON_AcceptsNestedAndFlatShapes(t *testing.T) {
	raw := []byte(`{
		"items": [
			{
				"Space": {"Slug": "prod", "SpaceID": "sp-1"},
				"Unit": {
					"Slug": "payments-api",
					"UnitID": "u-1",
					"HeadRevisionNum": 9,
					"LiveRevisionNum": 7,
					"TargetSlug": "cluster-a"
				}
			},
			{
				"space": {"slug": "staging", "spaceId": "sp-2"},
				"unit": {
					"slug": "payments-worker",
					"unitId": "u-2",
					"headRevisionNum": "5",
					"liveRevisionNum": "5"
				},
				"target": {"slug": "cluster-b"}
			}
		]
	}`)

	got, err := parseCubUnitListJSON(raw)
	if err != nil {
		t.Fatalf("parseCubUnitListJSON() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].UnitSlug != "payments-api" || got[0].TargetSlug != "cluster-a" || got[0].HeadRevisionNum != 9 || got[0].LiveRevisionNum != 7 {
		t.Fatalf("first entry = %+v, want slug/target/revisions populated", got[0])
	}
	if got[1].UnitSlug != "payments-worker" || got[1].TargetSlug != "cluster-b" || got[1].SpaceSlug != "staging" {
		t.Fatalf("second entry = %+v, want camelCase fields populated", got[1])
	}
}

func TestParseCubTargetListJSON_AcceptsNestedAndFlatShapes(t *testing.T) {
	raw := []byte(`{
		"Targets": [
			{"Target": {"Slug": "k8s-prod", "ProviderType": "Kubernetes", "ToolchainType": "Kubernetes/YAML"}},
			{"target": {"slug": "argo-renderer", "providerType": "ArgoCDRenderer", "toolchainType": "Kubernetes/YAML"}}
		]
	}`)

	got, err := parseCubTargetListJSON(raw)
	if err != nil {
		t.Fatalf("parseCubTargetListJSON() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Slug != "k8s-prod" || got[1].Slug != "argo-renderer" {
		t.Fatalf("got = %+v, want both target slugs", got)
	}
}

func TestParseCubContextJSON_AcceptsCamelAndPascalCase(t *testing.T) {
	raw := []byte(`{
		"name": "alexis@example.com",
		"coordinate": {"serverURL": "https://hub.example.com", "organizationID": "org-1"},
		"settings": {"defaultSpace": "payments"}
	}`)

	ctx, err := parseCubContextJSON(raw)
	if err != nil {
		t.Fatalf("parseCubContextJSON() error = %v", err)
	}
	if ctx.Name != "alexis@example.com" || ctx.Coordinate.ServerURL != "https://hub.example.com" || ctx.Settings.DefaultSpace != "payments" {
		t.Fatalf("ctx = %+v, want parsed context fields", ctx)
	}
}

func TestDecodeCompareUnitMetadataFromGetJSON_AcceptsCamelCase(t *testing.T) {
	raw := `{
		"space": {"slug": "prod", "spaceId": "sp-123"},
		"unit": {
			"slug": "checkout",
			"unitId": "u-123",
			"headRevisionNum": "9",
			"liveRevisionNum": 7,
			"lastAppliedRevisionNum": "8",
			"data": "YXBpVmVyc2lvbjogdjEK"
		}
	}`

	got, err := decodeCompareUnitMetadataFromGetJSON(raw)
	if err != nil {
		t.Fatalf("decodeCompareUnitMetadataFromGetJSON() error = %v", err)
	}
	if got.UnitSlug != "checkout" || got.SpaceName != "prod" || got.HeadRevisionNum != 9 || got.LiveRevisionNum != 7 || got.LastAppliedRevision != 8 {
		t.Fatalf("got = %+v, want tolerant metadata parse", got)
	}
}

func TestParseCubWorkerListJSON_AcceptsNestedWorkerShape(t *testing.T) {
	raw := []byte(`{
		"workers": [
			{"worker": {"slug": "cluster-worker", "cluster": "prod-cluster", "condition": "Connected"}}
		]
	}`)

	got, err := parseCubWorkerListJSON(raw)
	if err != nil {
		t.Fatalf("parseCubWorkerListJSON() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Name != "cluster-worker" || got[0].Cluster != "prod-cluster" || got[0].Condition != "Connected" {
		t.Fatalf("got[0] = %+v, want parsed worker fields", got[0])
	}
}
