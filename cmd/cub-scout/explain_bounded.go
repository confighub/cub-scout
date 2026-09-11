// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/confighub/cub-scout/internal/mapsvc"
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	explainBounded    bool
	explainAPIVersion string
	explainContext    string
	explainRefresh    bool
)

// boundedExplainSession is owned by one host/TUI, not shared globally. Reload
// the selected kubeconfig before reuse so changed context/credentials invalidate
// its reader. Only the fingerprint is retained here; it is never serialized.
type boundedExplainSession struct {
	mu          sync.Mutex
	fingerprint [32]byte
	reader      *agent.BoundedResourceReader
}

func boundedExplainRef(args []string, apiVersion, namespace, kubeContext string) (agent.BoundedResourceRef, error) {
	ref := agent.BoundedResourceRef{APIVersion: apiVersion, Namespace: namespace}
	if len(args) == 1 {
		parts := strings.Split(args[0], "/")
		if len(parts) != 2 {
			return ref, fmt.Errorf("bounded explain requires exact Kind/name")
		}
		ref.Kind, ref.Name = parts[0], parts[1]
	} else if len(args) == 2 {
		ref.Kind, ref.Name = args[0], args[1]
	} else {
		return ref, fmt.Errorf("bounded explain requires exact Kind/name")
	}
	if strings.TrimSpace(kubeContext) == "" {
		return ref, fmt.Errorf("bounded explain requires --kube-context")
	}
	return ref, ref.Validate()
}

func (s *boundedExplainSession) observe(ctx context.Context, ref agent.BoundedResourceRef, kubeContext string, refresh bool) (ExplainSummary, error) {
	if err := ref.Validate(); err != nil {
		return ExplainSummary{}, err
	}
	if err := ctx.Err(); err != nil {
		return ExplainSummary{}, err
	}
	reader, err := s.forContext(kubeContext)
	if err != nil {
		return ExplainSummary{}, err
	}
	obj, evidence, err := reader.Read(ctx, ref, refresh)
	return buildBoundedExplainSummary(obj, evidence, err), nil
}

func (s *boundedExplainSession) forContext(kubeContext string) (*agent.BoundedResourceReader, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fail := func(err error) (*agent.BoundedResourceReader, error) {
		s.reader = nil
		return nil, err
	}
	if strings.TrimSpace(kubeContext) == "" {
		return fail(fmt.Errorf("bounded explain requires --kube-context"))
	}
	raw, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return fail(fmt.Errorf("load bounded kube context: %w", err))
	}
	raw.CurrentContext = kubeContext
	if err := clientcmdapi.MinifyConfig(raw); err != nil {
		return fail(fmt.Errorf("select bounded kube context: %w", err))
	}
	if err := clientcmdapi.FlattenConfig(raw); err != nil {
		return fail(fmt.Errorf("load bounded kube credentials: %w", err))
	}
	for _, auth := range raw.AuthInfos {
		if auth.TokenFile != "" {
			data, err := os.ReadFile(auth.TokenFile)
			if err != nil {
				return fail(fmt.Errorf("load bounded token file: %w", err))
			}
			auth.Token, auth.TokenFile = strings.TrimSpace(string(data)), ""
		}
	}
	encoded, err := clientcmd.Write(*raw)
	if err != nil {
		return fail(err)
	}
	fingerprint := sha256.Sum256(encoded)
	if s.reader != nil && s.fingerprint == fingerprint {
		return s.reader, nil
	}
	config, err := clientcmd.NewNonInteractiveClientConfig(*raw, kubeContext, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return fail(err)
	}
	reader, err := agent.NewBoundedResourceReader(config, kubeContext)
	if err != nil {
		return fail(err)
	}
	s.reader, s.fingerprint = reader, fingerprint
	return reader, nil
}

func buildBoundedExplainSummary(obj *unstructured.Unstructured, evidence agent.BoundedReadEvidence, readErr error) ExplainSummary {
	summary := ExplainSummary{
		Resource:  evidence.Resource.Kind + "/" + evidence.Resource.Name,
		Namespace: evidence.Resource.Namespace, Owner: "Unknown", Health: "Unavailable",
		Source: "Not assessed (bounded read)", DeployedVia: "Not assessed (bounded read)",
		Risks: "Not assessed (bounded read)", Drift: "Not assessed (bounded read)",
		ResourceRead: &evidence,
		Omissions: []agent.Omission{
			{Missing: "source-controller-evidence", Reason: "Bounded read does not query controller sources, ConfigHub, or delivery history.", Severity: "info"},
			{Missing: "related-pod-event-evidence", Reason: "Bounded read does not query related pods or events.", Severity: "info"},
			{Missing: "desired-live-comparison", Reason: "No desired configuration or drift/scan assessment was requested.", Severity: "info"},
		},
		Notes: []string{
			"Bounded object evidence only: no source/controller, ConfigHub, event, related-pod, or drift reads.",
			"Object-local readiness is not proof of delivery completion or application success.",
		},
	}
	if readErr != nil {
		summary.Notes = append(summary.Notes, "Evidence unavailable: "+readErr.Error())
		summary.Omissions = append(summary.Omissions, agent.Omission{Missing: "live-resource", Reason: readErr.Error(), Severity: "warning"})
		return summary
	}
	if obj == nil {
		return summary
	}
	owner := agent.DetectOwnership(obj)
	if owner.Type != agent.OwnerUnknown && owner.Type != "" {
		summary.Owner = mapsvc.DisplayOwner(owner.Type)
	}
	if owner.Source != "" {
		summary.Notes = append(summary.Notes, "Ownership evidence: "+owner.Source)
	}
	summary.Health = "Unknown"
	if boundedIsWorkload(obj) {
		st, _ := agent.WorkloadConvergence(obj)
		summary.Health = st.String()
		if decision, ok := agent.BuildRolloutDecisionForWorkload(obj, nil, 0, evidence.ObservedAt); ok {
			summary.CurrentChange = &decision
		}
	} else if boundedHasReadyCondition(obj) {
		st, _ := agent.WorkloadConvergence(obj)
		summary.Health = st.String()
	} else {
		summary.Notes = append(summary.Notes, "No supported object-local readiness evidence; health remains unknown.")
	}
	attr := agent.AttributeFieldMutation(obj, owner)
	summary.MutationCause, summary.MutationManager = attr.Cause, attr.ManagerHint
	return summary
}

func boundedIsWorkload(obj *unstructured.Unstructured) bool {
	switch obj.GetAPIVersion() {
	case "apps/v1":
		return obj.GetKind() == "Deployment" || obj.GetKind() == "StatefulSet" || obj.GetKind() == "DaemonSet"
	case "v1":
		return obj.GetKind() == "Pod"
	case "batch/v1":
		return obj.GetKind() == "Job"
	}
	return false
}

func boundedHasReadyCondition(obj *unstructured.Unstructured) bool {
	conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, item := range conditions {
		condition, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if condition["type"] == "Ready" && (condition["status"] == "True" || condition["status"] == "False" || condition["status"] == "Unknown") {
			return true
		}
	}
	return false
}

func formatBoundedRead(e *agent.BoundedReadEvidence) string {
	observed := "unavailable"
	if !e.ObservedAt.IsZero() {
		observed = e.ObservedAt.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("context=%q api=%s observed=%s cache=%s reads=%d discovery + %d object",
		e.Context, e.Resource.APIVersion, observed, e.Cache, e.Reads.Discovery, e.Reads.Object)
}

func boundedExplainCommand(ref agent.BoundedResourceRef, kubeContext, separator string) string {
	// Kube context names are arbitrary strings, not shell-safe identifiers.
	quotedContext := "'" + strings.ReplaceAll(kubeContext, "'", "'\"'\"'") + "'"
	parts := []string{"cub-scout explain " + ref.Kind + "/" + ref.Name, "--bounded --api-version " + ref.APIVersion, "--kube-context " + quotedContext}
	if ref.Namespace != "" {
		parts = append(parts, "--namespace "+ref.Namespace)
	}
	return strings.Join(parts, separator)
}

func validateBoundedExplainArguments(arguments map[string]interface{}) error {
	for _, key := range []string{"bounded", "refresh"} {
		if value, present := arguments[key]; present {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s must be a boolean", key)
			}
		}
	}
	for _, key := range []string{"api_version", "context", "namespace", "resource"} {
		if value, present := arguments[key]; present {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s must be a string", key)
			}
		}
	}
	return nil
}

// The real stdio gateway reuses one session. Injected test runners and all
// unbounded tools keep the existing subprocess adapter.
func boundedMCPRunner(fallback mcpToolRunner) mcpToolRunner {
	session := &boundedExplainSession{}
	return func(ctx context.Context, args []string) (string, error) {
		if len(args) < 2 || args[0] != "explain" {
			return fallback(ctx, args)
		}
		flags := pflag.NewFlagSet("bounded explain", pflag.ContinueOnError)
		flags.SetOutput(io.Discard)
		bounded, refresh := flags.Bool("bounded", false, ""), flags.Bool("refresh", false, "")
		apiVersion, kubeContext := flags.String("api-version", "", ""), flags.String("kube-context", "", "")
		namespace := flags.StringP("namespace", "n", "", "")
		flags.String("format", "json", "")
		if err := flags.Parse(args[2:]); err != nil {
			return "", err
		}
		if !*bounded {
			return fallback(ctx, args)
		}
		ref, err := boundedExplainRef(args[1:2], *apiVersion, *namespace, *kubeContext)
		if err != nil {
			return "", err
		}
		summary, err := session.observe(ctx, ref, *kubeContext, *refresh)
		if err != nil {
			return "", err
		}
		output, err := json.Marshal(withExplainJSONHints(summary, HintContext{Mode: HintModeDefault}))
		return string(output), err
	}
}
