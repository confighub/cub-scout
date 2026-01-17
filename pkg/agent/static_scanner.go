// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// StaticScanResult holds results from static file analysis
type StaticScanResult struct {
	File          string          `json:"file"`
	ScannedAt     time.Time       `json:"scannedAt"`
	ResourceCount int             `json:"resourceCount"`
	Findings      []StaticFinding `json:"findings"`
	Error         string          `json:"error,omitempty"`
}

// StaticFinding represents a single finding from static analysis
type StaticFinding struct {
	CCVEID       string `json:"ccve_id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	ResourceName string `json:"resource_name"`
	Namespace    string `json:"namespace,omitempty"`
	Severity     string `json:"severity"`
	Category     string `json:"category"`
	Message      string `json:"message"`
	Remediation  string `json:"remediation,omitempty"`
}

// StaticScanner performs static analysis on YAML files
type StaticScanner struct {
	ccveDBDir string
	patterns  []StaticPattern
}

// StaticPattern defines a detection pattern for static analysis
type StaticPattern struct {
	CCVEID      string
	Name        string
	Severity    string
	Category    string
	Resources   []string // K8s resource kinds to check
	Condition   func(resource map[string]interface{}) (bool, string)
	Remediation string
}

// NewStaticScanner creates a new static scanner
func NewStaticScanner(ccveDBDir string) (*StaticScanner, error) {
	s := &StaticScanner{
		ccveDBDir: ccveDBDir,
	}
	s.loadBuiltinPatterns()
	return s, nil
}

// ScanFile scans a YAML file for misconfigurations
func (s *StaticScanner) ScanFile(ctx context.Context, filename string) (*StaticScanResult, error) {
	result := &StaticScanResult{
		File:      filename,
		ScannedAt: time.Now(),
		Findings:  []StaticFinding{},
	}

	// Read and parse YAML file
	resources, err := s.parseYAMLFile(filename)
	if err != nil {
		result.Error = fmt.Sprintf("failed to parse YAML: %v", err)
		return result, nil
	}

	result.ResourceCount = len(resources)

	// Check each resource against patterns
	for _, resource := range resources {
		kind, _ := getStringField(resource, "kind")
		metadata, _ := resource["metadata"].(map[string]interface{})
		name, _ := getStringField(metadata, "name")
		namespace, _ := getStringField(metadata, "namespace")

		for _, pattern := range s.patterns {
			// Check if pattern applies to this resource kind
			if !s.patternMatchesKind(pattern, kind) {
				continue
			}

			// Run the pattern condition
			if matched, message := pattern.Condition(resource); matched {
				finding := StaticFinding{
					CCVEID:       pattern.CCVEID,
					Name:         pattern.Name,
					Kind:         kind,
					ResourceName: name,
					Namespace:    namespace,
					Severity:     pattern.Severity,
					Category:     pattern.Category,
					Message:      message,
					Remediation:  pattern.Remediation,
				}
				result.Findings = append(result.Findings, finding)
			}
		}
	}

	return result, nil
}

// parseYAMLFile parses a YAML file with multiple documents
func (s *StaticScanner) parseYAMLFile(filename string) ([]map[string]interface{}, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var resources []map[string]interface{}
	decoder := yaml.NewDecoder(bufio.NewReader(file))

	for {
		var doc map[string]interface{}
		err := decoder.Decode(&doc)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		if doc != nil && len(doc) > 0 {
			resources = append(resources, doc)
		}
	}

	return resources, nil
}

// patternMatchesKind checks if a pattern applies to the resource kind
func (s *StaticScanner) patternMatchesKind(pattern StaticPattern, kind string) bool {
	if len(pattern.Resources) == 0 {
		return true // Match all if no resources specified
	}
	for _, r := range pattern.Resources {
		if strings.EqualFold(r, kind) {
			return true
		}
	}
	return false
}

// loadBuiltinPatterns loads built-in detection patterns
func (s *StaticScanner) loadBuiltinPatterns() {
	s.patterns = []StaticPattern{
		// STATE_MACHINE_STUCK patterns
		{
			CCVEID:    "CCVE-2025-0241",
			Name:      "Orphaned finalizer",
			Severity:  "critical",
			Category:  "STATE",
			Resources: []string{"Pod", "Namespace", "PersistentVolumeClaim", "CustomResourceDefinition"},
			Condition: func(r map[string]interface{}) (bool, string) {
				metadata, ok := r["metadata"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				finalizers, ok := metadata["finalizers"].([]interface{})
				if !ok || len(finalizers) == 0 {
					return false, ""
				}
				// Check for suspicious finalizers (non-standard controllers)
				for _, f := range finalizers {
					fin, ok := f.(string)
					if !ok {
						continue
					}
					// Suspicious patterns: nonexistent domains, orphaned controllers
					suspicious := []string{"nonexistent", "orphan", "cleanup.", "test."}
					for _, s := range suspicious {
						if strings.Contains(strings.ToLower(fin), s) {
							return true, fmt.Sprintf("Finalizer %q may reference non-existent controller", fin)
						}
					}
				}
				return false, ""
			},
			Remediation: "Remove orphaned finalizer or ensure controller is deployed",
		},
		{
			CCVEID:    "CCVE-2025-0242",
			Name:      "HelmRelease missing timeout",
			Severity:  "warning",
			Category:  "STATE",
			Resources: []string{"HelmRelease"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				// Check if timeout is missing
				if _, hasTimeout := spec["timeout"]; !hasTimeout {
					return true, "HelmRelease has no timeout configured - can hang indefinitely"
				}
				return false, ""
			},
			Remediation: "Add spec.timeout (e.g., '10m') to prevent indefinite hangs",
		},
		{
			CCVEID:    "CCVE-2025-0243",
			Name:      "Kustomization missing timeout",
			Severity:  "warning",
			Category:  "STATE",
			Resources: []string{"Kustomization"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				if _, hasTimeout := spec["timeout"]; !hasTimeout {
					return true, "Kustomization has no timeout configured - can hang indefinitely"
				}
				return false, ""
			},
			Remediation: "Add spec.timeout (e.g., '5m') to prevent indefinite hangs",
		},

		// SILENT_FAILURE patterns
		{
			CCVEID:    "CCVE-2025-0244",
			Name:      "Probe timeout exceeds period",
			Severity:  "warning",
			Category:  "SILENT",
			Resources: []string{"Deployment", "StatefulSet", "DaemonSet", "Pod"},
			Condition: func(r map[string]interface{}) (bool, string) {
				containers := getContainers(r)
				for _, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					for _, probeType := range []string{"livenessProbe", "readinessProbe", "startupProbe"} {
						probe, ok := container[probeType].(map[string]interface{})
						if !ok {
							continue
						}
						timeout := getIntField(probe, "timeoutSeconds", 1)
						period := getIntField(probe, "periodSeconds", 10)
						if timeout > period {
							return true, fmt.Sprintf("%s timeout (%ds) > period (%ds) - probe may never succeed", probeType, timeout, period)
						}
					}
				}
				return false, ""
			},
			Remediation: "Ensure probe timeoutSeconds <= periodSeconds",
		},
		{
			CCVEID:    "CCVE-2025-0245",
			Name:      "revisionHistoryLimit is zero",
			Severity:  "warning",
			Category:  "CONFIG",
			Resources: []string{"Deployment", "StatefulSet", "DaemonSet"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				if limit, ok := spec["revisionHistoryLimit"]; ok {
					if intVal, ok := limit.(int); ok && intVal == 0 {
						return true, "revisionHistoryLimit: 0 prevents rollback capability"
					}
				}
				return false, ""
			},
			Remediation: "Set revisionHistoryLimit >= 2 to allow rollback",
		},

		// TIMING_BOMB patterns
		{
			CCVEID:    "CCVE-2025-0246",
			Name:      "PDB with minAvailable 100%",
			Severity:  "critical",
			Category:  "TIMING",
			Resources: []string{"PodDisruptionBudget"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				if minAvail, ok := spec["minAvailable"]; ok {
					if str, ok := minAvail.(string); ok && str == "100%" {
						return true, "minAvailable: 100% blocks all voluntary disruptions"
					}
				}
				return false, ""
			},
			Remediation: "Use minAvailable < 100% or maxUnavailable > 0",
		},
		{
			CCVEID:    "CCVE-2025-0247",
			Name:      "HPA with min equals max",
			Severity:  "info",
			Category:  "CONFIG",
			Resources: []string{"HorizontalPodAutoscaler"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				minReplicas := getIntField(spec, "minReplicas", 1)
				maxReplicas := getIntField(spec, "maxReplicas", 0)
				if maxReplicas > 0 && minReplicas == maxReplicas {
					return true, fmt.Sprintf("HPA min (%d) == max (%d) - autoscaling disabled", minReplicas, maxReplicas)
				}
				return false, ""
			},
			Remediation: "Set maxReplicas > minReplicas for effective autoscaling",
		},

		// REFERENCE_NOT_FOUND patterns
		{
			CCVEID:    "CCVE-2025-0248",
			Name:      "Missing resource limits",
			Severity:  "warning",
			Category:  "CONFIG",
			Resources: []string{"Deployment", "StatefulSet", "DaemonSet", "Pod"},
			Condition: func(r map[string]interface{}) (bool, string) {
				containers := getContainers(r)
				for _, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					resources, ok := container["resources"].(map[string]interface{})
					if !ok {
						name, _ := getStringField(container, "name")
						return true, fmt.Sprintf("Container %q has no resource limits defined", name)
					}
					if _, hasLimits := resources["limits"]; !hasLimits {
						name, _ := getStringField(container, "name")
						return true, fmt.Sprintf("Container %q has no resource limits defined", name)
					}
				}
				return false, ""
			},
			Remediation: "Add resources.limits.cpu and resources.limits.memory",
		},

		// CROSS_REFERENCE_MISMATCH patterns
		{
			CCVEID:    "CCVE-2025-0249",
			Name:      "NetworkPolicy with empty podSelector blocks all",
			Severity:  "warning",
			Category:  "CONFIG",
			Resources: []string{"NetworkPolicy"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				podSelector, ok := spec["podSelector"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				// Empty podSelector with ingress rules but no egress = blocks egress
				if len(podSelector) == 0 {
					policyTypes, _ := spec["policyTypes"].([]interface{})
					hasEgress := false
					for _, pt := range policyTypes {
						if str, ok := pt.(string); ok && str == "Egress" {
							hasEgress = true
						}
					}
					if hasEgress {
						egress, _ := spec["egress"].([]interface{})
						if len(egress) == 0 {
							return true, "NetworkPolicy with Egress type but no egress rules blocks all egress including DNS"
						}
					}
				}
				return false, ""
			},
			Remediation: "Add egress rules for required traffic (especially DNS on port 53)",
		},

		// TIMING_BOMB: Certificate with very short duration
		{
			CCVEID:    "CCVE-2025-0035",
			Name:      "Certificate duration too short",
			Severity:  "warning",
			Category:  "TIMING",
			Resources: []string{"Certificate"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				duration, ok := spec["duration"].(string)
				if !ok {
					return false, ""
				}
				// Check for very short durations (< 24h)
				shortDurations := []string{"1h", "2h", "3h", "4h", "6h", "8h", "12h"}
				for _, short := range shortDurations {
					if duration == short {
						return true, fmt.Sprintf("Certificate duration %s is very short - renewal may fail if rate-limited", duration)
					}
				}
				return false, ""
			},
			Remediation: "Use duration >= 24h to allow sufficient renewal time",
		},

		// UPGRADE_LANDMINE: CRD with deprecated stored version
		{
			CCVEID:    "CCVE-2025-0184",
			Name:      "CRD has deprecated stored version",
			Severity:  "critical",
			Category:  "UPGRADE",
			Resources: []string{"CustomResourceDefinition"},
			Condition: func(r map[string]interface{}) (bool, string) {
				// Check spec.versions for deprecated: true with storage: true
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				versions, ok := spec["versions"].([]interface{})
				if !ok {
					return false, ""
				}
				// Build map of deprecated versions
				deprecatedVersions := make(map[string]bool)
				for _, v := range versions {
					ver, ok := v.(map[string]interface{})
					if !ok {
						continue
					}
					name, _ := getStringField(ver, "name")
					if deprecated, ok := ver["deprecated"].(bool); ok && deprecated {
						deprecatedVersions[name] = true
					}
				}

				// Check status.storedVersions for deprecated ones
				status, ok := r["status"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				storedVersions, ok := status["storedVersions"].([]interface{})
				if !ok {
					return false, ""
				}
				for _, sv := range storedVersions {
					svStr, ok := sv.(string)
					if !ok {
						continue
					}
					if deprecatedVersions[svStr] {
						return true, fmt.Sprintf("CRD has deprecated version %q in storedVersions - migration required", svStr)
					}
				}
				return false, ""
			},
			Remediation: "Run storage version migrator to convert all CRs to current version",
		},

		// RESOURCE_EXHAUSTION: JVM container with low memory
		{
			CCVEID:    "CCVE-2025-0254",
			Name:      "JVM container with insufficient memory",
			Severity:  "warning",
			Category:  "RESOURCE",
			Resources: []string{"Deployment", "StatefulSet", "DaemonSet", "Pod"},
			Condition: func(r map[string]interface{}) (bool, string) {
				containers := getContainers(r)
				for _, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					// Check if image looks like JVM
					image, _ := getStringField(container, "image")
					isJVM := strings.Contains(strings.ToLower(image), "jdk") ||
						strings.Contains(strings.ToLower(image), "jre") ||
						strings.Contains(strings.ToLower(image), "openjdk") ||
						strings.Contains(strings.ToLower(image), "java") ||
						strings.Contains(strings.ToLower(image), "maven") ||
						strings.Contains(strings.ToLower(image), "gradle") ||
						strings.Contains(strings.ToLower(image), "spring")

					if !isJVM {
						continue
					}

					// Check memory limits
					resources, ok := container["resources"].(map[string]interface{})
					if !ok {
						continue
					}
					limits, ok := resources["limits"].(map[string]interface{})
					if !ok {
						continue
					}
					memory, ok := limits["memory"].(string)
					if !ok {
						continue
					}
					// Check for low memory (< 256Mi)
					lowMemory := []string{"32Mi", "64Mi", "128Mi", "100Mi", "150Mi", "200Mi"}
					for _, low := range lowMemory {
						if memory == low {
							name, _ := getStringField(container, "name")
							return true, fmt.Sprintf("JVM container %q has only %s memory - JVM typically needs >= 256Mi", name, memory)
						}
					}
				}
				return false, ""
			},
			Remediation: "Increase memory limit to at least 256Mi for JVM applications",
		},

		// CROSS_REFERENCE: NetworkPolicy blocks cluster DNS
		{
			CCVEID:    "CCVE-2025-0274",
			Name:      "NetworkPolicy may block cluster DNS",
			Severity:  "warning",
			Category:  "NETWORK",
			Resources: []string{"NetworkPolicy"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}

				// Check if Egress policy type is set
				policyTypes, _ := spec["policyTypes"].([]interface{})
				hasEgress := false
				for _, pt := range policyTypes {
					if str, ok := pt.(string); ok && str == "Egress" {
						hasEgress = true
					}
				}
				if !hasEgress {
					return false, ""
				}

				// Check egress rules
				egress, ok := spec["egress"].([]interface{})
				if !ok || len(egress) == 0 {
					return false, ""
				}

				// Look for DNS rules that only allow external IPs
				for _, e := range egress {
					rule, ok := e.(map[string]interface{})
					if !ok {
						continue
					}
					ports, _ := rule["ports"].([]interface{})
					hasDNSPort := false
					for _, p := range ports {
						port, ok := p.(map[string]interface{})
						if !ok {
							continue
						}
						portNum := getIntField(port, "port", 0)
						if portNum == 53 {
							hasDNSPort = true
							break
						}
					}
					if !hasDNSPort {
						continue
					}

					// Check if only external IPs are allowed
					to, _ := rule["to"].([]interface{})
					hasOnlyIPBlock := true
					hasClusterAllow := false
					for _, t := range to {
						target, ok := t.(map[string]interface{})
						if !ok {
							continue
						}
						if _, hasIPBlock := target["ipBlock"]; !hasIPBlock {
							hasOnlyIPBlock = false
						}
						// Check for namespace selector (allows cluster DNS)
						if _, hasNS := target["namespaceSelector"]; hasNS {
							hasClusterAllow = true
						}
						if _, hasPod := target["podSelector"]; hasPod {
							hasClusterAllow = true
						}
					}

					if hasDNSPort && hasOnlyIPBlock && !hasClusterAllow {
						return true, "NetworkPolicy allows DNS (port 53) only to external IPs - cluster DNS (CoreDNS) may be blocked"
					}
				}
				return false, ""
			},
			Remediation: "Add egress rule allowing DNS to kube-system namespace for cluster DNS resolution",
		},

		// REFERENCE_NOT_FOUND: Ingress TLS secret (static warning)
		{
			CCVEID:    "CCVE-2025-0063",
			Name:      "Ingress TLS secretName reference",
			Severity:  "info",
			Category:  "DEPEND",
			Resources: []string{"Ingress"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				tls, ok := spec["tls"].([]interface{})
				if !ok || len(tls) == 0 {
					return false, ""
				}
				metadata, _ := r["metadata"].(map[string]interface{})
				namespace, _ := getStringField(metadata, "namespace")
				if namespace == "" {
					namespace = "default"
				}

				for _, t := range tls {
					tlsEntry, ok := t.(map[string]interface{})
					if !ok {
						continue
					}
					secretName, ok := getStringField(tlsEntry, "secretName")
					if !ok || secretName == "" {
						continue
					}
					// Flag as info - secret must exist in same namespace
					return true, fmt.Sprintf("Ingress TLS references secret %q - must exist in namespace %q", secretName, namespace)
				}
				return false, ""
			},
			Remediation: "Ensure TLS secret exists in the same namespace as the Ingress",
		},

		// TIMING_BOMB: ExternalSecret with long refresh interval
		{
			CCVEID:    "CCVE-2025-0095",
			Name:      "ExternalSecret refresh interval too long",
			Severity:  "warning",
			Category:  "TIMING",
			Resources: []string{"ExternalSecret"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				interval, ok := spec["refreshInterval"].(string)
				if !ok {
					return false, ""
				}
				// Check for long intervals (>= 12h)
				longIntervals := []string{"12h", "24h", "48h", "1d", "7d"}
				for _, long := range longIntervals {
					if interval == long {
						return true, fmt.Sprintf("ExternalSecret refreshInterval %s may cause stale secrets in production", interval)
					}
				}
				return false, ""
			},
			Remediation: "Use refreshInterval <= 1h for production secrets",
		},

		// RESOURCE_EXHAUSTION: Over-provisioned CPU request
		{
			CCVEID:    "CCVE-2025-0280",
			Name:      "Over-provisioned CPU request",
			Severity:  "warning",
			Category:  "CONFIG",
			Resources: []string{"Deployment", "StatefulSet", "DaemonSet", "Pod", "Job", "CronJob"},
			Condition: func(r map[string]interface{}) (bool, string) {
				containers := getContainers(r)
				for _, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					name, _ := getStringField(container, "name")
					resources, ok := container["resources"].(map[string]interface{})
					if !ok {
						continue
					}
					requests, ok := resources["requests"].(map[string]interface{})
					if !ok {
						continue
					}
					cpu, ok := requests["cpu"].(string)
					if !ok {
						continue
					}
					// Check for excessive CPU (>= 4 cores)
					excessiveCPU := []string{"4", "4000m", "5", "5000m", "6", "6000m", "7", "7000m", "8", "8000m", "10", "10000m", "16", "16000m", "32", "32000m"}
					for _, excessive := range excessiveCPU {
						if cpu == excessive {
							return true, fmt.Sprintf("Container %q requests %s CPU - most containers need < 2 cores; consider right-sizing", name, cpu)
						}
					}
				}
				return false, ""
			},
			Remediation: "Review CPU request - most applications need < 2 cores. Use HPA for burst capacity instead of over-provisioning.",
		},

		// RESOURCE_EXHAUSTION: Over-provisioned memory request
		{
			CCVEID:    "CCVE-2025-0281",
			Name:      "Over-provisioned memory request",
			Severity:  "warning",
			Category:  "CONFIG",
			Resources: []string{"Deployment", "StatefulSet", "DaemonSet", "Pod", "Job", "CronJob"},
			Condition: func(r map[string]interface{}) (bool, string) {
				containers := getContainers(r)
				for _, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					name, _ := getStringField(container, "name")
					resources, ok := container["resources"].(map[string]interface{})
					if !ok {
						continue
					}
					requests, ok := resources["requests"].(map[string]interface{})
					if !ok {
						continue
					}
					memory, ok := requests["memory"].(string)
					if !ok {
						continue
					}
					// Check for excessive memory (>= 8Gi)
					excessiveMemory := []string{"8Gi", "8192Mi", "10Gi", "12Gi", "16Gi", "32Gi", "64Gi", "128Gi"}
					for _, excessive := range excessiveMemory {
						if memory == excessive {
							return true, fmt.Sprintf("Container %q requests %s memory - review if this is needed; may waste cluster resources", name, memory)
						}
					}
				}
				return false, ""
			},
			Remediation: "Review memory request - ensure workload actually needs this much. Over-provisioning wastes cluster resources and increases costs.",
		},

		// RESOURCE_EXHAUSTION: Large limit-to-request ratio (burstable with high contention risk)
		{
			CCVEID:    "CCVE-2025-0282",
			Name:      "Large limit-to-request ratio",
			Severity:  "info",
			Category:  "CONFIG",
			Resources: []string{"Deployment", "StatefulSet", "DaemonSet", "Pod"},
			Condition: func(r map[string]interface{}) (bool, string) {
				containers := getContainers(r)
				for _, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					name, _ := getStringField(container, "name")
					resources, ok := container["resources"].(map[string]interface{})
					if !ok {
						continue
					}
					requests, _ := resources["requests"].(map[string]interface{})
					limits, _ := resources["limits"].(map[string]interface{})
					if requests == nil || limits == nil {
						continue
					}

					// Check memory ratio: small request, large limit
					reqMem, hasReqMem := requests["memory"].(string)
					limMem, hasLimMem := limits["memory"].(string)
					if hasReqMem && hasLimMem {
						// Detect patterns like 128Mi request with 8Gi limit
						smallRequests := []string{"64Mi", "128Mi", "256Mi", "512Mi"}
						largeLimit := []string{"4Gi", "8Gi", "16Gi", "32Gi"}
						for _, small := range smallRequests {
							if reqMem == small {
								for _, large := range largeLimit {
									if limMem == large {
										return true, fmt.Sprintf("Container %q has %s request but %s limit - large ratio may cause OOM under contention", name, reqMem, limMem)
									}
								}
							}
						}
					}
				}
				return false, ""
			},
			Remediation: "Set requests closer to limits for consistent scheduling. Large ratios cause burstable QoS and potential OOM under node pressure.",
		},

		// Grafana sidecar whitespace (famous CCVE-2025-0027)
		{
			CCVEID:    "CCVE-2025-0027",
			Name:      "Grafana sidecar namespace whitespace",
			Severity:  "critical",
			Category:  "CONFIG",
			Resources: []string{"Deployment"},
			Condition: func(r map[string]interface{}) (bool, string) {
				metadata, _ := r["metadata"].(map[string]interface{})
				name, _ := getStringField(metadata, "name")
				if !strings.Contains(strings.ToLower(name), "grafana") {
					return false, ""
				}
				containers := getContainers(r)
				for _, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					cname, _ := getStringField(container, "name")
					if !strings.Contains(strings.ToLower(cname), "sidecar") {
						continue
					}
					envs, ok := container["env"].([]interface{})
					if !ok {
						continue
					}
					for _, e := range envs {
						env, ok := e.(map[string]interface{})
						if !ok {
							continue
						}
						envName, _ := getStringField(env, "name")
						if envName == "NAMESPACE" {
							value, _ := getStringField(env, "value")
							// Check for spaces after commas
							if matched, _ := regexp.MatchString(`,\s+`, value); matched {
								return true, "NAMESPACE env var contains whitespace after comma - breaks namespace filtering"
							}
						}
					}
				}
				return false, ""
			},
			Remediation: "Remove spaces from comma-separated namespace list",
		},

		// STATE_MACHINE_STUCK: Deployment rollout impossible (xBOW 026)
		{
			CCVEID:    "CCVE-2025-3725",
			Name:      "Deployment rollout impossible",
			Severity:  "critical",
			Category:  "STATE",
			Resources: []string{"Deployment"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				strategy, ok := spec["strategy"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				strategyType, _ := getStringField(strategy, "type")
				if strategyType != "RollingUpdate" {
					return false, ""
				}
				rollingUpdate, ok := strategy["rollingUpdate"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				maxUnavailable := getIntOrStringField(rollingUpdate, "maxUnavailable")
				maxSurge := getIntOrStringField(rollingUpdate, "maxSurge")
				// Both zero means no pods can be taken offline or added
				if maxUnavailable == "0" && maxSurge == "0" {
					return true, "maxUnavailable=0 AND maxSurge=0 makes rollout impossible - no pods can be replaced"
				}
				return false, ""
			},
			Remediation: "Set maxUnavailable > 0 or maxSurge > 0 to allow rolling updates",
		},

		// STATE_MACHINE_STUCK: CronJob Forbid without timeout (xBOW 027)
		{
			CCVEID:    "CCVE-2025-3726",
			Name:      "CronJob Forbid without activeDeadlineSeconds",
			Severity:  "warning",
			Category:  "STATE",
			Resources: []string{"CronJob"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				policy, _ := getStringField(spec, "concurrencyPolicy")
				if policy != "Forbid" {
					return false, ""
				}
				// Check jobTemplate.spec.activeDeadlineSeconds
				jobTemplate, ok := spec["jobTemplate"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				jobSpec, ok := jobTemplate["spec"].(map[string]interface{})
				if !ok {
					return true, "CronJob has concurrencyPolicy: Forbid but no activeDeadlineSeconds - stuck jobs block all future runs"
				}
				if _, hasDeadline := jobSpec["activeDeadlineSeconds"]; !hasDeadline {
					return true, "CronJob has concurrencyPolicy: Forbid but no activeDeadlineSeconds - stuck jobs block all future runs"
				}
				return false, ""
			},
			Remediation: "Add spec.jobTemplate.spec.activeDeadlineSeconds to ensure jobs timeout",
		},

		// CROSS_REFERENCE_MISMATCH: HPA targets non-existent deployment (xBOW 037)
		{
			CCVEID:    "CCVE-2025-3727",
			Name:      "HPA scaleTargetRef validation",
			Severity:  "info",
			Category:  "DEPEND",
			Resources: []string{"HorizontalPodAutoscaler"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				scaleTargetRef, ok := spec["scaleTargetRef"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				name, _ := getStringField(scaleTargetRef, "name")
				kind, _ := getStringField(scaleTargetRef, "kind")
				// Flag as info - target must exist (static check can't verify)
				if name != "" && kind != "" {
					return true, fmt.Sprintf("HPA targets %s/%s - verify this resource exists (typos cause silent failure)", kind, name)
				}
				return false, ""
			},
			Remediation: "Verify that the scaleTargetRef name matches exactly (case-sensitive)",
		},

		// CROSS_REFERENCE_MISMATCH: PDB selector may not match pods (xBOW 038)
		{
			CCVEID:    "CCVE-2025-3728",
			Name:      "PDB selector validation",
			Severity:  "info",
			Category:  "DEPEND",
			Resources: []string{"PodDisruptionBudget"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				selector, ok := spec["selector"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				matchLabels, ok := selector["matchLabels"].(map[string]interface{})
				if !ok || len(matchLabels) == 0 {
					return false, ""
				}
				// Flag for review - selector must match pods
				labels := []string{}
				for k, v := range matchLabels {
					if s, ok := v.(string); ok {
						labels = append(labels, fmt.Sprintf("%s=%s", k, s))
					}
				}
				if len(labels) > 0 {
					return true, fmt.Sprintf("PDB selects pods with %v - verify pods have these labels", labels)
				}
				return false, ""
			},
			Remediation: "Ensure PDB selector.matchLabels matches your Deployment/StatefulSet pod labels",
		},

		// REFERENCE_NOT_FOUND: Certificate references Issuer vs ClusterIssuer (xBOW 041)
		{
			CCVEID:    "CCVE-2025-3729",
			Name:      "Certificate issuerRef kind validation",
			Severity:  "info",
			Category:  "DEPEND",
			Resources: []string{"Certificate"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				issuerRef, ok := spec["issuerRef"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				kind, _ := getStringField(issuerRef, "kind")
				name, _ := getStringField(issuerRef, "name")
				if kind == "Issuer" {
					metadata, _ := r["metadata"].(map[string]interface{})
					namespace, _ := getStringField(metadata, "namespace")
					if namespace == "" {
						namespace = "default"
					}
					return true, fmt.Sprintf("Certificate references Issuer %q - must exist in namespace %q (use ClusterIssuer for cluster-wide)", name, namespace)
				} else if kind == "ClusterIssuer" {
					return true, fmt.Sprintf("Certificate references ClusterIssuer %q - verify it exists (ClusterIssuers are cluster-scoped)", name)
				}
				return false, ""
			},
			Remediation: "Verify issuerRef.kind (Issuer=namespaced, ClusterIssuer=cluster-scoped) and name are correct",
		},

		// REFERENCE_NOT_FOUND: ExternalSecret references SecretStore (xBOW 045)
		{
			CCVEID:    "CCVE-2025-3730",
			Name:      "ExternalSecret secretStoreRef validation",
			Severity:  "info",
			Category:  "DEPEND",
			Resources: []string{"ExternalSecret"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				storeRef, ok := spec["secretStoreRef"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				name, _ := getStringField(storeRef, "name")
				kind, _ := getStringField(storeRef, "kind")
				if kind == "" {
					kind = "SecretStore"
				}
				if name != "" {
					metadata, _ := r["metadata"].(map[string]interface{})
					namespace, _ := getStringField(metadata, "namespace")
					if namespace == "" {
						namespace = "default"
					}
					if kind == "ClusterSecretStore" {
						return true, fmt.Sprintf("ExternalSecret references ClusterSecretStore %q - verify it exists", name)
					}
					return true, fmt.Sprintf("ExternalSecret references SecretStore %q - must exist in namespace %q", name, namespace)
				}
				return false, ""
			},
			Remediation: "Verify secretStoreRef.name and kind match an existing SecretStore/ClusterSecretStore",
		},

		// REFERENCE_NOT_FOUND: Traefik IngressRoute cross-namespace (xBOW 042)
		{
			CCVEID:    "CCVE-2025-3731",
			Name:      "Traefik IngressRoute cross-namespace service reference",
			Severity:  "warning",
			Category:  "DEPEND",
			Resources: []string{"IngressRoute"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				routes, ok := spec["routes"].([]interface{})
				if !ok {
					return false, ""
				}
				metadata, _ := r["metadata"].(map[string]interface{})
				ingressNS, _ := getStringField(metadata, "namespace")
				if ingressNS == "" {
					ingressNS = "default"
				}

				for _, route := range routes {
					r, ok := route.(map[string]interface{})
					if !ok {
						continue
					}
					services, ok := r["services"].([]interface{})
					if !ok {
						continue
					}
					for _, svc := range services {
						s, ok := svc.(map[string]interface{})
						if !ok {
							continue
						}
						svcNS, _ := getStringField(s, "namespace")
						svcName, _ := getStringField(s, "name")
						if svcNS != "" && svcNS != ingressNS {
							return true, fmt.Sprintf("IngressRoute references service %s/%s - cross-namespace requires providers.kubernetesingress.allowCrossNamespace=true", svcNS, svcName)
						}
					}
				}
				return false, ""
			},
			Remediation: "Either move Service to same namespace or enable Traefik's allowCrossNamespace flag",
		},

		// CROSS_TOOL_INTERACTION: HPA and KEDA ScaledObject conflict (xBOW 066)
		{
			CCVEID:    "CCVE-2025-3732",
			Name:      "HPA may conflict with KEDA ScaledObject",
			Severity:  "info",
			Category:  "CONFIG",
			Resources: []string{"HorizontalPodAutoscaler"},
			Condition: func(r map[string]interface{}) (bool, string) {
				// Flag all HPAs as potential KEDA conflicts if they don't have keda annotations
				metadata, _ := r["metadata"].(map[string]interface{})
				annotations, _ := metadata["annotations"].(map[string]interface{})
				// Check if managed by KEDA
				if annotations != nil {
					if _, hasKeda := annotations["scaledobject.keda.sh/name"]; hasKeda {
						return false, "" // KEDA-managed, OK
					}
				}
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				scaleTargetRef, ok := spec["scaleTargetRef"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				name, _ := getStringField(scaleTargetRef, "name")
				kind, _ := getStringField(scaleTargetRef, "kind")
				// Info: if KEDA is used, there may be a conflict
				return true, fmt.Sprintf("HPA targets %s/%s - if KEDA ScaledObject also targets this, they will conflict", kind, name)
			},
			Remediation: "Use either HPA or KEDA ScaledObject, not both. KEDA creates its own HPA.",
		},

		// CROSS_TOOL_INTERACTION: Istio + Linkerd double injection (xBOW 039/065)
		{
			CCVEID:    "CCVE-2025-3733",
			Name:      "Multiple service mesh injection",
			Severity:  "warning",
			Category:  "CONFIG",
			Resources: []string{"Namespace", "Deployment", "Pod"},
			Condition: func(r map[string]interface{}) (bool, string) {
				metadata, ok := r["metadata"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				labels, _ := metadata["labels"].(map[string]interface{})
				annotations, _ := metadata["annotations"].(map[string]interface{})

				hasIstio := false
				hasLinkerd := false

				// Check labels
				if labels != nil {
					if v, ok := labels["istio-injection"]; ok {
						if s, ok := v.(string); ok && s == "enabled" {
							hasIstio = true
						}
					}
					if v, ok := labels["linkerd.io/inject"]; ok {
						if s, ok := v.(string); ok && s == "enabled" {
							hasLinkerd = true
						}
					}
				}
				// Check annotations
				if annotations != nil {
					if v, ok := annotations["sidecar.istio.io/inject"]; ok {
						if s, ok := v.(string); ok && s == "true" {
							hasIstio = true
						}
					}
					if v, ok := annotations["linkerd.io/inject"]; ok {
						if s, ok := v.(string); ok && s == "enabled" {
							hasLinkerd = true
						}
					}
				}

				if hasIstio && hasLinkerd {
					return true, "Both Istio and Linkerd injection enabled - only one service mesh should be used"
				}
				return false, ""
			},
			Remediation: "Choose one service mesh - disable injection for the unused one",
		},

		// TIMING_BOMB: ResourceQuota approaching limit vs HPA maxReplicas (xBOW 072)
		{
			CCVEID:    "CCVE-2025-3734",
			Name:      "HPA maxReplicas exceeds typical pod counts",
			Severity:  "info",
			Category:  "TIMING",
			Resources: []string{"HorizontalPodAutoscaler"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				maxReplicas := getIntField(spec, "maxReplicas", 0)
				// Flag if maxReplicas is very high (may hit quota)
				if maxReplicas >= 50 {
					return true, fmt.Sprintf("HPA maxReplicas=%d is high - verify ResourceQuota allows this many pods", maxReplicas)
				}
				return false, ""
			},
			Remediation: "Verify namespace ResourceQuota.spec.hard.pods >= HPA maxReplicas across all HPAs",
		},

		// TIMING_BOMB: CronJob without timezone (xBOW 076)
		{
			CCVEID:    "CCVE-2025-3735",
			Name:      "CronJob without timezone specification",
			Severity:  "info",
			Category:  "TIMING",
			Resources: []string{"CronJob"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				// Check if timeZone is set (K8s 1.27+)
				if _, hasTimeZone := spec["timeZone"]; !hasTimeZone {
					schedule, _ := getStringField(spec, "schedule")
					// Check for business-hours-ish schedules that might be timezone-sensitive
					if strings.Contains(schedule, "9 ") || strings.Contains(schedule, "17 ") ||
						strings.Contains(schedule, "8 ") || strings.Contains(schedule, "18 ") {
						return true, "CronJob has business-hours schedule but no timeZone - may shift during DST"
					}
				}
				return false, ""
			},
			Remediation: "Add spec.timeZone (e.g., 'America/New_York') for timezone-sensitive schedules",
		},

		// UPGRADE_LANDMINE: Traefik v3 API change (xBOW 082)
		{
			CCVEID:    "CCVE-2025-3736",
			Name:      "Traefik v2 IngressRoute API version",
			Severity:  "warning",
			Category:  "UPGRADE",
			Resources: []string{"IngressRoute", "IngressRouteTCP", "IngressRouteUDP", "Middleware"},
			Condition: func(r map[string]interface{}) (bool, string) {
				apiVersion, _ := getStringField(r, "apiVersion")
				if strings.HasPrefix(apiVersion, "traefik.containo.us/") {
					kind, _ := getStringField(r, "kind")
					return true, fmt.Sprintf("%s uses traefik.containo.us API - will be ignored after Traefik v3 upgrade (use traefik.io/v1alpha1)", kind)
				}
				return false, ""
			},
			Remediation: "Update apiVersion from traefik.containo.us/* to traefik.io/v1alpha1 before upgrading to Traefik v3",
		},

		// COMPOUND_ATTACK: Webhook with failurePolicy Fail targeting own namespace (xBOW 091)
		{
			CCVEID:    "CCVE-2025-3737",
			Name:      "Webhook may cause deadlock",
			Severity:  "warning",
			Category:  "CONFIG",
			Resources: []string{"ValidatingWebhookConfiguration", "MutatingWebhookConfiguration"},
			Condition: func(r map[string]interface{}) (bool, string) {
				webhooks, ok := r["webhooks"].([]interface{})
				if !ok {
					return false, ""
				}
				for _, w := range webhooks {
					webhook, ok := w.(map[string]interface{})
					if !ok {
						continue
					}
					failurePolicy, _ := getStringField(webhook, "failurePolicy")
					if failurePolicy != "Fail" {
						continue
					}
					// Check if webhook service might be in affected namespace
					clientConfig, ok := webhook["clientConfig"].(map[string]interface{})
					if !ok {
						continue
					}
					svc, ok := clientConfig["service"].(map[string]interface{})
					if !ok {
						continue
					}
					svcNS, _ := getStringField(svc, "namespace")
					svcName, _ := getStringField(svc, "name")

					// Check namespaceSelector
					nsSelector, _ := webhook["namespaceSelector"].(map[string]interface{})
					if nsSelector == nil {
						// No selector means all namespaces including webhook's own
						return true, fmt.Sprintf("Webhook with failurePolicy=Fail and no namespaceSelector - if %s/%s is unavailable, cluster operations may deadlock", svcNS, svcName)
					}
				}
				return false, ""
			},
			Remediation: "Add namespaceSelector to exclude the webhook service's namespace, or use failurePolicy: Ignore",
		},

		// REFERENCE_NOT_FOUND: ServiceAccount imagePullSecrets reference (xBOW 048)
		{
			CCVEID:    "CCVE-2025-3738",
			Name:      "ServiceAccount imagePullSecrets validation",
			Severity:  "info",
			Category:  "DEPEND",
			Resources: []string{"ServiceAccount"},
			Condition: func(r map[string]interface{}) (bool, string) {
				secrets, ok := r["imagePullSecrets"].([]interface{})
				if !ok || len(secrets) == 0 {
					return false, ""
				}
				metadata, _ := r["metadata"].(map[string]interface{})
				namespace, _ := getStringField(metadata, "namespace")
				if namespace == "" {
					namespace = "default"
				}
				for _, s := range secrets {
					secret, ok := s.(map[string]interface{})
					if !ok {
						continue
					}
					name, _ := getStringField(secret, "name")
					if name != "" {
						return true, fmt.Sprintf("ServiceAccount references imagePullSecret %q - must exist in namespace %q", name, namespace)
					}
				}
				return false, ""
			},
			Remediation: "Ensure imagePullSecrets exist in the same namespace as the ServiceAccount",
		},

		// REFERENCE_NOT_FOUND: ClusterRoleBinding subject SA validation (xBOW 049)
		{
			CCVEID:    "CCVE-2025-3739",
			Name:      "ClusterRoleBinding ServiceAccount validation",
			Severity:  "info",
			Category:  "DEPEND",
			Resources: []string{"ClusterRoleBinding", "RoleBinding"},
			Condition: func(r map[string]interface{}) (bool, string) {
				subjects, ok := r["subjects"].([]interface{})
				if !ok {
					return false, ""
				}
				for _, s := range subjects {
					subject, ok := s.(map[string]interface{})
					if !ok {
						continue
					}
					kind, _ := getStringField(subject, "kind")
					if kind != "ServiceAccount" {
						continue
					}
					name, _ := getStringField(subject, "name")
					namespace, _ := getStringField(subject, "namespace")
					if namespace == "" {
						namespace = "default"
					}
					if name != "" {
						return true, fmt.Sprintf("RoleBinding references ServiceAccount %s/%s - if SA is deleted, binding becomes orphaned", namespace, name)
					}
				}
				return false, ""
			},
			Remediation: "Verify ServiceAccounts exist; delete orphaned bindings after SA removal",
		},

		// CROSS_REFERENCE_MISMATCH: Istio mTLS mode conflict (xBOW 031)
		{
			CCVEID:    "CCVE-2025-3740",
			Name:      "Istio DestinationRule TLS mode",
			Severity:  "warning",
			Category:  "CONFIG",
			Resources: []string{"DestinationRule"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				trafficPolicy, ok := spec["trafficPolicy"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				tls, ok := trafficPolicy["tls"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				mode, _ := getStringField(tls, "mode")
				if mode == "DISABLE" {
					host, _ := getStringField(spec, "host")
					return true, fmt.Sprintf("DestinationRule for %q has tls.mode=DISABLE - will fail if PeerAuthentication requires mTLS", host)
				}
				return false, ""
			},
			Remediation: "Match DestinationRule tls.mode with PeerAuthentication mtls.mode (ISTIO_MUTUAL for STRICT)",
		},

		// CROSS_REFERENCE_MISMATCH: Gateway API HTTPRoute parentRef validation (xBOW 036)
		{
			CCVEID:    "CCVE-2025-3741",
			Name:      "HTTPRoute parentRef validation",
			Severity:  "info",
			Category:  "DEPEND",
			Resources: []string{"HTTPRoute", "GRPCRoute", "TLSRoute", "TCPRoute", "UDPRoute"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				parentRefs, ok := spec["parentRefs"].([]interface{})
				if !ok {
					return false, ""
				}
				kind, _ := getStringField(r, "kind")
				for _, pr := range parentRefs {
					ref, ok := pr.(map[string]interface{})
					if !ok {
						continue
					}
					name, _ := getStringField(ref, "name")
					namespace, _ := getStringField(ref, "namespace")
					refKind, _ := getStringField(ref, "kind")
					if refKind == "" {
						refKind = "Gateway"
					}
					// Check for case sensitivity issues - Gateway names are case-sensitive
					if name != "" && name != strings.ToLower(name) {
						return true, fmt.Sprintf("%s parentRef name %q contains uppercase - Gateway names are case-sensitive", kind, name)
					}
					if name != "" {
						msg := fmt.Sprintf("%s references %s/%s", kind, refKind, name)
						if namespace != "" {
							msg += fmt.Sprintf(" in namespace %s", namespace)
						}
						return true, msg + " - verify Gateway exists (names are case-sensitive)"
					}
				}
				return false, ""
			},
			Remediation: "Verify parentRef.name matches Gateway name exactly (case-sensitive)",
		},

		// REFERENCE_NOT_FOUND: Crossplane ProviderConfig credential secret (xBOW 043)
		{
			CCVEID:    "CCVE-2025-3742",
			Name:      "Crossplane ProviderConfig credential reference",
			Severity:  "warning",
			Category:  "DEPEND",
			Resources: []string{"ProviderConfig"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				credentials, ok := spec["credentials"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				source, _ := getStringField(credentials, "source")
				if source != "Secret" {
					return false, ""
				}
				secretRef, ok := credentials["secretRef"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				name, _ := getStringField(secretRef, "name")
				namespace, _ := getStringField(secretRef, "namespace")
				if name != "" {
					return true, fmt.Sprintf("ProviderConfig references credential Secret %s/%s - all managed resources fail if missing", namespace, name)
				}
				return false, ""
			},
			Remediation: "Ensure the credential Secret exists in the specified namespace before creating managed resources",
		},

		// REFERENCE_NOT_FOUND: Tekton Task/TaskRun private registry images (xBOW 050)
		{
			CCVEID:    "CCVE-2025-3743",
			Name:      "Tekton Task uses private registry images",
			Severity:  "info",
			Category:  "DEPEND",
			Resources: []string{"Task", "ClusterTask", "TaskRun", "PipelineRun"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				kind, _ := getStringField(r, "kind")

				// For Task/ClusterTask, check steps
				steps, ok := spec["steps"].([]interface{})
				if ok {
					for _, s := range steps {
						step, ok := s.(map[string]interface{})
						if !ok {
							continue
						}
						image, _ := getStringField(step, "image")
						// Check for private registry patterns (not docker.io, gcr.io, quay.io, etc.)
						if image != "" && !isPublicRegistry(image) {
							return true, fmt.Sprintf("%s step uses private registry image %q - ensure imagePullSecrets configured", kind, image)
						}
					}
				}

				// For TaskRun/PipelineRun, check if imagePullSecrets or serviceAccountName is set
				if kind == "TaskRun" || kind == "PipelineRun" {
					podTemplate, _ := spec["podTemplate"].(map[string]interface{})
					if podTemplate != nil {
						if _, hasSecrets := podTemplate["imagePullSecrets"]; hasSecrets {
							return false, "" // Has imagePullSecrets, OK
						}
					}
					if _, hasSA := spec["serviceAccountName"]; !hasSA {
						return true, fmt.Sprintf("%s has no serviceAccountName or podTemplate.imagePullSecrets - may fail for private images", kind)
					}
				}
				return false, ""
			},
			Remediation: "Add serviceAccountName with imagePullSecrets or spec.podTemplate.imagePullSecrets for private registries",
		},

		// RESOURCE_EXHAUSTION: Large ConfigMap (xBOW 052)
		{
			CCVEID:    "CCVE-2025-3744",
			Name:      "Large ConfigMap approaching size limit",
			Severity:  "warning",
			Category:  "CONFIG",
			Resources: []string{"ConfigMap"},
			Condition: func(r map[string]interface{}) (bool, string) {
				data, _ := r["data"].(map[string]interface{})
				binaryData, _ := r["binaryData"].(map[string]interface{})

				totalSize := 0
				keyCount := 0

				// Estimate size from data keys
				for k, v := range data {
					keyCount++
					totalSize += len(k)
					if s, ok := v.(string); ok {
						totalSize += len(s)
					}
				}
				for k, v := range binaryData {
					keyCount++
					totalSize += len(k)
					if s, ok := v.(string); ok {
						totalSize += len(s) // base64 encoded
					}
				}

				// 1MB limit, warn at 500KB
				if totalSize > 500*1024 {
					metadata, _ := r["metadata"].(map[string]interface{})
					name, _ := getStringField(metadata, "name")
					return true, fmt.Sprintf("ConfigMap %q is ~%dKB (%d keys) - approaching 1MB etcd limit, consider splitting", name, totalSize/1024, keyCount)
				}
				return false, ""
			},
			Remediation: "Split large ConfigMaps into smaller ones, or use external config storage for large data",
		},

		// RESOURCE_EXHAUSTION: FlowSchema deprioritizes kube-system (xBOW 058)
		{
			CCVEID:    "CCVE-2025-3745",
			Name:      "FlowSchema may deprioritize system controllers",
			Severity:  "warning",
			Category:  "CONFIG",
			Resources: []string{"FlowSchema"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				rules, ok := spec["rules"].([]interface{})
				if !ok {
					return false, ""
				}
				priorityLevel, _ := spec["priorityLevelConfiguration"].(map[string]interface{})
				priorityName, _ := getStringField(priorityLevel, "name")

				// Check if any rule matches kube-system service accounts
				for _, rule := range rules {
					r, ok := rule.(map[string]interface{})
					if !ok {
						continue
					}
					subjects, ok := r["subjects"].([]interface{})
					if !ok {
						continue
					}
					for _, subj := range subjects {
						s, ok := subj.(map[string]interface{})
						if !ok {
							continue
						}
						kind, _ := getStringField(s, "kind")
						if kind != "ServiceAccount" {
							continue
						}
						sa, ok := s["serviceAccount"].(map[string]interface{})
						if !ok {
							continue
						}
						ns, _ := getStringField(sa, "namespace")
						if ns == "kube-system" {
							// Check if priority level is low/restrictive
							if strings.Contains(strings.ToLower(priorityName), "low") ||
								strings.Contains(strings.ToLower(priorityName), "restrict") {
								return true, fmt.Sprintf("FlowSchema routes kube-system ServiceAccounts to %q - may throttle critical controllers", priorityName)
							}
						}
					}
				}
				return false, ""
			},
			Remediation: "Exclude kube-system from restrictive FlowSchemas or use exempt priority level for system controllers",
		},

		// UPGRADE_LANDMINE: Istio deprecated API versions (xBOW 085)
		{
			CCVEID:    "CCVE-2025-3746",
			Name:      "Istio deprecated API version",
			Severity:  "warning",
			Category:  "UPGRADE",
			Resources: []string{"VirtualService", "DestinationRule", "Gateway", "ServiceEntry", "Sidecar", "EnvoyFilter"},
			Condition: func(r map[string]interface{}) (bool, string) {
				apiVersion, _ := getStringField(r, "apiVersion")
				kind, _ := getStringField(r, "kind")
				// v1alpha3 is deprecated, should use v1beta1 or v1
				if strings.Contains(apiVersion, "v1alpha3") {
					return true, fmt.Sprintf("%s uses deprecated API %s - migrate to networking.istio.io/v1beta1 or v1 before Istio upgrade", kind, apiVersion)
				}
				// Check for deprecated fields
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				// Check for deprecated h2UpgradePolicy in DestinationRule
				if kind == "DestinationRule" {
					trafficPolicy, ok := spec["trafficPolicy"].(map[string]interface{})
					if ok {
						connPool, ok := trafficPolicy["connectionPool"].(map[string]interface{})
						if ok {
							http, ok := connPool["http"].(map[string]interface{})
							if ok {
								if _, hasH2 := http["h2UpgradePolicy"]; hasH2 {
									return true, fmt.Sprintf("%s uses deprecated h2UpgradePolicy field - removed in Istio 1.18+", kind)
								}
							}
						}
					}
				}
				return false, ""
			},
			Remediation: "Update to networking.istio.io/v1beta1 or v1 and remove deprecated fields before Istio upgrade",
		},

		// CROSS_REFERENCE_MISMATCH: ServiceMonitor selector validation (xBOW 035)
		{
			CCVEID:    "CCVE-2025-3747",
			Name:      "ServiceMonitor selector validation",
			Severity:  "info",
			Category:  "DEPEND",
			Resources: []string{"ServiceMonitor", "PodMonitor"},
			Condition: func(r map[string]interface{}) (bool, string) {
				spec, ok := r["spec"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				kind, _ := getStringField(r, "kind")
				selector, ok := spec["selector"].(map[string]interface{})
				if !ok {
					return false, ""
				}
				matchLabels, ok := selector["matchLabels"].(map[string]interface{})
				if !ok || len(matchLabels) == 0 {
					return false, ""
				}
				// Flag ServiceMonitors with specific matchLabels - must match Service labels exactly
				labels := []string{}
				for k, v := range matchLabels {
					if s, ok := v.(string); ok {
						labels = append(labels, fmt.Sprintf("%s=%s", k, s))
					}
				}
				if len(labels) > 0 {
					return true, fmt.Sprintf("%s selects with %v - verify target Service/Pod has these exact labels", kind, labels)
				}
				return false, ""
			},
			Remediation: "Ensure ServiceMonitor selector.matchLabels matches target Service labels exactly",
		},

		// RESOURCE_EXHAUSTION: ConfigMap with many keys (heuristic for large configs) (xBOW 052)
		{
			CCVEID:    "CCVE-2025-3748",
			Name:      "ConfigMap with many data keys",
			Severity:  "info",
			Category:  "CONFIG",
			Resources: []string{"ConfigMap"},
			Condition: func(r map[string]interface{}) (bool, string) {
				data, _ := r["data"].(map[string]interface{})
				binaryData, _ := r["binaryData"].(map[string]interface{})
				keyCount := len(data) + len(binaryData)
				// Many keys suggests large or complex config (4+ keys is a heuristic)
				if keyCount >= 4 {
					metadata, _ := r["metadata"].(map[string]interface{})
					name, _ := getStringField(metadata, "name")
					return true, fmt.Sprintf("ConfigMap %q has %d keys - consider if this should be split for maintainability", name, keyCount)
				}
				return false, ""
			},
			Remediation: "Consider splitting ConfigMaps with many keys into logical groups",
		},

		// RESOURCE_EXHAUSTION: Loki config with low rate limits (xBOW 054)
		{
			CCVEID:    "CCVE-2025-3749",
			Name:      "Loki ConfigMap may have low ingestion limits",
			Severity:  "info",
			Category:  "CONFIG",
			Resources: []string{"ConfigMap"},
			Condition: func(r map[string]interface{}) (bool, string) {
				metadata, _ := r["metadata"].(map[string]interface{})
				name, _ := getStringField(metadata, "name")
				// Check if this is a Loki config
				if !strings.Contains(strings.ToLower(name), "loki") {
					return false, ""
				}
				data, _ := r["data"].(map[string]interface{})
				for key, value := range data {
					if strings.Contains(key, "loki") || strings.HasSuffix(key, ".yaml") || strings.HasSuffix(key, ".yml") {
						if str, ok := value.(string); ok {
							// Check for ingestion_rate_mb setting
							if strings.Contains(str, "ingestion_rate_mb") ||
								strings.Contains(str, "per_stream_rate_limit") {
								return true, fmt.Sprintf("ConfigMap %q contains Loki config with rate limits - verify limits are sufficient for log volume", name)
							}
						}
					}
				}
				return false, ""
			},
			Remediation: "Review Loki limits_config settings to ensure they handle expected log volume",
		},

		// RESOURCE_EXHAUSTION: Prometheus config without cardinality controls (xBOW 059)
		{
			CCVEID:    "CCVE-2025-3750",
			Name:      "Prometheus config may allow high cardinality",
			Severity:  "info",
			Category:  "CONFIG",
			Resources: []string{"ConfigMap"},
			Condition: func(r map[string]interface{}) (bool, string) {
				metadata, _ := r["metadata"].(map[string]interface{})
				name, _ := getStringField(metadata, "name")
				// Check if this is a Prometheus config
				if !strings.Contains(strings.ToLower(name), "prometheus") {
					return false, ""
				}
				data, _ := r["data"].(map[string]interface{})
				for key, value := range data {
					if strings.Contains(key, "prometheus") || key == "prometheus.yml" || key == "prometheus.yaml" {
						if str, ok := value.(string); ok {
							// Check if scrape config lacks label dropping
							if strings.Contains(str, "scrape_configs") &&
								!strings.Contains(str, "labeldrop") &&
								!strings.Contains(str, "labelkeep") {
								return true, fmt.Sprintf("ConfigMap %q contains Prometheus config without label filtering - may allow high cardinality", name)
							}
						}
					}
				}
				return false, ""
			},
			Remediation: "Add relabel_configs with labeldrop/labelkeep to control metric cardinality",
		},

		// RESOURCE_EXHAUSTION: CoreDNS config with small cache (xBOW 060)
		{
			CCVEID:    "CCVE-2025-3751",
			Name:      "CoreDNS config may have small cache",
			Severity:  "info",
			Category:  "CONFIG",
			Resources: []string{"ConfigMap"},
			Condition: func(r map[string]interface{}) (bool, string) {
				metadata, _ := r["metadata"].(map[string]interface{})
				name, _ := getStringField(metadata, "name")
				// Check if this is CoreDNS config in kube-system
				if name != "coredns" {
					return false, ""
				}
				namespace, _ := getStringField(metadata, "namespace")
				if namespace != "kube-system" && namespace != "" {
					return false, ""
				}
				data, _ := r["data"].(map[string]interface{})
				corefile, ok := data["Corefile"].(string)
				if !ok {
					return false, ""
				}
				// Check for cache plugin with small success/denial values
				if strings.Contains(corefile, "cache") {
					// Look for success/denial settings
					if strings.Contains(corefile, "success") {
						// Try to find small numbers after success
						if strings.Contains(corefile, "success 1000") ||
							strings.Contains(corefile, "success 500") ||
							strings.Contains(corefile, "success 100") {
							return true, "CoreDNS cache success size may be too small for large clusters"
						}
					}
				}
				return false, ""
			},
			Remediation: "Increase CoreDNS cache success/denial sizes for clusters with many services",
		},
	}
}

// isPublicRegistry checks if an image is from a known public registry
func isPublicRegistry(image string) bool {
	publicRegistries := []string{
		"docker.io", "index.docker.io", "gcr.io", "quay.io", "ghcr.io",
		"registry.k8s.io", "k8s.gcr.io", "mcr.microsoft.com", "public.ecr.aws",
		"docker.elastic.co", "registry.hub.docker.com",
	}
	// Images without registry prefix are from Docker Hub
	if !strings.Contains(image, "/") || !strings.Contains(strings.Split(image, "/")[0], ".") {
		return true // docker.io default
	}
	imageLower := strings.ToLower(image)
	for _, reg := range publicRegistries {
		if strings.HasPrefix(imageLower, reg+"/") {
			return true
		}
	}
	return false
}

// getIntOrStringField handles IntOrString fields that can be int or string
func getIntOrStringField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return fmt.Sprintf("%d", val)
		case int64:
			return fmt.Sprintf("%d", val)
		case float64:
			return fmt.Sprintf("%d", int(val))
		case string:
			return val
		}
	}
	return ""
}

// Helper functions

func getStringField(m map[string]interface{}, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

func getIntField(m map[string]interface{}, key string, defaultVal int) int {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return defaultVal
}

func getContainers(r map[string]interface{}) []interface{} {
	// Handle Pod directly
	if spec, ok := r["spec"].(map[string]interface{}); ok {
		if containers, ok := spec["containers"].([]interface{}); ok {
			return containers
		}
		// Handle Deployment/StatefulSet/DaemonSet
		if template, ok := spec["template"].(map[string]interface{}); ok {
			if podSpec, ok := template["spec"].(map[string]interface{}); ok {
				if containers, ok := podSpec["containers"].([]interface{}); ok {
					return containers
				}
			}
		}
	}
	return nil
}

// findCCVEDBDir searches for CCVE database directory
func findCCVEDBDir() string {
	// Try relative to executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "..", "cve", "ccve")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	// Try relative to current directory
	cwd, err := os.Getwd()
	if err == nil {
		dir := filepath.Join(cwd, "cve", "ccve")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	return ""
}
