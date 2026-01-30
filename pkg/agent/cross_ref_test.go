// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestExtractWorkloadReferences_EnvFrom(t *testing.T) {
	// Create a Deployment with envFrom references
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "backend",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app",
								"envFrom": []interface{}{
									map[string]interface{}{
										"configMapRef": map[string]interface{}{
											"name": "app-config",
										},
									},
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "db-creds",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	refs := extractWorkloadReferences(deployment)

	if len(refs) != 2 {
		t.Errorf("Expected 2 references, got %d", len(refs))
	}

	// Check ConfigMap reference
	foundCM := false
	foundSecret := false
	for _, ref := range refs {
		if ref.kind == "ConfigMap" && ref.name == "app-config" && ref.refType == "envFrom.configMapRef" {
			foundCM = true
		}
		if ref.kind == "Secret" && ref.name == "db-creds" && ref.refType == "envFrom.secretRef" {
			foundSecret = true
		}
	}

	if !foundCM {
		t.Error("Expected ConfigMap reference 'app-config' not found")
	}
	if !foundSecret {
		t.Error("Expected Secret reference 'db-creds' not found")
	}
}

func TestExtractWorkloadReferences_EnvValueFrom(t *testing.T) {
	// Create a Deployment with env.valueFrom references
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "backend",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app",
								"env": []interface{}{
									map[string]interface{}{
										"name": "DB_HOST",
										"valueFrom": map[string]interface{}{
											"configMapKeyRef": map[string]interface{}{
												"name": "db-config",
												"key":  "host",
											},
										},
									},
									map[string]interface{}{
										"name": "DB_PASSWORD",
										"valueFrom": map[string]interface{}{
											"secretKeyRef": map[string]interface{}{
												"name": "db-secret",
												"key":  "password",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	refs := extractWorkloadReferences(deployment)

	if len(refs) != 2 {
		t.Errorf("Expected 2 references, got %d", len(refs))
	}

	foundCM := false
	foundSecret := false
	for _, ref := range refs {
		if ref.kind == "ConfigMap" && ref.name == "db-config" && ref.refType == "env.valueFrom.configMapKeyRef" {
			foundCM = true
		}
		if ref.kind == "Secret" && ref.name == "db-secret" && ref.refType == "env.valueFrom.secretKeyRef" {
			foundSecret = true
		}
	}

	if !foundCM {
		t.Error("Expected ConfigMap reference 'db-config' not found")
	}
	if !foundSecret {
		t.Error("Expected Secret reference 'db-secret' not found")
	}
}

func TestExtractWorkloadReferences_Volumes(t *testing.T) {
	// Create a Deployment with volume references
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "backend",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app",
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "config-volume",
								"configMap": map[string]interface{}{
									"name": "nginx-config",
								},
							},
							map[string]interface{}{
								"name": "secret-volume",
								"secret": map[string]interface{}{
									"secretName": "tls-certs",
								},
							},
						},
					},
				},
			},
		},
	}

	refs := extractWorkloadReferences(deployment)

	if len(refs) != 2 {
		t.Errorf("Expected 2 references, got %d", len(refs))
	}

	foundCM := false
	foundSecret := false
	for _, ref := range refs {
		if ref.kind == "ConfigMap" && ref.name == "nginx-config" && ref.refType == "volume.configMap" {
			foundCM = true
		}
		if ref.kind == "Secret" && ref.name == "tls-certs" && ref.refType == "volume.secret" {
			foundSecret = true
		}
	}

	if !foundCM {
		t.Error("Expected ConfigMap reference 'nginx-config' not found")
	}
	if !foundSecret {
		t.Error("Expected Secret reference 'tls-certs' not found")
	}
}

func TestExtractWorkloadReferences_ProjectedVolumes(t *testing.T) {
	// Create a Deployment with projected volume references
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "backend",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app",
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "projected-volume",
								"projected": map[string]interface{}{
									"sources": []interface{}{
										map[string]interface{}{
											"configMap": map[string]interface{}{
												"name": "proj-config",
											},
										},
										map[string]interface{}{
											"secret": map[string]interface{}{
												"name": "proj-secret",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	refs := extractWorkloadReferences(deployment)

	if len(refs) != 2 {
		t.Errorf("Expected 2 references, got %d", len(refs))
	}

	foundCM := false
	foundSecret := false
	for _, ref := range refs {
		if ref.kind == "ConfigMap" && ref.name == "proj-config" && ref.refType == "volume.projected.configMap" {
			foundCM = true
		}
		if ref.kind == "Secret" && ref.name == "proj-secret" && ref.refType == "volume.projected.secret" {
			foundSecret = true
		}
	}

	if !foundCM {
		t.Error("Expected ConfigMap reference 'proj-config' not found")
	}
	if !foundSecret {
		t.Error("Expected Secret reference 'proj-secret' not found")
	}
}

func TestExtractWorkloadReferences_Deduplication(t *testing.T) {
	// Create a Deployment that references the same secret multiple times
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "backend",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app",
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "db-creds",
										},
									},
								},
								"env": []interface{}{
									map[string]interface{}{
										"name": "DB_PASSWORD",
										"valueFrom": map[string]interface{}{
											"secretKeyRef": map[string]interface{}{
												"name": "db-creds",
												"key":  "password",
											},
										},
									},
								},
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "secret-volume",
								"secret": map[string]interface{}{
									"secretName": "db-creds",
								},
							},
						},
					},
				},
			},
		},
	}

	refs := extractWorkloadReferences(deployment)

	// Should only have 1 reference due to deduplication
	if len(refs) != 1 {
		t.Errorf("Expected 1 reference (deduplicated), got %d", len(refs))
	}

	if refs[0].kind != "Secret" || refs[0].name != "db-creds" {
		t.Errorf("Expected Secret/db-creds, got %s/%s", refs[0].kind, refs[0].name)
	}
}

func TestExtractWorkloadReferences_InitContainers(t *testing.T) {
	// Create a Deployment with init container references
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "backend",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"initContainers": []interface{}{
							map[string]interface{}{
								"name": "init",
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "init-secret",
										},
									},
								},
							},
						},
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app",
							},
						},
					},
				},
			},
		},
	}

	refs := extractWorkloadReferences(deployment)

	if len(refs) != 1 {
		t.Errorf("Expected 1 reference, got %d", len(refs))
	}

	if refs[0].kind != "Secret" || refs[0].name != "init-secret" {
		t.Errorf("Expected Secret/init-secret, got %s/%s", refs[0].kind, refs[0].name)
	}
}

func TestExtractWorkloadReferences_Empty(t *testing.T) {
	// Create a Deployment with no references
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "simple",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app",
								"image": "nginx",
							},
						},
					},
				},
			},
		},
	}

	refs := extractWorkloadReferences(deployment)

	if len(refs) != 0 {
		t.Errorf("Expected 0 references, got %d", len(refs))
	}
}
