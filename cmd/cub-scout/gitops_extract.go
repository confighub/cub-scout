// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExtractGitOpsConfig extracts configuration from the GitOps resource that manages a workload
func ExtractGitOpsConfig(w *WorkloadInfo) error {
	if w.GitOpsRef == nil {
		return fmt.Errorf("no GitOps reference available")
	}

	var rawConfig string
	var err error

	switch w.GitOpsRef.Kind {
	case "Application":
		rawConfig, err = extractArgoConfig(w.GitOpsRef)
	case "HelmRelease":
		rawConfig, err = extractFluxHelmReleaseConfig(w.GitOpsRef)
	case "Kustomization":
		rawConfig, err = extractFluxKustomizationConfig(w.GitOpsRef)
	case "HelmSecret":
		rawConfig, err = extractNativeHelmConfig(w.GitOpsRef.Namespace, w.GitOpsRef.Name)
	default:
		return fmt.Errorf("unknown GitOps resource kind: %s", w.GitOpsRef.Kind)
	}

	if err != nil {
		w.ConfigError = err
		return err
	}

	// Wrap the extracted config in a ConfigMap so it's valid Kubernetes YAML
	w.ExtractedConfig = wrapInConfigMap(w.Name, w.Namespace, w.GitOpsRef, rawConfig)
	return nil
}

// wrapInConfigMap wraps extracted GitOps config in a Kubernetes ConfigMap
// This is needed because ConfigHub units expect valid K8s manifests
func wrapInConfigMap(name, namespace string, ref *GitOpsReference, rawConfig string) string {
	var sb strings.Builder

	// Build the ConfigMap
	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: ConfigMap\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s-gitops-config\n", name))
	sb.WriteString(fmt.Sprintf("  namespace: %s\n", namespace))
	sb.WriteString("  labels:\n")
	sb.WriteString("    confighub.com/migrated: \"true\"\n")
	sb.WriteString(fmt.Sprintf("    confighub.com/source-kind: %s\n", ref.Kind))
	sb.WriteString(fmt.Sprintf("    confighub.com/source-name: %s\n", ref.Name))
	sb.WriteString("  annotations:\n")
	sb.WriteString(fmt.Sprintf("    confighub.com/source-namespace: %s\n", ref.Namespace))
	sb.WriteString("data:\n")

	// Add the raw config as a data field, properly indented
	sb.WriteString("  config.yaml: |\n")
	for _, line := range strings.Split(rawConfig, "\n") {
		sb.WriteString("    ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

// extractArgoConfig extracts configuration from an Argo CD Application
func extractArgoConfig(ref *GitOpsReference) (string, error) {
	cmd := exec.Command("kubectl", "get", "application.argoproj.io",
		"-n", ref.Namespace, ref.Name, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Argo Application: %w", err)
	}

	var app struct {
		Spec struct {
			Project string `json:"project"`
			Source  struct {
				RepoURL        string `json:"repoURL"`
				Path           string `json:"path"`
				TargetRevision string `json:"targetRevision"`
				Chart          string `json:"chart"`
				Helm           *struct {
					Values     string `json:"values"`
					ValueFiles []string `json:"valueFiles"`
					Parameters []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"parameters"`
					ReleaseName string `json:"releaseName"`
				} `json:"helm"`
				Kustomize *struct {
					Images []string `json:"images"`
				} `json:"kustomize"`
			} `json:"source"`
			Destination struct {
				Server    string `json:"server"`
				Namespace string `json:"namespace"`
				Name      string `json:"name"`
			} `json:"destination"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(output, &app); err != nil {
		return "", fmt.Errorf("failed to parse Argo Application: %w", err)
	}

	// Build header comments
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Migrated from: Argo CD Application %s/%s\n", ref.Namespace, ref.Name))
	if app.Spec.Source.RepoURL != "" {
		sb.WriteString(fmt.Sprintf("# Source: %s", app.Spec.Source.RepoURL))
		if app.Spec.Source.Path != "" {
			sb.WriteString(fmt.Sprintf(" @ %s", app.Spec.Source.Path))
		}
		if app.Spec.Source.TargetRevision != "" {
			sb.WriteString(fmt.Sprintf(" (rev: %s)", app.Spec.Source.TargetRevision))
		}
		sb.WriteString("\n")
	}
	if app.Spec.Source.Chart != "" {
		sb.WriteString(fmt.Sprintf("# Chart: %s\n", app.Spec.Source.Chart))
	}
	if app.Spec.Destination.Namespace != "" {
		sb.WriteString(fmt.Sprintf("# Target namespace: %s\n", app.Spec.Destination.Namespace))
	}

	// Extract Helm values if present
	if app.Spec.Source.Helm != nil {
		if len(app.Spec.Source.Helm.ValueFiles) > 0 {
			sb.WriteString(fmt.Sprintf("# Value files: %s\n", strings.Join(app.Spec.Source.Helm.ValueFiles, ", ")))
		}

		sb.WriteString("---\n")

		// If there are inline values, include them
		if app.Spec.Source.Helm.Values != "" {
			sb.WriteString(app.Spec.Source.Helm.Values)
			if !strings.HasSuffix(app.Spec.Source.Helm.Values, "\n") {
				sb.WriteString("\n")
			}
		}

		// If there are parameters, convert to YAML
		if len(app.Spec.Source.Helm.Parameters) > 0 {
			params := make(map[string]string)
			for _, p := range app.Spec.Source.Helm.Parameters {
				params[p.Name] = p.Value
			}
			if app.Spec.Source.Helm.Values == "" {
				// Only add params if no inline values (avoid duplication)
				yamlBytes, _ := yaml.Marshal(params)
				sb.Write(yamlBytes)
			} else {
				sb.WriteString("\n# Additional parameters:\n")
				yamlBytes, _ := yaml.Marshal(params)
				sb.Write(yamlBytes)
			}
		}
	} else if app.Spec.Source.Kustomize != nil {
		// Kustomize-based Argo Application
		sb.WriteString("---\n")
		sb.WriteString("# Kustomize-based application\n")
		if len(app.Spec.Source.Kustomize.Images) > 0 {
			sb.WriteString("# Image overrides:\n")
			for _, img := range app.Spec.Source.Kustomize.Images {
				sb.WriteString(fmt.Sprintf("#   - %s\n", img))
			}
		}
		sb.WriteString("source:\n")
		sb.WriteString(fmt.Sprintf("  repoURL: %s\n", app.Spec.Source.RepoURL))
		sb.WriteString(fmt.Sprintf("  path: %s\n", app.Spec.Source.Path))
		sb.WriteString(fmt.Sprintf("  targetRevision: %s\n", app.Spec.Source.TargetRevision))
	} else {
		// Plain manifest source
		sb.WriteString("---\n")
		sb.WriteString("source:\n")
		sb.WriteString(fmt.Sprintf("  repoURL: %s\n", app.Spec.Source.RepoURL))
		sb.WriteString(fmt.Sprintf("  path: %s\n", app.Spec.Source.Path))
		sb.WriteString(fmt.Sprintf("  targetRevision: %s\n", app.Spec.Source.TargetRevision))
	}

	return sb.String(), nil
}

// extractFluxHelmReleaseConfig extracts configuration from a Flux HelmRelease
func extractFluxHelmReleaseConfig(ref *GitOpsReference) (string, error) {
	cmd := exec.Command("kubectl", "get", "helmrelease.helm.toolkit.fluxcd.io",
		"-n", ref.Namespace, ref.Name, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Flux HelmRelease: %w", err)
	}

	var hr struct {
		Spec struct {
			Chart struct {
				Spec struct {
					Chart     string `json:"chart"`
					Version   string `json:"version"`
					SourceRef struct {
						Kind      string `json:"kind"`
						Name      string `json:"name"`
						Namespace string `json:"namespace"`
					} `json:"sourceRef"`
				} `json:"spec"`
			} `json:"chart"`
			Values     map[string]interface{} `json:"values"`
			ValuesFrom []struct {
				Kind       string `json:"kind"`
				Name       string `json:"name"`
				ValuesKey  string `json:"valuesKey"`
				TargetPath string `json:"targetPath"`
				Optional   bool   `json:"optional"`
			} `json:"valuesFrom"`
			Interval    string `json:"interval"`
			ReleaseName string `json:"releaseName"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(output, &hr); err != nil {
		return "", fmt.Errorf("failed to parse Flux HelmRelease: %w", err)
	}

	// Build header comments
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Migrated from: Flux HelmRelease %s/%s\n", ref.Namespace, ref.Name))

	chartSpec := hr.Spec.Chart.Spec
	if chartSpec.Chart != "" {
		version := chartSpec.Version
		if version == "" {
			version = "latest"
		}
		sb.WriteString(fmt.Sprintf("# Chart: %s @ %s\n", chartSpec.Chart, version))
	}

	if chartSpec.SourceRef.Name != "" {
		sourceNs := chartSpec.SourceRef.Namespace
		if sourceNs == "" {
			sourceNs = ref.Namespace
		}
		sb.WriteString(fmt.Sprintf("# Source: %s %s/%s\n",
			chartSpec.SourceRef.Kind, sourceNs, chartSpec.SourceRef.Name))
	}

	// Note valuesFrom references (don't resolve for security)
	if len(hr.Spec.ValuesFrom) > 0 {
		sb.WriteString("#\n# NOTE: Original HelmRelease used valuesFrom:\n")
		for _, vf := range hr.Spec.ValuesFrom {
			optional := ""
			if vf.Optional {
				optional = " (optional)"
			}
			sb.WriteString(fmt.Sprintf("#   - %s: %s%s\n", vf.Kind, vf.Name, optional))
		}
		sb.WriteString("# These references were not resolved during migration.\n")
	}

	sb.WriteString("---\n")

	// Include inline values
	if len(hr.Spec.Values) > 0 {
		yamlBytes, err := yaml.Marshal(hr.Spec.Values)
		if err != nil {
			return "", fmt.Errorf("failed to marshal values: %w", err)
		}
		sb.Write(yamlBytes)
	} else {
		sb.WriteString("# No inline values defined\n")
	}

	return sb.String(), nil
}

// extractFluxKustomizationConfig extracts configuration from a Flux Kustomization
func extractFluxKustomizationConfig(ref *GitOpsReference) (string, error) {
	cmd := exec.Command("kubectl", "get", "kustomization.kustomize.toolkit.fluxcd.io",
		"-n", ref.Namespace, ref.Name, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Flux Kustomization: %w", err)
	}

	var ks struct {
		Spec struct {
			Path      string `json:"path"`
			SourceRef struct {
				Kind      string `json:"kind"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"sourceRef"`
			TargetNamespace string `json:"targetNamespace"`
			Interval        string `json:"interval"`
			Prune           bool   `json:"prune"`
			PostBuild       *struct {
				Substitute     map[string]string `json:"substitute"`
				SubstituteFrom []struct {
					Kind string `json:"kind"`
					Name string `json:"name"`
				} `json:"substituteFrom"`
			} `json:"postBuild"`
			Patches []struct {
				Patch  string `json:"patch"`
				Target struct {
					Group     string `json:"group"`
					Version   string `json:"version"`
					Kind      string `json:"kind"`
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"target"`
			} `json:"patches"`
			Images []struct {
				Name    string `json:"name"`
				NewName string `json:"newName"`
				NewTag  string `json:"newTag"`
				Digest  string `json:"digest"`
			} `json:"images"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(output, &ks); err != nil {
		return "", fmt.Errorf("failed to parse Flux Kustomization: %w", err)
	}

	// Build header comments
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Migrated from: Flux Kustomization %s/%s\n", ref.Namespace, ref.Name))

	if ks.Spec.SourceRef.Name != "" {
		sourceNs := ks.Spec.SourceRef.Namespace
		if sourceNs == "" {
			sourceNs = ref.Namespace
		}
		sb.WriteString(fmt.Sprintf("# Source: %s %s/%s\n",
			ks.Spec.SourceRef.Kind, sourceNs, ks.Spec.SourceRef.Name))
	}
	if ks.Spec.Path != "" {
		sb.WriteString(fmt.Sprintf("# Path: %s\n", ks.Spec.Path))
	}
	if ks.Spec.TargetNamespace != "" {
		sb.WriteString(fmt.Sprintf("# Target namespace: %s\n", ks.Spec.TargetNamespace))
	}

	sb.WriteString("---\n")

	// Include postBuild substitutions if present
	if ks.Spec.PostBuild != nil && len(ks.Spec.PostBuild.Substitute) > 0 {
		sb.WriteString("# Variable substitutions from postBuild:\n")
		yamlBytes, _ := yaml.Marshal(ks.Spec.PostBuild.Substitute)
		sb.Write(yamlBytes)
		sb.WriteString("\n")
	}

	// Note substituteFrom references
	if ks.Spec.PostBuild != nil && len(ks.Spec.PostBuild.SubstituteFrom) > 0 {
		sb.WriteString("# NOTE: Original Kustomization used substituteFrom:\n")
		for _, sf := range ks.Spec.PostBuild.SubstituteFrom {
			sb.WriteString(fmt.Sprintf("#   - %s: %s\n", sf.Kind, sf.Name))
		}
	}

	// Include image overrides
	if len(ks.Spec.Images) > 0 {
		sb.WriteString("\n# Image overrides:\n")
		sb.WriteString("images:\n")
		for _, img := range ks.Spec.Images {
			sb.WriteString(fmt.Sprintf("  - name: %s\n", img.Name))
			if img.NewName != "" {
				sb.WriteString(fmt.Sprintf("    newName: %s\n", img.NewName))
			}
			if img.NewTag != "" {
				sb.WriteString(fmt.Sprintf("    newTag: %s\n", img.NewTag))
			}
			if img.Digest != "" {
				sb.WriteString(fmt.Sprintf("    digest: %s\n", img.Digest))
			}
		}
	}

	// Include patches
	if len(ks.Spec.Patches) > 0 {
		sb.WriteString("\n# Patches:\n")
		sb.WriteString("patches:\n")
		for _, p := range ks.Spec.Patches {
			sb.WriteString("  - target:\n")
			if p.Target.Kind != "" {
				sb.WriteString(fmt.Sprintf("      kind: %s\n", p.Target.Kind))
			}
			if p.Target.Name != "" {
				sb.WriteString(fmt.Sprintf("      name: %s\n", p.Target.Name))
			}
			if p.Patch != "" {
				sb.WriteString("    patch: |\n")
				for _, line := range strings.Split(p.Patch, "\n") {
					sb.WriteString(fmt.Sprintf("      %s\n", line))
				}
			}
		}
	}

	// If nothing was extracted, note the source reference
	if ks.Spec.PostBuild == nil && len(ks.Spec.Images) == 0 && len(ks.Spec.Patches) == 0 {
		sb.WriteString("# Kustomization has no inline configuration.\n")
		sb.WriteString("# Configuration is defined in the source repository.\n")
		sb.WriteString("source:\n")
		sb.WriteString(fmt.Sprintf("  kind: %s\n", ks.Spec.SourceRef.Kind))
		sb.WriteString(fmt.Sprintf("  name: %s\n", ks.Spec.SourceRef.Name))
		sb.WriteString(fmt.Sprintf("  path: %s\n", ks.Spec.Path))
	}

	return sb.String(), nil
}

// extractNativeHelmConfig extracts configuration from a native Helm release Secret
func extractNativeHelmConfig(namespace, releaseName string) (string, error) {
	// Find the latest release secret
	cmd := exec.Command("kubectl", "get", "secret",
		"-n", namespace,
		"-l", fmt.Sprintf("name=%s,owner=helm", releaseName),
		"-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Helm release secrets: %w", err)
	}

	var secretList struct {
		Items []struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			Data map[string]string `json:"data"`
			Type string            `json:"type"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &secretList); err != nil {
		return "", fmt.Errorf("failed to parse Helm secrets: %w", err)
	}

	if len(secretList.Items) == 0 {
		return "", fmt.Errorf("no Helm release secrets found for %s/%s", namespace, releaseName)
	}

	// Find the latest version (highest version number)
	var latestSecret *struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Data map[string]string `json:"data"`
		Type string            `json:"type"`
	}
	var latestVersion int

	for i := range secretList.Items {
		s := &secretList.Items[i]
		if s.Type != "helm.sh/release.v1" {
			continue
		}
		versionStr := s.Metadata.Labels["version"]
		var version int
		fmt.Sscanf(versionStr, "%d", &version)
		if version > latestVersion {
			latestVersion = version
			latestSecret = s
		}
	}

	if latestSecret == nil {
		return "", fmt.Errorf("no valid Helm release found for %s/%s", namespace, releaseName)
	}

	// Decode the release data (base64 -> gzip -> JSON)
	releaseData, ok := latestSecret.Data["release"]
	if !ok {
		return "", fmt.Errorf("no release data in Helm secret")
	}

	decoded, err := base64.StdEncoding.DecodeString(releaseData)
	if err != nil {
		return "", fmt.Errorf("failed to decode release data: %w", err)
	}

	// Decompress gzip
	gzReader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		return "", fmt.Errorf("failed to decompress release data: %w", err)
	}

	// Parse the release JSON
	var release struct {
		Name      string                 `json:"name"`
		Namespace string                 `json:"namespace"`
		Version   int                    `json:"version"`
		Config    map[string]interface{} `json:"config"`
		Chart     struct {
			Metadata struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"metadata"`
		} `json:"chart"`
	}

	if err := json.Unmarshal(decompressed, &release); err != nil {
		return "", fmt.Errorf("failed to parse release data: %w", err)
	}

	// Build header comments
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Migrated from: Helm Release %s/%s (version %d)\n",
		namespace, releaseName, release.Version))
	if release.Chart.Metadata.Name != "" {
		sb.WriteString(fmt.Sprintf("# Chart: %s @ %s\n",
			release.Chart.Metadata.Name, release.Chart.Metadata.Version))
	}
	sb.WriteString("---\n")

	// Include config values
	if len(release.Config) > 0 {
		yamlBytes, err := yaml.Marshal(release.Config)
		if err != nil {
			return "", fmt.Errorf("failed to marshal config: %w", err)
		}
		sb.Write(yamlBytes)
	} else {
		sb.WriteString("# No custom values - using chart defaults\n")
	}

	return sb.String(), nil
}

