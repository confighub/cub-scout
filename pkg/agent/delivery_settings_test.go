// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func deliveryObject(apiVersion, kind, namespace, name string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion, "kind": kind,
		"metadata": map[string]interface{}{"namespace": namespace, "name": name},
	}}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	return obj
}

func settingsApp(namespace, name string, spec map[string]interface{}) *unstructured.Unstructured {
	return deliveryObject("argoproj.io/v1alpha1", "Application", namespace, name, spec)
}

func settingNamed(t *testing.T, deployer DeliveryDeployerSettings, name string) DeliverySetting {
	t.Helper()
	var found []DeliverySetting
	for _, setting := range deployer.Settings {
		if setting.Name == name {
			found = append(found, setting)
		}
	}
	require.Len(t, found, 1, "setting %q in %+v", name, deployer.Settings)
	return found[0]
}

func settingNames(deployer DeliveryDeployerSettings) []string {
	names := make([]string, 0, len(deployer.Settings))
	for _, setting := range deployer.Settings {
		label := setting.Name
		if setting.Category == DeliverySettingOption {
			label += "=" + setting.Value
		}
		names = append(names, label)
	}
	return names
}

// #839: an Application with no automated block syncs manually. That is the
// absence of a declaration, so it is reported as unset with the default named,
// and self-heal and prune, which only exist under automated sync, do not apply.
func TestArgoApplicationWithoutAutomatedSyncIsUnsetNotDeclaredOff(t *testing.T) {
	for name, spec := range map[string]map[string]interface{}{
		"no syncPolicy":   {"project": "payments"},
		"empty policy":    {"project": "payments", "syncPolicy": map[string]interface{}{}},
		"automated: null": {"project": "payments", "syncPolicy": map[string]interface{}{"automated": nil}},
	} {
		t.Run(name, func(t *testing.T) {
			got := ArgoApplicationDeliverySettings(settingsApp("argocd", "ledger", spec))
			require.Equal(t, DeliverySetting{Name: "auto-sync", Category: DeliverySettingPolicy, Value: DeliveryValueUnset,
				Default: DeliveryValueOff, Effective: DeliveryValueOff, Path: "spec.syncPolicy.automated"}, settingNamed(t, got, "auto-sync"))
			for _, policy := range []string{"self-heal", "prune"} {
				setting := settingNamed(t, got, policy)
				require.Equal(t, DeliveryValueNotApplicable, setting.Value)
				require.Empty(t, setting.Effective, "a setting that does not apply has no effective value")
				require.Equal(t, "auto-sync is off", setting.Detail)
			}
			require.Equal(t, DeliveryGroupProject, got.GroupKind)
			require.Equal(t, "payments", got.Group)
		})
	}
}

func TestArgoApplicationAutomatedPolicy(t *testing.T) {
	automated := func(fields map[string]interface{}) *unstructured.Unstructured {
		return settingsApp("argocd", "ledger", map[string]interface{}{"project": "default",
			"syncPolicy": map[string]interface{}{"automated": fields}})
	}
	values := func(app *unstructured.Unstructured) [3]string {
		got := ArgoApplicationDeliverySettings(app)
		return [3]string{settingNamed(t, got, "auto-sync").Value, settingNamed(t, got, "self-heal").Value, settingNamed(t, got, "prune").Value}
	}

	require.Equal(t, [3]string{"on", "on", "on"}, values(automated(map[string]interface{}{"selfHeal": true, "prune": true})))
	require.Equal(t, [3]string{"on", "off", "off"}, values(automated(map[string]interface{}{"selfHeal": false, "prune": false})))

	// An empty automated block turns sync on and declares nothing else.
	empty := ArgoApplicationDeliverySettings(automated(map[string]interface{}{}))
	require.Equal(t, DeliveryValueOn, settingNamed(t, empty, "auto-sync").Value)
	for _, policy := range []string{"self-heal", "prune"} {
		setting := settingNamed(t, empty, policy)
		require.Equal(t, DeliveryValueUnset, setting.Value, policy)
		require.Equal(t, DeliveryValueOff, setting.Default, policy)
		require.Equal(t, DeliveryValueOff, setting.Effective, policy)
	}
	require.Equal(t, "spec.syncPolicy.automated.selfHeal", settingNamed(t, empty, "self-heal").Path)

	// Argo CD 3.0: enabled=false keeps the block and turns automated sync off.
	disabled := ArgoApplicationDeliverySettings(automated(map[string]interface{}{"enabled": false, "selfHeal": true}))
	sync := settingNamed(t, disabled, "auto-sync")
	require.Equal(t, DeliveryValueOff, sync.Value)
	require.Equal(t, "spec.syncPolicy.automated.enabled", sync.Path)
	require.Equal(t, DeliveryValueNotApplicable, settingNamed(t, disabled, "self-heal").Value)
	require.Equal(t, [3]string{"on", "on", "off"}, values(automated(map[string]interface{}{"enabled": true, "selfHeal": true, "prune": false})))

	// A value of the wrong type is not a declared false.
	malformed := settingNamed(t, ArgoApplicationDeliverySettings(automated(map[string]interface{}{"selfHeal": "yes"})), "self-heal")
	require.Equal(t, DeliveryValueUnset, malformed.Value)
	require.Equal(t, "the field is not a boolean", malformed.Detail)
}

func TestArgoApplicationSyncOptionsArePassedThroughWithTheirValues(t *testing.T) {
	app := settingsApp("argocd", "ledger", map[string]interface{}{
		"project": "default",
		"syncPolicy": map[string]interface{}{
			"automated": map[string]interface{}{"allowEmpty": true},
			"syncOptions": []interface{}{
				"Validate=false", "PrunePropagationPolicy=foreground", "Prune=false", " ServerSideApply=true ",
				"SomethingNewer=a=b", "BareOption", "", int64(7),
			},
		},
	})
	got := ArgoApplicationDeliverySettings(app)
	require.Equal(t, []string{
		"auto-sync", "self-heal", "prune",
		"7=", "BareOption=", "Prune=false", "PrunePropagationPolicy=foreground", "ServerSideApply=true",
		"SomethingNewer=a=b", "Validate=false", "automated.allowEmpty=true",
	}, settingNames(got))
	// The automated-prune policy and the Prune sync option are different settings.
	require.Equal(t, DeliverySettingPolicy, settingNamed(t, got, "prune").Category)
	require.Equal(t, DeliverySettingOption, settingNamed(t, got, "Prune").Category)
	require.Equal(t, "spec.syncPolicy.syncOptions", settingNamed(t, got, "Validate").Path)
}

func TestArgoApplicationIgnoreDifferences(t *testing.T) {
	rules := []interface{}{
		map[string]interface{}{"group": "apps", "kind": "Deployment", "jsonPointers": []interface{}{"/spec/replicas"}},
		map[string]interface{}{"kind": "ConfigMap", "jqPathExpressions": []interface{}{".data.x"}},
	}
	spec := func(options ...interface{}) map[string]interface{} {
		return map[string]interface{}{"project": "default", "ignoreDifferences": rules,
			"syncPolicy": map[string]interface{}{"syncOptions": options}}
	}
	compareOnly := ArgoApplicationDeliverySettings(settingsApp("argocd", "a", spec()))
	require.Equal(t, "2 rules, comparison only", settingNamed(t, compareOnly, "ignoreDifferences").Detail)
	require.Equal(t, rules, compareOnly.IgnoreRules, "rules are reported verbatim")

	respected := ArgoApplicationDeliverySettings(settingsApp("argocd", "a", spec("RespectIgnoreDifferences=true")))
	require.Equal(t, "2 rules, also respected on sync", settingNamed(t, respected, "ignoreDifferences").Detail)
	notRespected := ArgoApplicationDeliverySettings(settingsApp("argocd", "a", spec("RespectIgnoreDifferences=false")))
	require.Equal(t, "2 rules, comparison only", settingNamed(t, notRespected, "ignoreDifferences").Detail)

	none := ArgoApplicationDeliverySettings(settingsApp("argocd", "a", map[string]interface{}{"ignoreDifferences": []interface{}{}}))
	require.NotContains(t, settingNames(none), "ignoreDifferences=set")
	require.Nil(t, none.IgnoreRules)
	require.Empty(t, none.Group, "a project that is not declared is not named")
}

func TestArgoApplicationNamesItsApplicationSet(t *testing.T) {
	app := settingsApp("argocd", "ledger-eu", map[string]interface{}{"project": "payments"})
	app.SetOwnerReferences([]metav1.OwnerReference{
		{APIVersion: "v1", Kind: "ConfigMap", Name: "other"},
		{APIVersion: "argoproj.io/v1alpha1", Kind: "ApplicationSet", Name: "ledger"},
	})
	require.Equal(t, "ApplicationSet/ledger", ArgoApplicationDeliverySettings(app).GeneratedBy)
	require.Empty(t, ArgoApplicationDeliverySettings(settingsApp("argocd", "plain", nil)).GeneratedBy)
}

func TestFluxKustomizationDeliverySettings(t *testing.T) {
	declared := FluxKustomizationDeliverySettings(deliveryObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps",
		map[string]interface{}{"prune": false, "suspend": true, "force": true, "wait": false, "deletionPolicy": "Orphan"}))
	require.Equal(t, []string{"suspend", "prune", "force", "wait", "deletionPolicy=Orphan"}, settingNames(declared))
	require.Equal(t, DeliveryValueOn, settingNamed(t, declared, "suspend").Value)
	require.Equal(t, DeliveryValueOff, settingNamed(t, declared, "prune").Value)
	require.Equal(t, DeliveryValueOn, settingNamed(t, declared, "force").Value)
	require.Equal(t, DeliveryValueOff, settingNamed(t, declared, "wait").Value)
	require.Equal(t, DeliveryGroupNamespace, declared.GroupKind)
	require.Equal(t, "flux-system", declared.Group)

	absent := FluxKustomizationDeliverySettings(deliveryObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps", nil))
	require.Equal(t, []string{"suspend", "prune", "force", "wait"}, settingNames(absent))
	for _, name := range []string{"suspend", "force", "wait"} {
		setting := settingNamed(t, absent, name)
		require.Equal(t, DeliveryValueUnset, setting.Value, name)
		require.Equal(t, DeliveryValueOff, setting.Default, name)
	}
	// prune is required by the CRD: when it is missing there is no default to
	// fall back on, and it must not read as "does not prune".
	prune := settingNamed(t, absent, "prune")
	require.Equal(t, DeliveryValueUnset, prune.Value)
	require.Empty(t, prune.Default)
	require.Empty(t, prune.Effective)
}

func TestFluxHelmReleaseDeliverySettings(t *testing.T) {
	ignore := []interface{}{map[string]interface{}{"paths": []interface{}{"/spec/replicas"}, "target": map[string]interface{}{"kind": "Deployment"}}}
	tuned := FluxHelmReleaseDeliverySettings(deliveryObject("helm.toolkit.fluxcd.io/v2", "HelmRelease", "team-h", "tuned", map[string]interface{}{
		"suspend":        false,
		"driftDetection": map[string]interface{}{"mode": "warn", "ignore": ignore},
		"install":        map[string]interface{}{"createNamespace": true, "remediation": map[string]interface{}{"retries": int64(3)}},
		"upgrade": map[string]interface{}{"force": true, "timeout": "5m",
			"remediation": map[string]interface{}{"strategy": "rollback", "retries": float64(2)}},
		"test":   map[string]interface{}{"enable": false, "filters": []interface{}{map[string]interface{}{"name": "smoke"}}},
		"values": map[string]interface{}{"replicaCount": int64(2)},
	}))
	require.Equal(t, []string{
		"suspend", "driftDetection.mode",
		"driftDetection.ignore=set", "install.createNamespace=true", "install.remediation.retries=3",
		"test.enable=false", `test.filters=[{"name":"smoke"}]`, "upgrade.force=true", "upgrade.remediation.retries=2",
		"upgrade.remediation.strategy=rollback", "upgrade.timeout=5m",
	}, settingNames(tuned), "chart values are not delivery settings")
	require.Equal(t, DeliveryValueOff, settingNamed(t, tuned, "suspend").Value)
	require.Equal(t, "warn", settingNamed(t, tuned, "driftDetection.mode").Value)
	require.Equal(t, "1 rule", settingNamed(t, tuned, "driftDetection.ignore").Detail)
	require.Equal(t, ignore, tuned.IgnoreRules)
	require.Equal(t, "spec.upgrade.remediation.strategy", settingNamed(t, tuned, "upgrade.remediation.strategy").Path)

	plain := FluxHelmReleaseDeliverySettings(deliveryObject("helm.toolkit.fluxcd.io/v2", "HelmRelease", "team-h", "plain", nil))
	require.Equal(t, []string{"suspend", "driftDetection.mode"}, settingNames(plain))
	drift := settingNamed(t, plain, "driftDetection.mode")
	require.Equal(t, [3]string{DeliveryValueUnset, "disabled", "disabled"}, [3]string{drift.Value, drift.Default, drift.Effective})
}

var deliveryListKinds = map[schema.GroupVersionResource]string{
	{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}:                  "ApplicationList",
	{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}:      "KustomizationList",
	{Group: "kustomize.toolkit.fluxcd.io", Version: "v1beta2", Resource: "kustomizations"}: "KustomizationList",
	{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}:             "HelmReleaseList",
	{Group: "helm.toolkit.fluxcd.io", Version: "v2beta2", Resource: "helmreleases"}:        "HelmReleaseList",
	{Group: "helm.toolkit.fluxcd.io", Version: "v2beta1", Resource: "helmreleases"}:        "HelmReleaseList",
	{Version: "v1", Resource: "configmaps"}:                                                "ConfigMapList",
}

func deliveryClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), deliveryListKinds, objects...)
}

func argoConfigMap(namespace string, data map[string]interface{}) *unstructured.Unstructured {
	obj := deliveryObject("v1", "ConfigMap", namespace, "argocd-cm", nil)
	if data != nil {
		obj.Object["data"] = data
	}
	return obj
}

// failList makes list calls for one resource (and optionally one version) fail.
func failList(client *dynamicfake.FakeDynamicClient, resource, version string, err error) {
	client.PrependReactor("list", resource, func(action ktesting.Action) (bool, runtime.Object, error) {
		if version != "" && action.GetResource().Version != version {
			return false, nil, nil
		}
		return true, nil, err
	})
}

func readsByKind(inventory DeliverySettingsInventory) map[string]DeliverySettingsRead {
	reads := map[string]DeliverySettingsRead{}
	for _, read := range inventory.Reads {
		reads[read.Kind] = read
	}
	return reads
}

func notFound(resource string) error {
	return apierrors.NewNotFound(schema.GroupResource{Resource: resource}, "")
}

// The checklist item that started #839: a forbidden Application list printed
// nothing, which reads as "no Applications". A list that fails is not read; a
// kind the server does not serve is not installed; neither is an empty result.
func TestCollectDeliverySettingsSeparatesNotReadFromNotInstalledFromEmpty(t *testing.T) {
	client := deliveryClient(
		deliveryObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps", map[string]interface{}{"prune": true}),
	)
	failList(client, "applications", "", apierrors.NewForbidden(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "", nil))
	for _, version := range []string{"v2", "v2beta2", "v2beta1"} {
		failList(client, "helmreleases", version, notFound("helmreleases"))
	}

	inventory := CollectDeliverySettings(context.Background(), client, DeliverySettingsOptions{})
	reads := readsByKind(inventory)
	require.Len(t, inventory.Reads, 3, "every kind is accounted for")

	require.Equal(t, DeliveryReadNotRead, reads["Application"].Status)
	require.Equal(t, "forbidden", reads["Application"].Reason)
	require.NotEmpty(t, reads["Application"].Message)
	require.Equal(t, "applications.argoproj.io/v1alpha1", reads["Application"].Resource)

	require.Equal(t, DeliveryReadNotInstalled, reads["HelmRelease"].Status)
	require.Empty(t, reads["HelmRelease"].Reason)
	require.Equal(t, "helmreleases.helm.toolkit.fluxcd.io/v2", reads["HelmRelease"].Resource)

	require.Equal(t, DeliveryReadRead, reads["Kustomization"].Status)
	require.Equal(t, 1, reads["Kustomization"].Count)
	require.Len(t, inventory.Deployers, 1)
	require.Empty(t, inventory.LinkSources, "no Application was read, so no argocd-cm is looked up")

	// Installed and empty is a third state: read, with nothing found.
	empty := readsByKind(CollectDeliverySettings(context.Background(), deliveryClient(), DeliverySettingsOptions{}))
	require.Equal(t, DeliverySettingsRead{Controller: DeliveryControllerArgoCD, Kind: "Application",
		Resource: "applications.argoproj.io/v1alpha1", Status: DeliveryReadRead}, empty["Application"])
}

func TestCollectDeliverySettingsClassifiesListFailures(t *testing.T) {
	for reason, err := range map[string]error{
		"unauthorized": apierrors.NewUnauthorized("token expired"),
		"timeout":      apierrors.NewTimeoutError("slow", 1),
		"list_failed":  apierrors.NewInternalError(context.DeadlineExceeded),
	} {
		client := deliveryClient()
		failList(client, "kustomizations", "", err)
		read := readsByKind(CollectDeliverySettings(context.Background(), client, DeliverySettingsOptions{}))["Kustomization"]
		require.Equal(t, DeliveryReadNotRead, read.Status, reason)
		require.Equal(t, reason, read.Reason)
	}
}

// An older Flux serves HelmRelease only as v2beta2. Reporting "not installed"
// because v2 is missing would hide every HelmRelease on that cluster.
func TestCollectDeliverySettingsReadsAnOlderServedVersion(t *testing.T) {
	client := deliveryClient(
		deliveryObject("helm.toolkit.fluxcd.io/v2beta2", "HelmRelease", "team-h", "legacy", map[string]interface{}{"suspend": true}),
		deliveryObject("kustomize.toolkit.fluxcd.io/v1beta2", "Kustomization", "flux-system", "legacy", map[string]interface{}{"prune": true}),
	)
	failList(client, "helmreleases", "v2", notFound("helmreleases"))
	failList(client, "kustomizations", "v1", notFound("kustomizations"))

	inventory := CollectDeliverySettings(context.Background(), client, DeliverySettingsOptions{})
	reads := readsByKind(inventory)
	require.Equal(t, DeliveryReadRead, reads["HelmRelease"].Status)
	require.Equal(t, "helmreleases.helm.toolkit.fluxcd.io/v2beta2", reads["HelmRelease"].Resource, "the version that was read is the one named")
	require.Equal(t, "kustomizations.kustomize.toolkit.fluxcd.io/v1beta2", reads["Kustomization"].Resource)
	require.Len(t, inventory.Deployers, 2)

	// A forbidden preferred version is not retried on an older one: the
	// denial is the answer.
	denied := deliveryClient(deliveryObject("helm.toolkit.fluxcd.io/v2beta2", "HelmRelease", "team-h", "legacy", nil))
	failList(denied, "helmreleases", "v2", apierrors.NewForbidden(schema.GroupResource{Resource: "helmreleases"}, "", nil))
	deniedInventory := CollectDeliverySettings(context.Background(), denied, DeliverySettingsOptions{})
	require.Equal(t, DeliveryReadNotRead, readsByKind(deniedInventory)["HelmRelease"].Status)
	require.Empty(t, deniedInventory.Deployers)
}

func TestCollectDeliverySettingsScopesToANamespaceAndSortsDeployers(t *testing.T) {
	client := deliveryClient(
		settingsApp("argocd", "zeta", map[string]interface{}{"project": "payments"}),
		settingsApp("argocd", "alpha", map[string]interface{}{"project": "payments"}),
		settingsApp("argocd", "beta", map[string]interface{}{"project": "data"}),
		settingsApp("team-a", "inside", map[string]interface{}{"project": "data"}),
		deliveryObject("helm.toolkit.fluxcd.io/v2", "HelmRelease", "team-a", "chart", nil),
		deliveryObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps", nil),
	)
	order := func(inventory DeliverySettingsInventory) []string {
		var out []string
		for _, deployer := range inventory.Deployers {
			out = append(out, deployer.Kind+"/"+deployer.Namespace+"/"+deployer.Name)
		}
		return out
	}
	require.Equal(t, []string{
		"Application/argocd/beta", "Application/team-a/inside", "Application/argocd/alpha", "Application/argocd/zeta",
		"HelmRelease/team-a/chart", "Kustomization/flux-system/apps",
	}, order(CollectDeliverySettings(context.Background(), client, DeliverySettingsOptions{})))
	require.Equal(t, []string{"Application/team-a/inside", "HelmRelease/team-a/chart"},
		order(CollectDeliverySettings(context.Background(), client, DeliverySettingsOptions{Namespace: "team-a"})))
}

// A link is shown only when argocd-cm in the Application's own namespace
// declares a usable url. It is never built from a guess or another namespace.
func TestCollectDeliverySettingsLinksOnlyFromADeclaredArgoCDURL(t *testing.T) {
	client := deliveryClient(
		settingsApp("argocd", "ledger api", nil),
		settingsApp("unset", "a", nil),
		settingsApp("script", "a", nil),
		settingsApp("relative", "a", nil),
		settingsApp("denied", "a", nil),
		settingsApp("outside", "a", nil),
		deliveryObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "argocd", "apps", nil),
		argoConfigMap("argocd", map[string]interface{}{"url": " https://argocd.example.test/ "}),
		argoConfigMap("unset", map[string]interface{}{"other": "x"}),
		argoConfigMap("script", map[string]interface{}{"url": "javascript:alert(1)"}),
		argoConfigMap("relative", map[string]interface{}{"url": "argocd.example.test"}),
	)
	client.PrependReactor("get", "configmaps", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() == "denied" {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, "argocd-cm", nil)
		}
		return false, nil, nil
	})

	inventory := CollectDeliverySettings(context.Background(), client, DeliverySettingsOptions{})
	require.Equal(t, []DeliveryLinkSource{
		{Namespace: "argocd", Status: DeliveryLinkFound, URL: "https://argocd.example.test"},
		{Namespace: "denied", Status: DeliveryLinkNotRead, Reason: "forbidden"},
		{Namespace: "outside", Status: DeliveryLinkNotFound},
		{Namespace: "relative", Status: DeliveryLinkInvalid},
		{Namespace: "script", Status: DeliveryLinkInvalid},
		{Namespace: "unset", Status: DeliveryLinkURLUnset},
	}, inventory.LinkSources)
	for _, deployer := range inventory.Deployers {
		switch {
		case deployer.Kind == "Application" && deployer.Namespace == "argocd":
			require.Equal(t, "https://argocd.example.test/applications/argocd/ledger%20api", deployer.URL)
		default:
			require.Empty(t, deployer.URL, "%s %s/%s", deployer.Kind, deployer.Namespace, deployer.Name)
		}
	}
}

func TestGroupDeliverySettingsInvertsDeployersBySettingValue(t *testing.T) {
	deployers := []DeliveryDeployerSettings{
		ArgoApplicationDeliverySettings(settingsApp("argocd", "manual", map[string]interface{}{"project": "payments"})),
		ArgoApplicationDeliverySettings(settingsApp("argocd", "partial", map[string]interface{}{"project": "payments",
			"syncPolicy": map[string]interface{}{"automated": map[string]interface{}{"prune": true},
				"syncOptions": []interface{}{"Validate=false", "ServerSideApply=true"}}})),
		ArgoApplicationDeliverySettings(settingsApp("argocd", "full", map[string]interface{}{"project": "payments",
			"syncPolicy": map[string]interface{}{"automated": map[string]interface{}{"prune": true, "selfHeal": true},
				"syncOptions": []interface{}{"ServerSideApply=true"}}})),
		ArgoApplicationDeliverySettings(settingsApp("argocd", "other", map[string]interface{}{"project": "data",
			"syncPolicy": map[string]interface{}{"automated": map[string]interface{}{"selfHeal": false}}})),
		FluxKustomizationDeliverySettings(deliveryObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps", nil)),
	}
	deployers[1].URL = "https://argocd.example.test/applications/argocd/partial"

	groups := GroupDeliverySettings(deployers, true)
	titles := make([]string, 0, len(groups))
	for _, group := range groups {
		titles = append(titles, group.Controller+"/"+group.Kind+"/"+group.GroupKind+"/"+group.Group)
	}
	require.Equal(t, []string{"ArgoCD/Application/project/data", "ArgoCD/Application/project/payments",
		"Flux/Kustomization/namespace/flux-system"}, titles)

	payments := groups[1]
	require.Equal(t, 3, payments.Deployers)
	settingOrder := make([]string, 0, len(payments.Settings))
	summaries := map[string]DeliverySettingSummary{}
	for _, summary := range payments.Settings {
		settingOrder = append(settingOrder, summary.Name)
		summaries[summary.Name] = summary
	}
	require.Equal(t, []string{"auto-sync", "self-heal", "prune", "ServerSideApply", "Validate"}, settingOrder,
		"policies in declared order, then options by name")

	refNames := func(value DeliverySettingValue) []string {
		var out []string
		for _, ref := range value.Deployers {
			out = append(out, ref.Name)
		}
		return out
	}
	heal := summaries["self-heal"].Values
	require.Equal(t, []string{"on", "off", "n/a"}, []string{heal[0].Value, heal[1].Value, heal[2].Value})
	require.Equal(t, []string{"full"}, refNames(heal[0]))
	// "partial" does not declare selfHeal: it counts under the default, marked unset.
	require.Equal(t, []string{"partial"}, refNames(heal[1]))
	require.Equal(t, 1, heal[1].Unset)
	require.True(t, heal[1].Deployers[0].Unset)
	require.Equal(t, "https://argocd.example.test/applications/argocd/partial", heal[1].Deployers[0].URL)
	require.Equal(t, []string{"manual"}, refNames(heal[2]))
	require.Equal(t, "auto-sync is off", heal[2].Deployers[0].Detail)
	require.Zero(t, heal[2].Unset, "a setting that does not apply is not an unset default")
	require.Equal(t, []string{"full", "partial"}, refNames(summaries["ServerSideApply"].Values[0]))

	// A required field with no default stays "unset"; it is not folded into off.
	prune := groups[2].Settings[1]
	require.Equal(t, "prune", prune.Name)
	require.Equal(t, DeliveryValueUnset, prune.Values[0].Value)
	require.Zero(t, prune.Values[0].Unset)

	all := GroupDeliverySettings(deployers, false)
	require.Len(t, all, 2)
	require.Equal(t, DeliveryGroupAll, all[0].GroupKind)
	require.Empty(t, all[0].Group)
	require.Equal(t, 4, all[0].Deployers)
	require.Empty(t, GroupDeliverySettings(nil, true))
}

func TestDeliverySettingMatch(t *testing.T) {
	partial := ArgoApplicationDeliverySettings(settingsApp("argocd", "partial", map[string]interface{}{
		"syncPolicy": map[string]interface{}{"automated": map[string]interface{}{"selfHeal": true},
			"syncOptions": []interface{}{"Validate=false", "Prune=false"}}}))
	manual := ArgoApplicationDeliverySettings(settingsApp("argocd", "manual", nil))
	kustomization := FluxKustomizationDeliverySettings(deliveryObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps", nil))

	for _, tc := range []struct {
		match    DeliverySettingMatch
		deployer DeliveryDeployerSettings
		want     bool
		why      string
	}{
		{DeliverySettingMatch{Name: "self-heal", Value: "on"}, partial, true, "declared on"},
		{DeliverySettingMatch{Name: "self-heal", Value: "true"}, partial, true, "true is on"},
		{DeliverySettingMatch{Name: "prune", Value: "off"}, partial, true, "unset counts under the controller default"},
		{DeliverySettingMatch{Name: "prune", Value: "unset"}, partial, true, "and can be asked for as unset"},
		{DeliverySettingMatch{Name: "Prune", Value: "off"}, partial, true, "the sync option Prune=false"},
		{DeliverySettingMatch{Name: "Prune", Value: "true"}, partial, false, ""},
		{DeliverySettingMatch{Name: "validate", Value: "false"}, partial, false, "names are exact"},
		{DeliverySettingMatch{Name: "Validate", AnyValue: true}, partial, true, "any value"},
		{DeliverySettingMatch{Name: "Validate", AnyValue: true}, manual, false, "the option is not declared"},
		{DeliverySettingMatch{Name: "self-heal", Value: "off"}, manual, false, "n/a is not off"},
		{DeliverySettingMatch{Name: "self-heal", Value: "n/a"}, manual, true, ""},
		{DeliverySettingMatch{Name: "auto-sync", Value: "off"}, manual, true, "manual sync by default"},
		{DeliverySettingMatch{Name: "prune", Value: "off"}, kustomization, false, "a missing required field is not off"},
		{DeliverySettingMatch{Name: "prune", Value: "unset"}, kustomization, true, ""},
	} {
		require.Equal(t, tc.want, tc.match.Matches(tc.deployer), "%+v on %s: %s", tc.match, tc.deployer.Name, tc.why)
	}
}

// The objects in testdata/delivery-settings were returned by a real API server
// running Argo CD and Flux (examples/delivery-settings/verify-live.py).
func TestDeliverySettingsFromRecordedObjects(t *testing.T) {
	load := func(name string) []unstructured.Unstructured {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", "delivery-settings", name))
		require.NoError(t, err)
		var list unstructured.UnstructuredList
		require.NoError(t, json.Unmarshal(data, &list))
		require.NotEmpty(t, list.Items, name)
		return list.Items
	}
	declared := map[string]map[string]string{}
	record := func(deployer DeliveryDeployerSettings) {
		values := map[string]string{}
		for _, setting := range deployer.Settings {
			values[setting.Name] = setting.Value
		}
		declared[deployer.Kind+"/"+deployer.Namespace+"/"+deployer.Name] = values
	}
	for _, item := range load("applications.json") {
		record(ArgoApplicationDeliverySettings(&item))
	}
	for _, item := range load("kustomizations.json") {
		record(FluxKustomizationDeliverySettings(&item))
	}
	for _, item := range load("helmreleases.json") {
		record(FluxHelmReleaseDeliverySettings(&item))
	}
	var want map[string]map[string]string
	data, err := os.ReadFile(filepath.Join("testdata", "delivery-settings", "expected-declared.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &want))
	require.Equal(t, want, declared)
}
