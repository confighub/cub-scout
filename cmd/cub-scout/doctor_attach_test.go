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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// recordedAttachReader answers the report's reads from one snapshot the
// attach lane recorded on a real cluster (test/e2e/fixtures/attach-lane).
type recordedAttachReader struct {
	exist     map[string]bool // gvr.Resource/namespace/name
	objects   map[string]*unstructured.Unstructured
	lists     map[schema.GroupVersionResource][]unstructured.Unstructured
	forbidden map[schema.GroupVersionResource]bool
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
	r := &recordedAttachReader{
		exist:     map[string]bool{},
		objects:   map[string]*unstructured.Unstructured{},
		lists:     map[schema.GroupVersionResource][]unstructured.Unstructured{},
		forbidden: map[schema.GroupVersionResource]bool{},
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
// and nothing from ConfigHub. Attach was then seen to leave either controller
// as found and to patch argocd-cm, which did not set the key.
func TestBeforeAttachOnTheRecordedBrownfieldCluster(t *testing.T) {
	for _, controller := range []string{"argo", "flux"} {
		t.Run(controller, func(t *testing.T) {
			report := buildDoctorAttachReport(context.Background(), loadRecordedAttachSnapshot(t, controller, "before"))
			if len(report.WiredThrough) != 0 {
				t.Fatalf("wiredThrough = %v before any attach", report.WiredThrough)
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
			if attachControllerOf(t, report, "sveltos").Basis != attachBasisCubSource {
				t.Error("Sveltos was not recorded; its basis must say cub-source")
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
	}
	for controller, want := range cases {
		t.Run(controller, func(t *testing.T) {
			report := buildDoctorAttachReport(context.Background(), loadRecordedAttachSnapshot(t, controller, "after"))
			if strings.Join(report.WiredThrough, ",") != strings.Join(want.refused, ",") {
				t.Fatalf("wiredThrough = %v, want %v", report.WiredThrough, want.refused)
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
	if len(report.WiredThrough) != 0 {
		t.Errorf("wiredThrough = %v from unreadable Secrets", report.WiredThrough)
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
		"Already attached to a ConfigHub through: argo",
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
