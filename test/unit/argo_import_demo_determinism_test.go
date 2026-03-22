package unit

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestArgoImportDemoScript_PinsArgoInstallVersion(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "demo.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	script := string(content)

	if strings.Contains(script, "argoproj/argo-cd/stable/manifests/install.yaml") {
		t.Fatalf("demo still installs ArgoCD from floating stable manifest")
	}

	versionRe := regexp.MustCompile(`ARGOCD_VERSION="v\d+\.\d+\.\d+"`)
	if !versionRe.MatchString(script) {
		t.Fatalf("expected ARGOCD_VERSION to be pinned to a release tag")
	}

	if !strings.Contains(script, "argoproj/argo-cd/${ARGOCD_VERSION}/manifests/install.yaml") {
		t.Fatalf("expected pinned ArgoCD install URL to use ${ARGOCD_VERSION}")
	}
}

func TestArgoImportGuestbookFixtures_PinTargetRevision(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "fixtures", "guestbook-apps.yaml")
	content, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	fixture := string(content)

	if strings.Contains(fixture, "targetRevision: HEAD") {
		t.Fatalf("guestbook fixture still uses floating targetRevision: HEAD")
	}

	shaRe := regexp.MustCompile(`targetRevision:\s*[a-f0-9]{40}`)
	matches := shaRe.FindAllString(fixture, -1)
	if len(matches) < 2 {
		t.Fatalf("expected both guestbook apps to pin targetRevision to a commit SHA")
	}
}

func TestArgoImportDemoScript_WaitsForSyncedHealthyGuestbookApps(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "demo.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	script := string(content)

	if strings.Contains(script, "DEPLOY_COUNT=") {
		t.Fatalf("demo readiness still uses deployment-count gate")
	}

	required := []string{
		"guestbook_app_ready()",
		".status.sync.status",
		".status.health.status",
		"guestbook_app_ready helm-guestbook && guestbook_app_ready kustomize-guestbook",
	}
	for _, needle := range required {
		if !strings.Contains(script, needle) {
			t.Fatalf("expected Synced+Healthy readiness check to include %q", needle)
		}
	}
}

func TestArgoImportDemoScript_UsesHTTPSForArgoCDSessionToken(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "demo.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	script := string(content)

	required := []string{
		"port-forward svc/argocd-server 8888:443",
		"curl -sk -H \"Content-Type: application/json\" \"https://localhost:8888/api/v1/session\"",
	}
	for _, needle := range required {
		if !strings.Contains(script, needle) {
			t.Fatalf("expected ArgoCD token flow to include %q", needle)
		}
	}

	if strings.Contains(script, "\"http://localhost:8888/api/v1/session\"") {
		t.Fatalf("ArgoCD token flow still uses plain HTTP session endpoint")
	}
}
