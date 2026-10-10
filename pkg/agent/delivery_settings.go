// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Delivery settings are what a deployer's own spec says about how it applies,
// corrects and deletes (#839). They are declared configuration read from the
// object, not observed behaviour and not a verdict on whether a setting is
// acceptable.

const (
	DeliveryControllerArgoCD = "ArgoCD"
	DeliveryControllerFlux   = "Flux"

	// DeliverySettingPolicy rows are a fixed set per kind and are always
	// present. DeliverySettingOption rows appear only when declared and are
	// passed through under the controller's own name.
	DeliverySettingPolicy = "policy"
	DeliverySettingOption = "option"

	DeliveryValueOn            = "on"
	DeliveryValueOff           = "off"
	DeliveryValueSet           = "set"
	DeliveryValueUnset         = "unset"
	DeliveryValueNotApplicable = "n/a"

	DeliveryGroupProject   = "project"
	DeliveryGroupNamespace = "namespace"
	DeliveryGroupAll       = "all"

	DeliveryReadRead         = "read"
	DeliveryReadNotInstalled = "not_installed"
	DeliveryReadNotRead      = "not_read"

	DeliveryLinkFound    = "found"
	DeliveryLinkURLUnset = "url_unset"
	DeliveryLinkInvalid  = "invalid"
	DeliveryLinkNotFound = "not_found"
	DeliveryLinkNotRead  = "not_read"
)

// DeliverySetting is one declared setting of one deployer.
type DeliverySetting struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	// Value is what the spec declares, DeliveryValueUnset when the field is
	// absent, or DeliveryValueNotApplicable when another setting makes it moot.
	Value string `json:"value"`
	// Default names the controller's documented default. It is set only when
	// Value is unset and the default is documented; an absent field is never
	// reported as a declared false.
	Default string `json:"default,omitempty"`
	// Effective is Value when declared, otherwise Default. It is empty when
	// the field is absent and no default is documented.
	Effective string `json:"effective,omitempty"`
	Path      string `json:"path"`
	Detail    string `json:"detail,omitempty"`
}

// DeliveryDeployerSettings is the settings of one deployer object.
type DeliveryDeployerSettings struct {
	Controller string `json:"controller"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	// GroupKind is "project" for an Argo CD Application and "namespace" for a
	// Flux object, which has no project.
	GroupKind string `json:"groupKind"`
	Group     string `json:"group"`
	// GeneratedBy names an owning ApplicationSet, whose template is where the
	// settings were written.
	GeneratedBy string            `json:"generatedBy,omitempty"`
	URL         string            `json:"url,omitempty"`
	Settings    []DeliverySetting `json:"settings"`
	// IgnoreRules are the declared ignore rules, verbatim.
	IgnoreRules []interface{} `json:"ignoreRules,omitempty"`
}

// DeliverySettingsRead records one list call: what was asked and whether it
// was answered. A kind that could not be listed is not a kind with no objects.
type DeliverySettingsRead struct {
	Controller string `json:"controller"`
	Kind       string `json:"kind"`
	Resource   string `json:"resource"`
	Status     string `json:"status"`
	Count      int    `json:"count"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
}

// DeliveryLinkSource records the lookup of the Argo CD external URL in one
// namespace's argocd-cm ConfigMap.
type DeliveryLinkSource struct {
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	URL       string `json:"url,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// DeliverySettingsInventory is everything one collection read.
type DeliverySettingsInventory struct {
	Deployers   []DeliveryDeployerSettings `json:"deployers"`
	Reads       []DeliverySettingsRead     `json:"reads"`
	LinkSources []DeliveryLinkSource       `json:"linkSources,omitempty"`
}

// DeliverySettingsOptions scopes a collection. An empty Namespace lists every
// namespace.
type DeliverySettingsOptions struct {
	Namespace string
}

func declaredSetting(name, category, value, path string) DeliverySetting {
	return DeliverySetting{Name: name, Category: category, Value: value, Effective: value, Path: path}
}

func unsetSetting(name, path, defaultValue string) DeliverySetting {
	return DeliverySetting{Name: name, Category: DeliverySettingPolicy, Value: DeliveryValueUnset,
		Default: defaultValue, Effective: defaultValue, Path: path}
}

func onOff(value bool) string {
	if value {
		return DeliveryValueOn
	}
	return DeliveryValueOff
}

// boolPolicy reads one boolean field. A field that is absent, or holds
// something other than a boolean, is unset; it is not false.
func boolPolicy(obj map[string]interface{}, name, defaultValue string, fields ...string) DeliverySetting {
	path := strings.Join(fields, ".")
	value, found, err := unstructured.NestedBool(obj, fields...)
	if err != nil || !found {
		setting := unsetSetting(name, path, defaultValue)
		if err != nil {
			setting.Detail = "the field is not a boolean"
		}
		return setting
	}
	return declaredSetting(name, DeliverySettingPolicy, onOff(value), path)
}

// nestedMapIfPresent returns the map at fields. A null or non-map value is
// reported as absent: `automated: null` declares nothing.
func nestedMapIfPresent(obj map[string]interface{}, fields ...string) (map[string]interface{}, bool) {
	value, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if err != nil || !found {
		return nil, false
	}
	asMap, ok := value.(map[string]interface{})
	return asMap, ok
}

func scalarString(value interface{}) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case bool:
		return strconv.FormatBool(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case int:
		return strconv.Itoa(typed), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	}
	return "", false
}

// ArgoApplicationDeliverySettings reads the sync policy of an Argo CD
// Application from its spec.
func ArgoApplicationDeliverySettings(app *unstructured.Unstructured) DeliveryDeployerSettings {
	project, _, _ := unstructured.NestedString(app.Object, "spec", "project")
	out := DeliveryDeployerSettings{
		Controller: DeliveryControllerArgoCD,
		Kind:       "Application",
		Namespace:  app.GetNamespace(),
		Name:       app.GetName(),
		GroupKind:  DeliveryGroupProject,
		Group:      project,
	}
	for _, owner := range app.GetOwnerReferences() {
		if owner.Kind == "ApplicationSet" {
			out.GeneratedBy = "ApplicationSet/" + owner.Name
			break
		}
	}

	const automatedPath = "spec.syncPolicy.automated"
	automated, hasAutomated := nestedMapIfPresent(app.Object, "spec", "syncPolicy", "automated")
	autoSync := unsetSetting("auto-sync", automatedPath, DeliveryValueOff)
	if hasAutomated {
		autoSync = declaredSetting("auto-sync", DeliverySettingPolicy, DeliveryValueOn, automatedPath)
		// Argo CD 3.1 added automated.enabled; an explicit false turns
		// automated sync off while the block stays.
		if enabled, found, err := unstructured.NestedBool(automated, "enabled"); err == nil && found && !enabled {
			autoSync = declaredSetting("auto-sync", DeliverySettingPolicy, DeliveryValueOff, automatedPath+".enabled")
		}
	}
	out.Settings = append(out.Settings, autoSync)
	for _, policy := range []struct{ name, field string }{{"self-heal", "selfHeal"}, {"prune", "prune"}} {
		if autoSync.Effective != DeliveryValueOn {
			out.Settings = append(out.Settings, DeliverySetting{
				Name: policy.name, Category: DeliverySettingPolicy, Value: DeliveryValueNotApplicable,
				Path: automatedPath + "." + policy.field, Detail: "auto-sync is off",
			})
			continue
		}
		setting := boolPolicy(automated, policy.name, DeliveryValueOff, policy.field)
		setting.Path = automatedPath + "." + policy.field
		out.Settings = append(out.Settings, setting)
	}

	var options []DeliverySetting
	if hasAutomated {
		if allowEmpty, found, err := unstructured.NestedBool(automated, "allowEmpty"); err == nil && found {
			options = append(options, declaredSetting("automated.allowEmpty", DeliverySettingOption,
				strconv.FormatBool(allowEmpty), automatedPath+".allowEmpty"))
		}
	}
	respectIgnore := false
	syncOptions, _, _ := unstructured.NestedSlice(app.Object, "spec", "syncPolicy", "syncOptions")
	for _, raw := range syncOptions {
		// Options are passed through as written; an option this code has
		// never heard of is still a declared option.
		text, ok := raw.(string)
		if !ok {
			text = fmt.Sprint(raw)
		}
		name, value, _ := strings.Cut(strings.TrimSpace(text), "=")
		if name == "" {
			continue
		}
		if name == "RespectIgnoreDifferences" && value == "true" {
			respectIgnore = true
		}
		options = append(options, declaredSetting(name, DeliverySettingOption, value, "spec.syncPolicy.syncOptions"))
	}
	if rules, _, _ := unstructured.NestedSlice(app.Object, "spec", "ignoreDifferences"); len(rules) > 0 {
		setting := declaredSetting("ignoreDifferences", DeliverySettingOption, DeliveryValueSet, "spec.ignoreDifferences")
		setting.Detail = ruleCount(len(rules)) + ", comparison only"
		if respectIgnore {
			setting.Detail = ruleCount(len(rules)) + ", also respected on sync"
		}
		options = append(options, setting)
		out.IgnoreRules = rules
	}
	out.Settings = append(out.Settings, sortedOptions(options)...)
	return out
}

// FluxKustomizationDeliverySettings reads the reconciliation settings of a
// Flux Kustomization from its spec.
func FluxKustomizationDeliverySettings(ks *unstructured.Unstructured) DeliveryDeployerSettings {
	out := DeliveryDeployerSettings{
		Controller: DeliveryControllerFlux,
		Kind:       "Kustomization",
		Namespace:  ks.GetNamespace(),
		Name:       ks.GetName(),
		GroupKind:  DeliveryGroupNamespace,
		Group:      ks.GetNamespace(),
	}
	out.Settings = append(out.Settings,
		boolPolicy(ks.Object, "suspend", DeliveryValueOff, "spec", "suspend"),
		// spec.prune is required by the CRD, so there is no default to name.
		boolPolicy(ks.Object, "prune", "", "spec", "prune"),
		boolPolicy(ks.Object, "force", DeliveryValueOff, "spec", "force"),
		boolPolicy(ks.Object, "wait", DeliveryValueOff, "spec", "wait"),
	)
	if policy, found, err := unstructured.NestedString(ks.Object, "spec", "deletionPolicy"); err == nil && found && policy != "" {
		out.Settings = append(out.Settings, declaredSetting("deletionPolicy", DeliverySettingOption, policy, "spec.deletionPolicy"))
	}
	return out
}

// fluxHelmReleaseActionBlocks are the HelmRelease spec blocks whose scalar
// fields configure how a Helm action runs.
var fluxHelmReleaseActionBlocks = []string{"install", "upgrade", "rollback", "uninstall", "test"}

// FluxHelmReleaseDeliverySettings reads the reconciliation settings of a Flux
// HelmRelease from its spec.
func FluxHelmReleaseDeliverySettings(hr *unstructured.Unstructured) DeliveryDeployerSettings {
	out := DeliveryDeployerSettings{
		Controller: DeliveryControllerFlux,
		Kind:       "HelmRelease",
		Namespace:  hr.GetNamespace(),
		Name:       hr.GetName(),
		GroupKind:  DeliveryGroupNamespace,
		Group:      hr.GetNamespace(),
	}
	out.Settings = append(out.Settings, boolPolicy(hr.Object, "suspend", DeliveryValueOff, "spec", "suspend"))
	const driftPath = "spec.driftDetection.mode"
	drift := unsetSetting("driftDetection.mode", driftPath, "disabled")
	if mode, found, err := unstructured.NestedString(hr.Object, "spec", "driftDetection", "mode"); err == nil && found && mode != "" {
		drift = declaredSetting("driftDetection.mode", DeliverySettingPolicy, mode, driftPath)
	}
	out.Settings = append(out.Settings, drift)

	var options []DeliverySetting
	for _, block := range fluxHelmReleaseActionBlocks {
		if fields, ok := nestedMapIfPresent(hr.Object, "spec", block); ok {
			options = appendScalarLeaves(options, block, fields)
		}
	}
	if rules, _, _ := unstructured.NestedSlice(hr.Object, "spec", "driftDetection", "ignore"); len(rules) > 0 {
		setting := declaredSetting("driftDetection.ignore", DeliverySettingOption, DeliveryValueSet, "spec.driftDetection.ignore")
		setting.Detail = ruleCount(len(rules))
		options = append(options, setting)
		out.IgnoreRules = rules
	}
	out.Settings = append(out.Settings, sortedOptions(options)...)
	return out
}

// appendScalarLeaves reports every declared leaf under a spec block by its
// dotted path. A leaf that is not a scalar is reported as its JSON.
func appendScalarLeaves(options []DeliverySetting, prefix string, fields map[string]interface{}) []DeliverySetting {
	for key, value := range fields {
		name := prefix + "." + key
		if nested, ok := value.(map[string]interface{}); ok {
			options = appendScalarLeaves(options, name, nested)
			continue
		}
		text, ok := scalarString(value)
		if !ok {
			encoded, err := json.Marshal(value)
			if err != nil {
				continue
			}
			text = string(encoded)
		}
		options = append(options, declaredSetting(name, DeliverySettingOption, text, "spec."+name))
	}
	return options
}

func ruleCount(n int) string {
	if n == 1 {
		return "1 rule"
	}
	return strconv.Itoa(n) + " rules"
}

func sortedOptions(options []DeliverySetting) []DeliverySetting {
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].Name != options[j].Name {
			return options[i].Name < options[j].Name
		}
		return options[i].Value < options[j].Value
	})
	return options
}

type deliverySettingsSource struct {
	controller string
	kind       string
	group      string
	resource   string
	// versions are tried in order; a served older version is read when the
	// preferred one is not installed.
	versions []string
	parse    func(*unstructured.Unstructured) DeliveryDeployerSettings
}

var deliverySettingsSources = []deliverySettingsSource{
	{DeliveryControllerArgoCD, "Application", "argoproj.io", "applications", []string{"v1alpha1"}, ArgoApplicationDeliverySettings},
	{DeliveryControllerFlux, "Kustomization", "kustomize.toolkit.fluxcd.io", "kustomizations", []string{"v1", "v1beta2", "v1beta1"}, FluxKustomizationDeliverySettings},
	{DeliveryControllerFlux, "HelmRelease", "helm.toolkit.fluxcd.io", "helmreleases", []string{"v2", "v2beta2", "v2beta1"}, FluxHelmReleaseDeliverySettings},
}

func deliveryReadReason(err error) string {
	switch {
	case apierrors.IsForbidden(err):
		return "forbidden"
	case apierrors.IsUnauthorized(err):
		return "unauthorized"
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return "timeout"
	default:
		return "list_failed"
	}
}

// listedDeployer is one deployer object as the API server returned it, with
// the source that says how to read it.
type listedDeployer struct {
	source deliverySettingsSource
	object *unstructured.Unstructured
}

// listDeliveryObjects lists every kind of deployer this package reads, in one
// namespace or, with an empty namespace, in all of them. Every list is
// recorded: a kind whose list failed is "not_read", and only a kind the API
// server does not serve is "not_installed".
func listDeliveryObjects(ctx context.Context, client dynamic.Interface, namespace string) ([]listedDeployer, []DeliverySettingsRead) {
	objects, reads := []listedDeployer{}, []DeliverySettingsRead{}
	for _, source := range deliverySettingsSources {
		read := DeliverySettingsRead{Controller: source.controller, Kind: source.kind, Status: DeliveryReadNotInstalled}
		for _, version := range source.versions {
			gvr := schema.GroupVersionResource{Group: source.group, Version: version, Resource: source.resource}
			read.Resource = fmt.Sprintf("%s.%s/%s", source.resource, source.group, version)
			list, err := client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				read.Status, read.Reason, read.Message = DeliveryReadNotRead, deliveryReadReason(err), err.Error()
				break
			}
			read.Status, read.Count = DeliveryReadRead, len(list.Items)
			for i := range list.Items {
				objects = append(objects, listedDeployer{source: source, object: &list.Items[i]})
			}
			break
		}
		if read.Status == DeliveryReadNotInstalled {
			read.Resource = fmt.Sprintf("%s.%s/%s", source.resource, source.group, source.versions[0])
		}
		reads = append(reads, read)
	}
	return objects, reads
}

// CollectDeliverySettings lists Argo CD Applications and Flux Kustomizations
// and HelmReleases and reads each one's delivery settings. Every list is
// recorded in Reads: a kind whose list failed is "not_read", and only a kind
// the API server does not serve is "not_installed".
func CollectDeliverySettings(ctx context.Context, client dynamic.Interface, opts DeliverySettingsOptions) DeliverySettingsInventory {
	inventory := DeliverySettingsInventory{Deployers: []DeliveryDeployerSettings{}, Reads: []DeliverySettingsRead{}}
	argoNamespaces := map[string]bool{}
	objects, reads := listDeliveryObjects(ctx, client, opts.Namespace)
	inventory.Reads = reads
	for _, object := range objects {
		deployer := object.source.parse(object.object)
		if object.source.controller == DeliveryControllerArgoCD {
			argoNamespaces[deployer.Namespace] = true
		}
		inventory.Deployers = append(inventory.Deployers, deployer)
	}
	sort.SliceStable(inventory.Deployers, func(i, j int) bool {
		a, b := inventory.Deployers[i], inventory.Deployers[j]
		for _, pair := range [][2]string{{a.Controller, b.Controller}, {a.Kind, b.Kind}, {a.Group, b.Group}, {a.Namespace, b.Namespace}} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return a.Name < b.Name
	})

	namespaces := make([]string, 0, len(argoNamespaces))
	for namespace := range argoNamespaces {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	bases := map[string]string{}
	for _, namespace := range namespaces {
		link := argoCDLinkSource(ctx, client, namespace)
		inventory.LinkSources = append(inventory.LinkSources, link)
		if link.Status == DeliveryLinkFound {
			bases[namespace] = link.URL
		}
	}
	for i := range inventory.Deployers {
		deployer := &inventory.Deployers[i]
		if base := bases[deployer.Namespace]; base != "" && deployer.Controller == DeliveryControllerArgoCD {
			deployer.URL = base + "/applications/" + url.PathEscape(deployer.Namespace) + "/" + url.PathEscape(deployer.Name)
		}
	}
	return inventory
}

// argoCDLinkSource reads the external URL Argo CD is configured with from the
// argocd-cm ConfigMap in the namespace an Application lives in. An Application
// outside the Argo CD namespace has no such ConfigMap beside it; its link is
// reported as not found, not constructed from another namespace's URL.
func argoCDLinkSource(ctx context.Context, client dynamic.Interface, namespace string) DeliveryLinkSource {
	link := DeliveryLinkSource{Namespace: namespace}
	configMap, err := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).
		Namespace(namespace).Get(ctx, "argocd-cm", metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		link.Status = DeliveryLinkNotFound
		return link
	case err != nil:
		link.Status, link.Reason = DeliveryLinkNotRead, deliveryReadReason(err)
		return link
	}
	raw, _, _ := unstructured.NestedString(configMap.Object, "data", "url")
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		link.Status = DeliveryLinkURLUnset
		return link
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		link.Status = DeliveryLinkInvalid
		return link
	}
	link.Status, link.URL = DeliveryLinkFound, raw
	return link
}

// DeliveryDeployerRef names one deployer inside a group.
type DeliveryDeployerRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	URL       string `json:"url,omitempty"`
	Detail    string `json:"detail,omitempty"`
	// Unset is true when the deployer does not declare the setting and is
	// counted under the controller's default.
	Unset bool `json:"unset,omitempty"`
}

// DeliverySettingValue is the deployers that share one value of one setting.
type DeliverySettingValue struct {
	Value     string                `json:"value"`
	Count     int                   `json:"count"`
	Unset     int                   `json:"unset,omitempty"`
	Deployers []DeliveryDeployerRef `json:"deployers"`
}

// DeliverySettingSummary is one setting across the deployers of a group.
type DeliverySettingSummary struct {
	Name     string                 `json:"name"`
	Category string                 `json:"category"`
	Values   []DeliverySettingValue `json:"values"`
}

// DeliverySettingsGroup is the deployers of one kind in one project or
// namespace, or of one kind across the whole read.
type DeliverySettingsGroup struct {
	Controller string                   `json:"controller"`
	Kind       string                   `json:"kind"`
	GroupKind  string                   `json:"groupKind"`
	Group      string                   `json:"group,omitempty"`
	Deployers  int                      `json:"deployers"`
	Settings   []DeliverySettingSummary `json:"settings"`
}

func deliveryValueRank(value string) int {
	switch value {
	case DeliveryValueOn:
		return 0
	case DeliveryValueOff:
		return 1
	case DeliveryValueUnset:
		return 3
	case DeliveryValueNotApplicable:
		return 4
	}
	return 2
}

// GroupDeliverySettings inverts the per-deployer settings: for each setting,
// which deployers have which value. With byGroup the deployers of a kind are
// split by project (Argo CD) or namespace (Flux); without it each kind is one
// group.
func GroupDeliverySettings(deployers []DeliveryDeployerSettings, byGroup bool) []DeliverySettingsGroup {
	type groupKey struct{ controller, kind, group string }
	type settingKey struct{ category, name string }
	var order []groupKey
	groups := map[groupKey]*DeliverySettingsGroup{}
	values := map[groupKey]map[settingKey]map[string]*DeliverySettingValue{}
	settingOrder := map[groupKey][]settingKey{}
	for _, deployer := range deployers {
		key := groupKey{deployer.Controller, deployer.Kind, ""}
		groupKind := DeliveryGroupAll
		if byGroup {
			key.group, groupKind = deployer.Group, deployer.GroupKind
		}
		group := groups[key]
		if group == nil {
			group = &DeliverySettingsGroup{Controller: key.controller, Kind: key.kind, GroupKind: groupKind, Group: key.group}
			groups[key] = group
			values[key] = map[settingKey]map[string]*DeliverySettingValue{}
			order = append(order, key)
		}
		group.Deployers++
		for _, setting := range deployer.Settings {
			sKey := settingKey{setting.Category, setting.Name}
			if values[key][sKey] == nil {
				values[key][sKey] = map[string]*DeliverySettingValue{}
				settingOrder[key] = append(settingOrder[key], sKey)
			}
			value := setting.Effective
			if value == "" {
				value = setting.Value
			}
			entry := values[key][sKey][value]
			if entry == nil {
				entry = &DeliverySettingValue{Value: value}
				values[key][sKey][value] = entry
			}
			// The same option written twice is still one deployer.
			if n := len(entry.Deployers); n > 0 && entry.Deployers[n-1].Namespace == deployer.Namespace && entry.Deployers[n-1].Name == deployer.Name {
				continue
			}
			ref := DeliveryDeployerRef{Namespace: deployer.Namespace, Name: deployer.Name, URL: deployer.URL, Detail: setting.Detail}
			if setting.Value == DeliveryValueUnset && value != DeliveryValueUnset {
				ref.Unset = true
				entry.Unset++
			}
			entry.Count++
			entry.Deployers = append(entry.Deployers, ref)
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.controller != b.controller {
			return a.controller < b.controller
		}
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return a.group < b.group
	})
	out := make([]DeliverySettingsGroup, 0, len(order))
	for _, key := range order {
		group := groups[key]
		keys := settingOrder[key]
		// Policy rows keep the order the kind declares them in; options
		// follow, sorted by name.
		sort.SliceStable(keys, func(i, j int) bool {
			if keys[i].category != keys[j].category {
				return keys[i].category == DeliverySettingPolicy
			}
			if keys[i].category == DeliverySettingPolicy {
				return false
			}
			return keys[i].name < keys[j].name
		})
		for _, sKey := range keys {
			summary := DeliverySettingSummary{Name: sKey.name, Category: sKey.category}
			for _, entry := range values[key][sKey] {
				sort.SliceStable(entry.Deployers, func(i, j int) bool {
					if entry.Deployers[i].Namespace != entry.Deployers[j].Namespace {
						return entry.Deployers[i].Namespace < entry.Deployers[j].Namespace
					}
					return entry.Deployers[i].Name < entry.Deployers[j].Name
				})
				summary.Values = append(summary.Values, *entry)
			}
			sort.SliceStable(summary.Values, func(i, j int) bool {
				a, b := summary.Values[i].Value, summary.Values[j].Value
				if deliveryValueRank(a) != deliveryValueRank(b) {
					return deliveryValueRank(a) < deliveryValueRank(b)
				}
				return a < b
			})
			group.Settings = append(group.Settings, summary)
		}
		out = append(out, *group)
	}
	return out
}

// DeliverySettingMatch selects deployers by one setting. With AnyValue it
// matches every deployer whose spec declares the setting, whatever the value.
type DeliverySettingMatch struct {
	Name     string
	Value    string
	AnyValue bool
}

func deliveryValuesEqual(a, b string) bool {
	normalise := func(value string) string {
		switch strings.ToLower(value) {
		case "on", "true":
			return "true"
		case "off", "false":
			return "false"
		}
		return value
	}
	return normalise(a) == normalise(b)
}

// Matches reports whether the deployer has the setting with the wanted value,
// declared or by the controller's default. Names are compared exactly: the
// Argo CD policy "prune" and the sync option "Prune" are different settings.
func (m DeliverySettingMatch) Matches(deployer DeliveryDeployerSettings) bool {
	for _, setting := range deployer.Settings {
		if setting.Name != m.Name {
			continue
		}
		if m.AnyValue {
			// Every Application has a self-heal row; only some declare it.
			// A row that is unset or does not apply is not a declaration.
			if setting.Value != DeliveryValueUnset && setting.Value != DeliveryValueNotApplicable {
				return true
			}
			continue
		}
		if deliveryValuesEqual(setting.Value, m.Value) ||
			(setting.Effective != "" && deliveryValuesEqual(setting.Effective, m.Value)) {
			return true
		}
	}
	return false
}
