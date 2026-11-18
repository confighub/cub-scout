package main

// doctor --before-attach (#888): what `cub cluster attach` would meet in this
// cluster, read before it runs. cub-scout only reads; it never attaches and
// never says a cluster is safe to attach.
//
// Every rule here rests on what attach was seen to do on a real cluster in
// the attach lane (scripts/ci/attach-lane.sh, cub v0.11.4, Argo CD v3.6.1 and
// Flux v2.10.0; recordings under test/e2e/fixtures/attach-lane/recorded), or,
// where marked "cub-source", on reading `cub cluster attach`'s source without
// running it. Each finding says which.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
)

// Bases: whose word an attach finding rests on.
const (
	attachBasisRecorded  = "recorded"   // seen in the attach lane
	attachBasisCubSource = "cub-source" // read from cub's source, not run
	attachBasisRead      = "read"       // about cub-scout's own read, not about attach
)

// errAttachKindNotServed is list's answer for a kind the cluster does not
// serve at the version read. It is neither "none" nor a failure: a cluster
// serving another version may hold objects of that kind.
var errAttachKindNotServed = errors.New("kind not served at this version")

// What attach would do with this controller, given what was read. These are
// conditional on the controller being the one attach is given (--controller).
const (
	attachWouldRefuse      = "refuse"       // earlier-attachment credentials are present
	attachWouldUseExisting = "use-existing" // the controller is installed; attach leaves it as found
	attachWouldInstall     = "install"      // not installed; attach installs it unless --no-install
	attachWouldUnknown     = "unknown"      // a read it depends on failed
)

// Read outcomes.
const (
	attachPresent = "present"
	attachAbsent  = "absent"
	attachUnread  = "unknown"
)

var (
	attachDeploymentGVR     = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	attachSecretGVR         = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	attachNamespaceGVR      = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	attachConfigMapGVR      = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	attachApplicationGVR    = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
	attachAppSetGVR         = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applicationsets"}
	attachKustomizationGVR  = schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
	attachOCIRepositoryGVR  = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "ocirepositories"}
	attachHelmReleaseGVR    = schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}
	attachClusterProfileGVR = schema.GroupVersionResource{Group: "config.projectsveltos.io", Version: "v1beta1", Resource: "clusterprofiles"}
)

// argocd-cm's key attach sets unless it is already set.
const attachArgoIgnoreDifferencesKey = "resource.customizations.ignoreDifferences.argoproj.io_Application"

// attachControllerSpec is what attach looks for, per controller.
type attachControllerSpec struct {
	Controller        string
	Namespace         string
	Deployment        string // attach's test for "installed"
	CredsSecret       string // an earlier attachment's registry credentials
	Basis             string
	FluxOrSveltosRoot schema.GroupVersionResource
}

var attachControllerSpecs = []attachControllerSpec{
	{Controller: "argo", Namespace: "argocd", Deployment: "argocd-server", CredsSecret: "confighub-oci-creds", Basis: attachBasisRecorded},
	{Controller: "flux", Namespace: "flux-system", Deployment: "kustomize-controller", CredsSecret: "confighub-oci", Basis: attachBasisRecorded, FluxOrSveltosRoot: attachKustomizationGVR},
	{Controller: "sveltos", Namespace: "projectsveltos", Deployment: "addon-controller", CredsSecret: "confighub-gateway", Basis: attachBasisRecorded, FluxOrSveltosRoot: attachClusterProfileGVR},
}

// The fixed name of the Flux and Sveltos roots attach applies.
const attachRootName = "confighub-root"

// DoctorAttachReport is doctor's --before-attach section.
type DoctorAttachReport struct {
	// Basis names what the rules rest on.
	Basis       string                   `json:"basis"`
	Controllers []DoctorAttachController `json:"controllers"`
	// CredentialsFor lists the controllers whose earlier-attachment
	// credentials Secret is in the cluster. That is not proof the attachment
	// is in use: a Secret can outlive it.
	CredentialsFor []string              `json:"credentialsFor"`
	Argobot        DoctorAttachArgobot   `json:"argobot"`
	Deployers      DoctorAttachDeployers `json:"deployers"`
	Unknowns       []string              `json:"unknowns"`
}

// DoctorAttachController is what `cub cluster attach --controller <c>` would
// meet.
type DoctorAttachController struct {
	Controller string `json:"controller"`
	// Installed is present, absent or unknown, from InstalledEvidence.
	Installed         string `json:"installed"`
	InstalledEvidence string `json:"installedEvidence"`
	// Credentials is the earlier-attachment Secret, read by name only.
	Credentials       string `json:"credentials"`
	CredentialsSecret string `json:"credentialsSecret"`
	// AttachWould is refuse, use-existing, install or unknown.
	AttachWould string `json:"attachWould"`
	Why         string `json:"why"`
	// Basis is recorded or cub-source.
	Basis string `json:"basis"`
	// EarlierObjects are objects of the kinds an attachment applies that are
	// here already: those pulling a source in ConfigHub's space layout, and the
	// Flux or Sveltos root by its fixed name. The registry is not confirmed as
	// ConfigHub's from the cluster alone.
	EarlierObjects []DoctorAttachObject `json:"earlierObjects,omitempty"`
	Notes          []DoctorAttachNote   `json:"notes,omitempty"`
}

// DoctorAttachObject names one object.
type DoctorAttachObject struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Source    string `json:"source,omitempty"`
}

// DoctorAttachNote is one more thing attach would meet, with its basis.
type DoctorAttachNote struct {
	Text  string `json:"text"`
	Basis string `json:"basis"`
}

// DoctorAttachArgobot is the bot attach installs unless --no-k8sbot.
type DoctorAttachArgobot struct {
	Namespace string `json:"namespace"`
	Secret    string `json:"secret"`
}

// DoctorAttachDeployers are the GitOps deployers already in the cluster.
type DoctorAttachDeployers struct {
	Counts map[string]int `json:"counts"`
	// Unread lists the kinds whose list failed.
	Unread []string `json:"unread,omitempty"`
	// NotServed lists the kinds not served at the version read; they are
	// not counted, since another version may hold some.
	NotServed []string `json:"notServed,omitempty"`
	Note      string   `json:"note"`
}

// attachReader is the reads the report needs. exists reads metadata only, so
// a Secret's data is never fetched.
type attachReader interface {
	exists(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (bool, error)
	get(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error)
	list(ctx context.Context, gvr schema.GroupVersionResource, namespace string) ([]unstructured.Unstructured, error)
}

type clusterAttachReader struct {
	meta metadata.Interface
	dyn  dynamic.Interface
}

func newClusterAttachReader(cfg *rest.Config) (*clusterAttachReader, error) {
	meta, err := metadata.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create metadata client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	return &clusterAttachReader{meta: meta, dyn: dyn}, nil
}

func (r *clusterAttachReader) exists(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (bool, error) {
	var err error
	if namespace == "" {
		_, err = r.meta.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	} else {
		_, err = r.meta.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (r *clusterAttachReader) get(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	var obj *unstructured.Unstructured
	var err error
	if namespace == "" {
		obj, err = r.dyn.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	} else {
		obj, err = r.dyn.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	}
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	return obj, err
}

func (r *clusterAttachReader) list(ctx context.Context, gvr schema.GroupVersionResource, namespace string) ([]unstructured.Unstructured, error) {
	var l *unstructured.UnstructuredList
	var err error
	if namespace == "" {
		l, err = r.dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
	} else {
		l, err = r.dyn.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	if apierrors.IsNotFound(err) {
		return nil, errAttachKindNotServed
	}
	if err != nil {
		return nil, err
	}
	return l.Items, nil
}

func collectDoctorAttachReport(ctx context.Context, cfg *rest.Config) (*DoctorAttachReport, error) {
	reader, err := newClusterAttachReader(cfg)
	if err != nil {
		return nil, err
	}
	return buildDoctorAttachReport(ctx, reader), nil
}

func attachReadOutcome(found bool, err error) string {
	switch {
	case err != nil:
		return attachUnread
	case found:
		return attachPresent
	default:
		return attachAbsent
	}
}

func buildDoctorAttachReport(ctx context.Context, r attachReader) *DoctorAttachReport {
	report := &DoctorAttachReport{
		Basis: "Read from this cluster, by name; Secrets by metadata only. What attach does with each finding was recorded " +
			"on a real cluster with cub v0.11.4, Argo CD v3.6.1, Flux v2.10.0 and Sveltos v1.16.0 as attach installed it (basis \"recorded\"), or read from cub's " +
			"source and not run (basis \"cub-source\").",
		CredentialsFor: []string{},
	}

	argoApps, argoAppsErr := r.list(ctx, attachApplicationGVR, "argocd")
	ociRepos, ociReposErr := r.list(ctx, attachOCIRepositoryGVR, "")

	for _, spec := range attachControllerSpecs {
		c := DoctorAttachController{
			Controller:        spec.Controller,
			InstalledEvidence: fmt.Sprintf("Deployment %s/%s", spec.Namespace, spec.Deployment),
			CredentialsSecret: fmt.Sprintf("%s/%s", spec.Namespace, spec.CredsSecret),
			Basis:             spec.Basis,
		}
		c.Installed = attachReadOutcome(r.exists(ctx, attachDeploymentGVR, spec.Namespace, spec.Deployment))
		c.Credentials = attachReadOutcome(r.exists(ctx, attachSecretGVR, spec.Namespace, spec.CredsSecret))
		if c.Credentials == attachPresent {
			report.CredentialsFor = append(report.CredentialsFor, spec.Controller)
		}

		switch {
		case c.Credentials == attachPresent:
			c.AttachWould = attachWouldRefuse
			c.Why = fmt.Sprintf("Secret %s holds an earlier attachment's registry credentials; attach refuses before it creates any Space in ConfigHub (its help says --existing-secrets skips this check)", c.CredentialsSecret)
		case c.Credentials == attachUnread:
			c.AttachWould = attachWouldUnknown
			c.Why = fmt.Sprintf("Secret %s could not be read, so whether an earlier attachment is here is unknown", c.CredentialsSecret)
		case c.Installed == attachPresent:
			c.AttachWould = attachWouldUseExisting
			c.Why = fmt.Sprintf("%s exists; attach leaves the installed controller as found", c.InstalledEvidence)
		case c.Installed == attachAbsent:
			c.AttachWould = attachWouldInstall
			c.Why = fmt.Sprintf("%s does not exist; attach installs the controller, or fails with --no-install", c.InstalledEvidence)
		default:
			c.AttachWould = attachWouldUnknown
			c.Why = fmt.Sprintf("%s could not be read, so whether the controller is installed is unknown", c.InstalledEvidence)
		}

		switch spec.Controller {
		case "argo":
			c.EarlierObjects = spaceLayoutSourcesOf(argoApps, "Application", "spec", "source", "repoURL")
			c.Notes = appendListNote(c.Notes, argoAppsErr, c.Installed, "Applications (argoproj.io/v1alpha1) in argocd")
			if c.Installed == attachPresent {
				c.Notes = append(c.Notes, argoConfigMapNote(ctx, r))
			}
		case "flux":
			c.EarlierObjects = spaceLayoutSourcesOf(ociRepos, "OCIRepository", "spec", "url")
			c.Notes = appendListNote(c.Notes, ociReposErr, c.Installed, "OCIRepositories (source.toolkit.fluxcd.io/v1)")
			c.EarlierObjects, c.Notes = fixedRootOf(ctx, r, spec, c.EarlierObjects, c.Notes)
		default:
			c.EarlierObjects, c.Notes = fixedRootOf(ctx, r, spec, c.EarlierObjects, c.Notes)
		}
		if len(c.EarlierObjects) > 0 && c.Credentials == attachAbsent {
			c.Notes = append(c.Notes, DoctorAttachNote{
				Text:  "objects of the kinds an attachment applies are here without its credentials Secret; what attach does with them was not recorded",
				Basis: spec.Basis,
			})
		}
		report.Controllers = append(report.Controllers, c)
	}

	report.Argobot = DoctorAttachArgobot{
		Namespace: attachReadOutcome(r.exists(ctx, attachNamespaceGVR, "", "argobot")),
		Secret:    attachReadOutcome(r.exists(ctx, attachSecretGVR, "argobot", "argobot-secrets")),
	}

	report.Deployers = DoctorAttachDeployers{
		Counts: map[string]int{},
		Note: "Attach leaves these as it finds them: in the recordings, with --prune, an Application and a Kustomization " +
			"that were here before were unchanged after attach. --prune governs what ConfigHub deploys later, not these.",
	}
	for _, d := range []struct {
		label string
		gvr   schema.GroupVersionResource
	}{
		{"Application", attachApplicationGVR},
		{"ApplicationSet", attachAppSetGVR},
		{"Kustomization", attachKustomizationGVR},
		{"HelmRelease", attachHelmReleaseGVR},
		{"ClusterProfile", attachClusterProfileGVR},
	} {
		items, err := r.list(ctx, d.gvr, "")
		if errors.Is(err, errAttachKindNotServed) {
			report.Deployers.NotServed = append(report.Deployers.NotServed, d.label)
			continue
		}
		if err != nil {
			report.Deployers.Unread = append(report.Deployers.Unread, d.label)
			continue
		}
		report.Deployers.Counts[d.label] = len(items)
	}
	sort.Strings(report.Deployers.Unread)
	sort.Strings(report.Deployers.NotServed)

	report.Unknowns = []string{
		"whether ConfigHub already has Spaces of the names attach would create; that is in ConfigHub, not the cluster",
		"whether the cluster can reach ConfigHub's registry; attach checks this itself",
		"whether these credentials have cluster-admin; attach checks this itself (its source runs `kubectl auth can-i create clusterroles`), and cub-scout does not",
		"whether what you later deploy through ConfigHub overlaps these deployers' objects",
	}
	return report
}

// spaceLayoutSourcesOf lists the objects whose source is in ConfigHub's
// space layout (oci://<registry>/space/<slug>), as every Application,
// OCIRepository and root an attachment applies is (recorded). The layout is
// matched; the registry is not confirmed as ConfigHub's.
func spaceLayoutSourcesOf(items []unstructured.Unstructured, kind string, path ...string) []DoctorAttachObject {
	var out []DoctorAttachObject
	for _, item := range items {
		source, _, _ := unstructured.NestedString(item.Object, path...)
		if !strings.HasPrefix(source, "oci://") {
			continue
		}
		if info := agent.ParseOCISource(source); info.Layout != agent.OCILayoutSpace {
			continue
		}
		out = append(out, DoctorAttachObject{Kind: kind, Namespace: item.GetNamespace(), Name: item.GetName(), Source: source})
	}
	sortAttachObjects(out)
	return out
}

func sortAttachObjects(objects []DoctorAttachObject) {
	sort.Slice(objects, func(i, j int) bool {
		a, b := objects[i], objects[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
}

// fixedRootOf adds the Flux Kustomization or Sveltos ClusterProfile attach
// names confighub-root, when it is here.
func fixedRootOf(ctx context.Context, r attachReader, spec attachControllerSpec, objects []DoctorAttachObject, notes []DoctorAttachNote) ([]DoctorAttachObject, []DoctorAttachNote) {
	namespace := spec.Namespace
	kind := "Kustomization"
	if spec.Controller == "sveltos" {
		namespace, kind = "", "ClusterProfile" // cluster-scoped
	}
	obj, err := r.get(ctx, spec.FluxOrSveltosRoot, namespace, attachRootName)
	if err != nil {
		notes = append(notes, DoctorAttachNote{Text: fmt.Sprintf("%s %s could not be read, so an earlier root is unknown", kind, attachRootName), Basis: attachBasisRead})
		return objects, notes
	}
	if obj != nil {
		objects = append(objects, DoctorAttachObject{Kind: kind, Namespace: namespace, Name: attachRootName})
		sortAttachObjects(objects)
	}
	return objects, notes
}

// appendListNote notes a list the earlier-object check depends on that was
// not read. A kind not served is expected when its controller is absent, and
// is then not noted.
func appendListNote(notes []DoctorAttachNote, err error, installed, what string) []DoctorAttachNote {
	switch {
	case err == nil:
		return notes
	case errors.Is(err, errAttachKindNotServed):
		if installed == attachAbsent {
			return notes
		}
		return append(notes, DoctorAttachNote{Text: what + " are not served at that version here, so earlier objects were not read", Basis: attachBasisRead})
	default:
		return append(notes, DoctorAttachNote{Text: what + " could not be listed, so earlier objects are unknown", Basis: attachBasisRead})
	}
}

func argoConfigMapNote(ctx context.Context, r attachReader) DoctorAttachNote {
	cm, err := r.get(ctx, attachConfigMapGVR, "argocd", "argocd-cm")
	if err != nil {
		return DoctorAttachNote{Text: "argocd-cm could not be read, so whether attach patches it is unknown", Basis: attachBasisRead}
	}
	if cm == nil {
		return DoctorAttachNote{Text: "argocd-cm is not here; attach patches it, and what it does when the ConfigMap is missing was not recorded", Basis: attachBasisCubSource}
	}
	value, _, _ := unstructured.NestedString(cm.Object, "data", attachArgoIgnoreDifferencesKey)
	if value == "" {
		return DoctorAttachNote{Text: "argocd-cm does not set " + attachArgoIgnoreDifferencesKey + "; attach patches argocd-cm to set it", Basis: attachBasisRecorded}
	}
	if strings.Contains(value, "confighub.com/") {
		// As an earlier attach leaves it (recorded).
		return DoctorAttachNote{Text: "argocd-cm already sets " + attachArgoIgnoreDifferencesKey + ", naming ConfigHub's annotations; attach leaves it as is", Basis: attachBasisRecorded}
	}
	return DoctorAttachNote{Text: "argocd-cm already sets " + attachArgoIgnoreDifferencesKey + " without ConfigHub's annotations; attach leaves it as is, and the root Application may show OutOfSync", Basis: attachBasisCubSource}
}

func renderDoctorAttachASCII(b *strings.Builder, report *DoctorAttachReport) {
	if report == nil {
		return
	}
	b.WriteString("\nBEFORE CUB CLUSTER ATTACH\n")
	b.WriteString("  Read only; cub-scout does not say whether attaching is safe.\n")
	if len(report.CredentialsFor) > 0 {
		fmt.Fprintf(b, "  An earlier attachment's credentials Secret is here for: %s\n", strings.Join(report.CredentialsFor, ", "))
	}
	for _, c := range report.Controllers {
		fmt.Fprintf(b, "  --controller %-8s %s: %s [%s]\n", c.Controller, c.AttachWould, c.Why, c.Basis)
		for _, obj := range c.EarlierObjects {
			where := obj.Name
			if obj.Namespace != "" {
				where = obj.Namespace + "/" + obj.Name
			}
			if obj.Source != "" {
				fmt.Fprintf(b, "      already here: %s %s (%s)\n", obj.Kind, where, obj.Source)
			} else {
				fmt.Fprintf(b, "      already here: %s %s\n", obj.Kind, where)
			}
		}
		for _, note := range c.Notes {
			fmt.Fprintf(b, "      %s [%s]\n", note.Text, note.Basis)
		}
	}
	fmt.Fprintf(b, "  argobot: namespace %s, Secret argobot/argobot-secrets %s\n", report.Argobot.Namespace, report.Argobot.Secret)
	labels := make([]string, 0, len(report.Deployers.Counts))
	for label := range report.Deployers.Counts {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	parts := make([]string, 0, len(labels))
	for _, label := range labels {
		parts = append(parts, fmt.Sprintf("%s %d", label, report.Deployers.Counts[label]))
	}
	fmt.Fprintf(b, "  Deployers here: %s\n", strings.Join(parts, ", "))
	if len(report.Deployers.NotServed) > 0 {
		fmt.Fprintf(b, "  Not served at the version read, so not counted: %s\n", strings.Join(report.Deployers.NotServed, ", "))
	}
	if len(report.Deployers.Unread) > 0 {
		fmt.Fprintf(b, "  Not read: %s\n", strings.Join(report.Deployers.Unread, ", "))
	}
	fmt.Fprintf(b, "  %s\n", report.Deployers.Note)
	b.WriteString("  Unknown:\n")
	for _, u := range report.Unknowns {
		fmt.Fprintf(b, "    - %s\n", u)
	}
}
