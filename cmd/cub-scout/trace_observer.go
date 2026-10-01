// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

// traceObservationOptions contains observation choices, independently of CLI
// globals and rendering. A Flux factory must use the supplied captured session;
// it owns any temporary credential files and returns their cleanup function.
type traceObservationOptions struct {
	DirectApplication bool
	Artifacts         bool
	Delivery          traceConfigHubDeliveryFlags
	Flux              func(*traceSession) (agent.Tracer, func() error, error)
}

type traceObservation struct {
	Result    *agent.TraceResult
	Artifacts map[string]mapsvc.TraceArtifactRef
}

// observeTrace is the common rich observation engine. It never resolves ambient
// configuration or renders output. CLI, MCP and TUI adapters supply the binding.
func observeTrace(ctx context.Context, session *traceSession, kind, name, namespace string, opts traceObservationOptions) (observation *traceObservation, returnErr error) {
	if session == nil {
		return nil, fmt.Errorf("trace session is unavailable")
	}
	dyn, err := session.dynamicClient()
	if err != nil {
		return nil, err
	}
	argo := agent.NewArgoTracerWithKubernetesClient(dyn)
	var cleanups []func() error
	defer func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if err := cleanups[i](); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("unable to remove temporary Trace credentials"))
			}
		}
	}()
	flux := func() (agent.Tracer, error) {
		if opts.Flux == nil {
			return nil, fmt.Errorf("captured Flux adapter is unavailable")
		}
		tracer, cleanup, err := opts.Flux(session)
		if cleanup != nil {
			cleanups = append(cleanups, cleanup)
		}
		if err != nil {
			return nil, err
		}
		if tracer == nil || !tracer.Available() {
			return nil, fmt.Errorf("flux CLI is unavailable")
		}
		return tracer, nil
	}

	var result *agent.TraceResult
	var ownership *agent.Ownership
	if opts.DirectApplication {
		if kind != "Application" {
			return nil, fmt.Errorf("direct Application trace requires kind Application")
		}
		result, err = argo.TraceApplicationInNamespace(ctx, name, namespace)
		ownership = &agent.Ownership{Type: agent.OwnerArgo}
	} else {
		ownership, err = detectResourceOwnershipWithTraceSession(ctx, session, kind, name, namespace)
		if err != nil {
			return nil, fmt.Errorf("ownership detection failed for %s/%s in %s: %w", kind, name, namespace, err)
		}
		switch ownership.Type {
		case agent.OwnerFlux:
			var tracer agent.Tracer
			tracer, err = flux()
			if err == nil {
				result, err = tracer.Trace(ctx, kind, name, namespace)
			}
		case agent.OwnerArgo:
			if kind == "Application" {
				result, err = argo.TraceApplicationInNamespace(ctx, name, namespace)
			} else if ownership.Name != "" {
				// Workload tracking metadata usually identifies the Application's
				// name but not its namespace. Resolve uniquely, never guess.
				result, err = argo.TraceApplicationInNamespace(ctx, ownership.Name, ownership.Namespace)
				if err == nil && result != nil {
					// The Application is the provenance chain, not the selected
					// observation target. Keep CLI and TUI anchored to the workload.
					result.Object = agent.ResourceRef{Kind: kind, Name: name, Namespace: namespace}
				}
			} else {
				err = fmt.Errorf("Argo ownership does not identify an Application")
			}
		case agent.OwnerHelm:
			client, clientErr := session.kubernetesClient()
			if clientErr != nil {
				return nil, clientErr
			}
			tracer := agent.NewHelmTracer(client)
			if ownership.Name != "" {
				result, err = tracer.TraceRelease(ctx, ownership.Name, namespace)
			} else {
				result, err = tracer.Trace(ctx, kind, name, namespace)
			}
			if shouldAttemptHelmViaArgoFallback(ownership, result) {
				fallback, fallbackOwner, used := tryHelmViaArgoFallback(ctx, dyn, kind, name, namespace, ownership, result, argo.TraceApplicationInNamespace)
				if used {
					result, ownership, err = fallback, fallbackOwner, nil
				}
			}
		case agent.OwnerCustom:
			result = buildCustomOwnerUnsupportedTraceResult(kind, name, namespace, ownership)
		case agent.OwnerSveltos:
			result = buildSveltosObservedTraceResult(ctx, dyn, kind, name, namespace, ownership)
		case agent.OwnerModelplane:
			result = buildModelplaneObservedTraceResult(ctx, dyn, kind, name, namespace, ownership)
		case agent.OwnerCrossplane:
			result = buildCrossplaneObservedTraceResult(kind, name, namespace, ownership)
		default:
			// An Application is readable directly even without ownership labels.
			if kind == "Application" {
				result, err = argo.TraceApplicationInNamespace(ctx, name, namespace)
				break
			}
			var tracer agent.Tracer
			tracer, err = flux()
			if err == nil {
				result, err = tracer.Trace(ctx, kind, name, namespace)
			}
			if err == nil && (result == nil || (result.Error != "" && !strings.Contains(result.Error, "not managed"))) {
				err = fmt.Errorf("resource not managed by a detected GitOps tool")
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("trace failed: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("trace returned no observation")
	}
	if namespace == "" && opts.DirectApplication {
		namespace = result.Object.Namespace
	}
	result.Context = session.contextLabel()
	result.DetectedOwner = ownership.Type
	if len(result.Chain) > 0 {
		enrichTraceWithTimingSession(ctx, session, result)
	}
	if kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" || kind == "Pod" {
		refs, refErr := detectCrossOwnerReferencesWithTraceSession(ctx, session, kind, name, namespace, ownership)
		if refErr == nil {
			result.CrossReferences = refs
		} else {
			result.Error = appendSentence(result.Error, "Cross-owner references unavailable: "+refErr.Error())
		}
	}
	secrets, secretErr := collectSecretEvidenceWithTraceSessionAndError(ctx, session, kind, name, namespace)
	if secrets != nil && secrets.Summary.Total > 0 {
		result.Secrets = secrets
	}
	if secretErr != nil {
		result.Error = appendSentence(result.Error, secretErr.Error())
	}
	events, eventErr := fetchResourceEventsWithTraceSession(ctx, session, namespace, kind, name)
	if eventErr == nil && events != nil && len(events.Events) > 0 {
		result.Events = events
	} else if eventErr != nil {
		result.Error = appendSentence(result.Error, "Events unavailable: "+eventErr.Error())
	}
	if opts.Delivery.Enabled {
		client := enrichTraceConfigHubFromLiveWithTraceSession(ctx, session, result, kind, name, namespace)
		flags := opts.Delivery
		flags.Namespace = namespace
		attachTraceConfigHubDeliveryEvidenceWithTraceSession(ctx, result, client, session, flags)
	}
	artifacts := buildUnknownTraceArtifacts(result)
	if opts.Artifacts {
		observedArtifacts, artifactErrors := collectTraceArtifactsWithTraceSessionAndErrors(ctx, session, result)
		artifacts = mergeTraceArtifacts(artifacts, observedArtifacts)
		for _, artifactErr := range artifactErrors {
			result.Error = appendSentence(result.Error, artifactErr.Error())
		}
	}
	return &traceObservation{Result: result, Artifacts: artifacts}, nil
}

// capturedTraceFluxFactory keeps child credentials private and scoped to the
// same session as in-process readers. observeTrace invokes cleanup on every exit.
func capturedTraceFluxFactory(session *traceSession) (agent.Tracer, func() error, error) {
	config, err := session.createChildKubeconfig()
	if err != nil {
		return nil, nil, err
	}
	return agent.NewFluxTracerWithKubeconfig(config.Path, config.Context), config.Cleanup, nil
}
