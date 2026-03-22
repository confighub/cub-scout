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

func TestArgoImportDemoScript_UsesKubernetesProxyForArgoCDSessionToken(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "demo.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	script := string(content)

	required := []string{
		`kubectl proxy --port "$ARGOCD_PROXY_PORT"`,
		`ARGOCD_SESSION_PROXY_URL="http://127.0.0.1:${ARGOCD_PROXY_PORT}/api/v1/namespaces/argocd/services/https:argocd-server:https/proxy/api/v1/session"`,
		`curl -sS --max-time 5 -H "Content-Type: application/json" "$ARGOCD_SESSION_PROXY_URL"`,
	}
	for _, needle := range required {
		if !strings.Contains(script, needle) {
			t.Fatalf("expected ArgoCD token flow to include %q", needle)
		}
	}

	if strings.Contains(script, "port-forward svc/argocd-server 8888:443") {
		t.Fatalf("ArgoCD token flow still relies on service port-forward")
	}
}

func TestArgoImportDemoScript_RetriesTokenPortForwardUntilServerIsReachable(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "demo.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	script := string(content)

	required := []string{
		`ARGOCD_TOKEN_MAX_ATTEMPTS=30`,
		`ARGOCD_PROXY_PORT=8001`,
		`for i in $(seq 1 "$ARGOCD_TOKEN_MAX_ATTEMPTS")`,
		`LAST_ARGOCD_SERVER_STATE="$(kubectl -n argocd get pod -l app.kubernetes.io/name=argocd-server`,
		`LAST_ARGOCD_SESSION_STATUS="$(printf '%s' "$RESPONSE"`,
		`kubectl proxy --port "$ARGOCD_PROXY_PORT" >"$ARGOCD_PROXY_LOG" 2>&1 &`,
		`curl -sS --max-time 5 -H "Content-Type: application/json" "$ARGOCD_SESSION_PROXY_URL"`,
		`printf "."`,
		`sleep "$ARGOCD_TOKEN_RETRY_SECONDS"`,
		`Could not get ArgoCD token after ${ARGOCD_TOKEN_MAX_ATTEMPTS} attempts`,
		`Last proxy status: $LAST_PROXY_STATUS`,
		`Last argocd-server state: $LAST_ARGOCD_SERVER_STATE`,
		`Last session response: $LAST_ARGOCD_SESSION_STATUS`,
	}
	for _, needle := range required {
		if !strings.Contains(script, needle) {
			t.Fatalf("expected retrying token flow to include %q", needle)
		}
	}
}

func TestArgoImportDemoScript_PassesHTTPSArgoServerToRenderer(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "demo.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	script := string(content)

	required := `-e "ARGOCD_SERVER=https://argocd-server.argocd.svc.cluster.local" \`
	if !strings.Contains(script, required) {
		t.Fatalf("expected renderer install to pass HTTPS ArgoCD service endpoint")
	}
	if strings.Contains(script, `-e "ARGOCD_SERVER=argocd-server.argocd.svc.cluster.local" \`) {
		t.Fatalf("renderer install still passes ArgoCD service endpoint without HTTPS scheme")
	}
}

func TestArgoImportDemoScript_PreloadsRendererImageIntoKind(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "demo.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	script := string(content)

	required := []string{
		`RENDERER_IMAGE="$(grep -m1 'image:' "$WORKER_MANIFEST" | awk '{print $2}' || true)"`,
		`docker pull "$RENDERER_IMAGE" >/dev/null 2>&1 || warn "Renderer image pull failed; cluster will pull directly"`,
		`kind load docker-image --name "$CLUSTER_NAME" "$RENDERER_IMAGE" >/dev/null 2>&1 || warn "Renderer image preload failed"`,
	}
	for _, needle := range required {
		if !strings.Contains(script, needle) {
			t.Fatalf("expected renderer preload flow to include %q", needle)
		}
	}
}
