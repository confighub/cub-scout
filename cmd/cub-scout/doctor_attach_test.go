package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"
)

// recordedAttachReader answers the report's reads from one snapshot the
// attach lane recorded on a real cluster (test/e2e/fixtures/attach-lane).
type recordedAttachReader struct {
	exist     map[string]bool // gvr.Resource/namespace/name
	objects   map[string]*unstructured.Unstructured
	lists     map[schema.GroupVersionResource][]unstructured.Unstructured
	forbidden map[schema.GroupVersionResource]bool
	notServed map[schema.GroupVersionResource]bool
	missing   map[string]bool // attachKey of an object get must not find
}

func attachKey(gvr schema.GroupVersionResource, namespace, name string) string {
	return gvr.Resource + "/" + namespace + "/" + name
}

func (r *recordedAttachReader) exists(_ context.Context, gvr schema.GroupVersionResource, namespace, name string) (bool, error) {
	if r.forbidden[gvr] {
		return false, apierrors.NewForbidden(gvr.GroupResource(), name, fmt.Errorf("recorded as forbidden"))
	}
	return r.exist[attachKey(gvr, namespace, name)], nil
}

func (r *recordedAttachReader) get(_ context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if r.forbidden[gvr] {
		return nil, apierrors.NewForbidden(gvr.GroupResource(), name, fmt.Errorf("recorded as forbidden"))
	}
	if r.missing[attachKey(gvr, namespace, name)] {
		return nil, nil
	}
	if obj, ok := r.objects[attachKey(gvr, namespace, name)]; ok {
		return obj, nil
	}
	for _, item := range r.lists[gvr] {
		if item.GetNamespace() == namespace && item.GetName() == name {
			copy := item
			return &copy, nil
		}
	}
	return nil, nil
}

func (r *recordedAttachReader) list(_ context.Context, gvr schema.GroupVersionResource, namespace string) ([]unstructured.Unstructured, error) {
	if r.forbidden[gvr] {
		return nil, apierrors.NewForbidden(gvr.GroupResource(), "", fmt.Errorf("recorded as forbidden"))
	}
	if r.notServed[gvr] {
		return nil, errAttachKindNotServed
	}
	var out []unstructured.Unstructured
	for _, item := range r.lists[gvr] {
		if namespace == "" || item.GetNamespace() == namespace {
			out = append(out, item)
		}
	}
	return out, nil
}

func loadRecordedAttachSnapshot(t *testing.T, controller, when string) *recordedAttachReader {
	t.Helper()
	dir := filepath.Join("..", "..", "test", "e2e", "fixtures", "attach-lane", "recorded", controller, when)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("TEMPORARY: no recording at %s yet", dir)
	}
	r := &recordedAttachReader{
		exist:     map[string]bool{},
		objects:   map[string]*unstructured.Unstructured{},
		lists:     map[schema.GroupVersionResource][]unstructured.Unstructured{},
		forbidden: map[schema.GroupVersionResource]bool{},
		notServed: map[schema.GroupVersionResource]bool{},
		missing:   map[string]bool{},
	}
	columns := func(name string, fn func(fields []string)) {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Scan() // header
		for scanner.Scan() {
			if fields := strings.Fields(scanner.Text()); len(fields) > 0 {
				fn(fields)
			}
		}
	}
	columns("secrets-by-name.txt", func(fields []string) {
		r.exist[attachKey(attachSecretGVR, fields[0], fields[1])] = true
	})
	columns("workloads.txt", func(fields []string) {
		if fields[0] == "Deployment" {
			r.exist[attachKey(attachDeploymentGVR, fields[1], fields[2])] = true
		}
	})
	raw, err := os.ReadFile(filepath.Join(dir, "namespaces.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Fields(string(raw)) {
		r.exist[attachKey(attachNamespaceGVR, "", strings.TrimPrefix(line, "namespace/"))] = true
	}
	var cm map[string]interface{}
	readJSON(t, filepath.Join(dir, "argocd-cm.json"), &cm)
	data := map[string]interface{}{}
	for _, key := range cm["keys"].([]interface{}) {
		data[key.(string)] = "recorded"
	}
	if value, ok := cm[attachArgoIgnoreDifferencesKey].(string); ok {
		data[attachArgoIgnoreDifferencesKey] = value
	}
	r.objects[attachKey(attachConfigMapGVR, "argocd", "argocd-cm")] = &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]interface{}{"namespace": "argocd", "name": "argocd-cm"},
		"data":     data,
	}}
	// ClusterProfiles are recorded only where Sveltos was installed; before
	// that the lane recorded the kind as not served.
	if _, err := os.Stat(filepath.Join(dir, "clusterprofiles.not-served")); err == nil {
		r.notServed[attachClusterProfileGVR] = true
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "clusterprofiles.json")); err == nil {
		var items []map[string]interface{}
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			r.lists[attachClusterProfileGVR] = append(r.lists[attachClusterProfileGVR], unstructured.Unstructured{Object: item})
		}
	}
	for file, gvr := range map[string]schema.GroupVersionResource{
		"applications.json":    attachApplicationGVR,
		"applicationsets.json": attachAppSetGVR,
		"kustomizations.json":  attachKustomizationGVR,
		"ocirepositories.json": attachOCIRepositoryGVR,
		"helmreleases.json":    attachHelmReleaseGVR,
	} {
		var items []map[string]interface{}
		readJSON(t, filepath.Join(dir, file), &items)
		for _, item := range items {
			r.lists[gvr] = append(r.lists[gvr], unstructured.Unstructured{Object: item})
		}
	}
	return r
}

func readJSON(t *testing.T, path string, into interface{}) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func attachControllerOf(t *testing.T, report *DoctorAttachReport, controller string) DoctorAttachController {
	t.Helper()
	for _, c := range report.Controllers {
		if c.Controller == controller {
			return c
		}
	}
	t.Fatalf("no %s in report", controller)
	return DoctorAttachController{}
}

func attachObjectNames(objects []DoctorAttachObject) []string {
	var out []string
	for _, o := range objects {
		out = append(out, o.Kind+" "+o.Namespace+"/"+o.Name)
	}
	return out
}

// Before attach the recorded cluster runs Argo CD and Flux with an app each,
// no Sveltos, and nothing from ConfigHub. Attach was then seen to leave Argo
// CD or Flux as found, to patch argocd-cm, which did not set the key, and to
// install Sveltos.
func TestBeforeAttachOnTheRecordedBrownfieldCluster(t *testing.T) {
	for _, controller := range []string{"argo", "flux", "sveltos"} {
		t.Run(controller, func(t *testing.T) {
			report := buildDoctorAttachReport(context.Background(), loadRecordedAttachSnapshot(t, controller, "before"))
			if len(report.CredentialsFor) != 0 {
				t.Fatalf("credentialsFor = %v before any attach", report.CredentialsFor)
			}
			for want, c := range map[string]string{"argo": attachWouldUseExisting, "flux": attachWouldUseExisting, "sveltos": attachWouldInstall} {
				got := attachControllerOf(t, report, want)
				if got.AttachWould != c {
					t.Errorf("%s: attachWould = %q (%s), want %q", want, got.AttachWould, got.Why, c)
				}
				if len(got.EarlierObjects) != 0 {
					t.Errorf("%s: earlier objects %v before any attach", want, attachObjectNames(got.EarlierObjects))
				}
			}
			argo := attachControllerOf(t, report, "argo")
			if len(argo.Notes) != 1 || !strings.Contains(argo.Notes[0].Text, "attach patches argocd-cm") || argo.Notes[0].Basis != attachBasisRecorded {
				t.Errorf("argo notes = %+v, want the recorded argocd-cm patch", argo.Notes)
			}
			for _, c := range report.Controllers {
				if c.Basis != attachBasisRecorded {
					t.Errorf("%s: basis %q; all three were recorded", c.Controller, c.Basis)
				}
			}
			if len(report.Deployers.NotServed) != 1 || report.Deployers.NotServed[0] != "ClusterProfile" {
				t.Errorf("notServed = %v; before Sveltos is installed, ClusterProfile is not served", report.Deployers.NotServed)
			}
			if report.Argobot.Namespace != attachAbsent || report.Argobot.Secret != attachAbsent {
				t.Errorf("argobot = %+v before attach", report.Argobot)
			}
			if report.Deployers.Counts["Application"] != 1 || report.Deployers.Counts["Kustomization"] != 1 {
				t.Errorf("deployers = %v, want the recorded guestbook Application and podinfo Kustomization", report.Deployers.Counts)
			}
			if len(report.Unknowns) == 0 {
				t.Error("the report must list what it cannot know")
			}
		})
	}
}

// After a real attach, a second attach through the same controller was
// refused, naming the credentials Secret (recorded). The report must say so,
// and list what the first attach applied.
func TestBeforeAttachAfterARecordedAttach(t *testing.T) {
	cases := map[string]struct {
		refused []string
		objects []string
	}{
		"argo": {
			refused: []string{"argo"},
			objects: []string{"Application argocd/argobot-scout-attach", "Application argocd/scout-attach-argo-apps"},
		},
		"flux": {
			refused: []string{"flux"},
			objects: []string{"Kustomization flux-system/confighub-root", "OCIRepository flux-system/argobot-scout-attach", "OCIRepository flux-system/confighub-root"},
		},
		"sveltos": {
			refused: []string{"sveltos"},
			objects: []string{"ClusterProfile /confighub-root"},
		},
	}
	for controller, want := range cases {
		t.Run(controller, func(t *testing.T) {
			report := buildDoctorAttachReport(context.Background(), loadRecordedAttachSnapshot(t, controller, "after"))
			if strings.Join(report.CredentialsFor, ",") != strings.Join(want.refused, ",") {
				t.Fatalf("credentialsFor = %v, want %v", report.CredentialsFor, want.refused)
			}
			c := attachControllerOf(t, report, controller)
			if c.AttachWould != attachWouldRefuse || !strings.Contains(c.Why, c.CredentialsSecret) {
				t.Errorf("attachWould = %q (%s), want refuse naming %s", c.AttachWould, c.Why, c.CredentialsSecret)
			}
			if got := strings.Join(attachObjectNames(c.EarlierObjects), "; "); got != strings.Join(want.objects, "; ") {
				t.Errorf("earlier objects = %s\nwant %s", got, strings.Join(want.objects, "; "))
			}
			if report.Argobot.Namespace != attachPresent || report.Argobot.Secret != attachPresent {
				t.Errorf("argobot = %+v after attach", report.Argobot)
			}
			if controller == "argo" {
				notes := fmt.Sprint(c.Notes)
				if !strings.Contains(notes, "naming ConfigHub's annotations") || strings.Contains(notes, "OutOfSync") {
					t.Errorf("argocd-cm as attach left it must not warn of OutOfSync: %s", notes)
				}
			}
			// The recorded refusal is attach's own word for this.
			log, err := os.ReadFile(filepath.Join("..", "..", "test", "e2e", "fixtures", "attach-lane", "recorded", controller, "attach-again.log"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(log), "already wired to a ConfigHub") || !strings.Contains(string(log), "Secret "+c.CredentialsSecret) {
				t.Errorf("recorded refusal does not name %s", c.CredentialsSecret)
			}
		})
	}
}

// A read that fails is unknown, never absent: a forbidden Secret read must
// not turn into "install" or "use-existing".
func TestBeforeAttachUnreadableSecretsAreUnknown(t *testing.T) {
	r := loadRecordedAttachSnapshot(t, "argo", "after")
	r.forbidden[attachSecretGVR] = true
	report := buildDoctorAttachReport(context.Background(), r)
	for _, c := range report.Controllers {
		if c.AttachWould != attachWouldUnknown || c.Credentials != attachUnread {
			t.Errorf("%s: attachWould = %q, credentials = %q with Secrets unreadable", c.Controller, c.AttachWould, c.Credentials)
		}
	}
	if len(report.CredentialsFor) != 0 {
		t.Errorf("credentialsFor = %v from unreadable Secrets", report.CredentialsFor)
	}
	if report.Argobot.Secret != attachUnread {
		t.Errorf("argobot secret = %q with Secrets unreadable", report.Argobot.Secret)
	}

	r = loadRecordedAttachSnapshot(t, "flux", "before")
	r.forbidden[attachKustomizationGVR] = true
	report = buildDoctorAttachReport(context.Background(), r)
	if got := report.Deployers.Unread; len(got) != 1 || got[0] != "Kustomization" {
		t.Errorf("unread deployers = %v, want Kustomization", got)
	}
	if _, counted := report.Deployers.Counts["Kustomization"]; counted {
		t.Error("an unreadable kind must not be counted as zero")
	}
}

func TestBeforeAttachASCIISaysWhatItCannotKnow(t *testing.T) {
	report := buildDoctorAttachReport(context.Background(), loadRecordedAttachSnapshot(t, "argo", "after"))
	var b strings.Builder
	renderDoctorAttachASCII(&b, report)
	out := b.String()
	for _, want := range []string{
		"BEFORE CUB CLUSTER ATTACH",
		"does not say whether attaching is safe",
		"An earlier attachment's credentials Secret is here for: argo",
		"--controller argo     refuse",
		"already here: Application argocd/scout-attach-argo-apps (oci://172.18.0.1:32181/space/scout-attach-argo-apps)",
		"Unknown:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ASCII lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "safe to attach") {
		t.Errorf("ASCII must never call a cluster safe to attach:\n%s", out)
	}
}

func TestDoctorMCPPassesBeforeAttach(t *testing.T) {
	tool, ok := newMCPGateway(nil).tools["doctor"]
	if !ok {
		t.Fatal("no doctor tool")
	}
	args, err := tool.BuildArgs(map[string]interface{}{"before_attach": true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "doctor --format json --before-attach" {
		t.Errorf("args = %v", args)
	}
}

func attachNotesOf(c DoctorAttachController) string {
	var parts []string
	for _, n := range c.Notes {
		parts = append(parts, n.Text+" ["+n.Basis+"]")
	}
	return strings.Join(parts, " | ")
}

// The reads the earlier-object check and the argocd-cm note depend on: a
// failure is noted as cub-scout's own read, never passed off as attach's.
func TestBeforeAttachFailedReadsAreNotedAsReads(t *testing.T) {
	cases := []struct {
		name       string
		controller string
		mutate     func(r *recordedAttachReader)
		want       string
	}{
		{"applications forbidden", "argo", func(r *recordedAttachReader) { r.forbidden[attachApplicationGVR] = true },
			"Applications (argoproj.io/v1alpha1) in argocd could not be listed, so earlier objects are unknown [read]"},
		{"ocirepositories forbidden", "flux", func(r *recordedAttachReader) { r.forbidden[attachOCIRepositoryGVR] = true },
			"OCIRepositories (source.toolkit.fluxcd.io/v1) could not be listed, so earlier objects are unknown [read]"},
		{"ocirepositories not served", "flux", func(r *recordedAttachReader) { r.notServed[attachOCIRepositoryGVR] = true },
			"OCIRepositories (source.toolkit.fluxcd.io/v1) are not served at that version here, so earlier objects were not read [read]"},
		{"flux root forbidden", "flux", func(r *recordedAttachReader) { r.forbidden[attachKustomizationGVR] = true },
			"Kustomization confighub-root could not be read, so an earlier root is unknown [read]"},
		{"argocd-cm forbidden", "argo", func(r *recordedAttachReader) { r.forbidden[attachConfigMapGVR] = true },
			"argocd-cm could not be read, so whether attach patches it is unknown [read]"},
		{"argocd-cm missing", "argo", func(r *recordedAttachReader) { r.missing[attachKey(attachConfigMapGVR, "argocd", "argocd-cm")] = true },
			"argocd-cm is not here; attach patches it, and what it does when the ConfigMap is missing was not recorded [cub-source]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := loadRecordedAttachSnapshot(t, tc.controller, "after")
			tc.mutate(r)
			c := attachControllerOf(t, buildDoctorAttachReport(context.Background(), r), tc.controller)
			if notes := attachNotesOf(c); !strings.Contains(notes, tc.want) {
				t.Errorf("notes = %s\nwant %s", notes, tc.want)
			}
		})
	}
}

// A kind not served at the version read is not counted as zero: a cluster
// serving only another version may hold objects of it.
func TestBeforeAttachNotServedKindIsNotCounted(t *testing.T) {
	r := loadRecordedAttachSnapshot(t, "flux", "after")
	r.notServed[attachHelmReleaseGVR] = true
	report := buildDoctorAttachReport(context.Background(), r)
	if _, counted := report.Deployers.Counts["HelmRelease"]; counted {
		t.Error("a kind not served must not be counted")
	}
	if strings.Join(report.Deployers.NotServed, ",") != "ClusterProfile,HelmRelease" {
		t.Errorf("notServed = %v", report.Deployers.NotServed)
	}
	// Sveltos is not installed here, so its ClusterProfile is not served
	// either, and that needs no note.
	if notes := attachNotesOf(attachControllerOf(t, report, "sveltos")); strings.Contains(notes, "not served") {
		t.Errorf("sveltos notes = %s", notes)
	}
}

// The real reader reads Secrets through the metadata client, so their data
// is never fetched, and maps NotFound to absent and an unserved kind to
// errAttachKindNotServed.
func TestClusterAttachReaderReadsSecretsAsMetadataOnly(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := metav1.AddMetaToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	secretMeta := &metav1.PartialObjectMetadata{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "argocd", Name: "confighub-oci-creds"},
	}
	meta := metadatafake.NewSimpleMetadataClient(scheme, secretMeta)
	// The dynamic client holds no Secret at all: a reader that fetched
	// Secrets through it would find none.
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		attachApplicationGVR: "ApplicationList",
		attachHelmReleaseGVR: "HelmReleaseList",
	})
	r := &clusterAttachReader{meta: meta, dyn: dyn}
	ctx := context.Background()

	found, err := r.exists(ctx, attachSecretGVR, "argocd", "confighub-oci-creds")
	if err != nil || !found {
		t.Fatalf("exists = %v, %v; want the Secret found through metadata", found, err)
	}
	found, err = r.exists(ctx, attachSecretGVR, "flux-system", "confighub-oci")
	if err != nil || found {
		t.Fatalf("exists = %v, %v; want absent with no error", found, err)
	}
	if obj, err := r.get(ctx, attachConfigMapGVR, "argocd", "argocd-cm"); obj != nil || err != nil {
		t.Fatalf("get of a missing object = %v, %v; want nil, nil", obj, err)
	}
	if items, err := r.list(ctx, attachApplicationGVR, "argocd"); err != nil || len(items) != 0 {
		t.Fatalf("list of a served, empty kind = %v, %v", items, err)
	}

	notServed := &clusterAttachReader{meta: meta, dyn: dyn}
	notServed.dyn.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "helmreleases", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(attachHelmReleaseGVR.GroupResource(), "")
	})
	if _, err := notServed.list(ctx, attachHelmReleaseGVR, ""); !errors.Is(err, errAttachKindNotServed) {
		t.Fatalf("list of an unserved kind = %v; want errAttachKindNotServed", err)
	}
}

// With fixture input there is no cluster to read: the section is missing
// and a warning says why, never silently.
func TestBeforeAttachWithFixtureInputWarns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doctor.json")
	if err := os.WriteFile(path, []byte(`{"cluster":"fixture","entries":[],"findings":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ObserveScopeSummary(context.Background(), ObserveScopeSummaryRequest{FixturePath: path, BeforeAttach: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Attach != nil || !strings.Contains(strings.Join(result.Warnings, "|"), "before-attach report unavailable") {
		t.Errorf("attach = %v, warnings = %v", result.Summary.Attach, result.Warnings)
	}
}
