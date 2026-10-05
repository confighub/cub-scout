#!/usr/bin/env bash
# Sourced by run-helm-version-matrix.sh; uses its owned cluster and collectors.
# All mutations are test-harness operations, never Scout commands.

run_lifecycle_matrix() {
	local version binary ns chart dir group
	readonly HOOK_IMAGE='docker.io/library/busybox@sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e'
	for version in helm3 helm4; do
		binary=$HELM3_BIN
		if [[ "$version" == helm4 ]]; then binary=$HELM4_BIN; fi
		ns="matrix-$version-lifecycle"
		chart="$EVIDENCE_DIR/chart-$version-lifecycle"
		dir="$EVIDENCE_DIR/lifecycle/$version"
		group="$version.scout-matrix.test"
		mkdir -p "$dir" "$chart/crds"
		cp -R "$EVIDENCE_DIR/chart/templates" "$chart/"
		cp "$EVIDENCE_DIR/chart/Chart.yaml" "$chart/"
		cat >"$chart/values.yaml" <<'EOF'
replicaCount: 1
marker: first
failHook: false
EOF
		cat >"$chart/crds/probe.yaml" <<EOF
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: probes.$group
  annotations:
    matrix.scout.test/crd-version: first
spec:
  group: $group
  scope: Namespaced
  names:
    plural: probes
    singular: probe
    kind: Probe
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                marker:
                  type: string
EOF
		cat >"$chart/templates/probe.yaml" <<EOF
apiVersion: $group/v1
kind: Probe
metadata:
  name: lifecycle-probe
  namespace: {{ .Release.Namespace }}
spec:
  marker: {{ .Values.marker | quote }}
EOF
		cat >"$chart/templates/hook.yaml" <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: lifecycle-hook
  namespace: {{ .Release.Namespace }}
  annotations:
    helm.sh/hook: post-install,post-upgrade
    helm.sh/hook-delete-policy: before-hook-creation
spec:
  backoffLimit: 0
  activeDeadlineSeconds: 60
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: check
          image: $HOOK_IMAGE
          command: [sh, -c]
          args: [{{ if .Values.failHook }}"exit 1"{{ else }}"exit 0"{{ end }}]
EOF
		shasum -a 256 "$chart/Chart.yaml" "$chart/values.yaml" "$chart/crds/probe.yaml" "$chart/templates/"*.yaml >"$dir/chart-initial.sha256"
		helm_call "$version-lifecycle-install" "$version" "$binary" install release-probe "$chart" -n "$ns" --create-namespace --wait --timeout 3m >"$dir/install.stdout" 2>"$dir/install.stderr"
		capture_release "$version-lifecycle-installed" "$binary" "$version" "$ns" 1
		kubectl_call "$version-successful-hook" -n "$ns" get job lifecycle-hook -o json >"$dir/hook-success.json"
		"$JQ_BIN" -e '.status.succeeded == 1 and ([.status.conditions[]? | select(.type == "Complete" and .status == "True")] | length) == 1' "$dir/hook-success.json" >/dev/null || fail "$version successful hook lacks completion proof"
		kubectl_call "$version-crd-established" wait --for=condition=Established "crd/probes.$group" --timeout=30s >"$dir/crd-wait.stdout"
		kubectl_call "$version-crd-initial" get "crd/probes.$group" -o json >"$dir/crd-initial.json"
		kubectl_call "$version-dependent-initial" -n "$ns" get "probes.$group" lifecycle-probe -o json >"$dir/dependent-initial.json"
		"$JQ_BIN" -e '.spec.marker == "first"' "$dir/dependent-initial.json" >/dev/null || fail "$version dependent resource differs"
		# Helm does not upgrade CRDs in crds/. Capture that separately from the
		# dependent object, which is part of the ordinary release manifest.
		python3 - "$chart/crds/probe.yaml" <<'PY'
from pathlib import Path
import sys
p = Path(sys.argv[1])
p.write_text(p.read_text().replace('crd-version: first', 'crd-version: second'))
PY
		shasum -a 256 "$chart/crds/probe.yaml" >"$dir/chart-upgrade-crd.sha256"
		local upgrade_exit=0
		helm_call "$version-failed-upgrade" "$version" "$binary" upgrade release-probe "$chart" -n "$ns" --set replicaCount=3 --set marker=second --set failHook=true --wait --timeout 90s >"$dir/failed-upgrade.stdout" 2>"$dir/failed-upgrade.stderr" || upgrade_exit=$?
		printf '%s\n' "$upgrade_exit" >"$dir/failed-upgrade.exit"
		[[ "$upgrade_exit" != 0 ]] || fail "$version failing hook did not fail upgrade"
		kubectl_call "$version-failed-hook" -n "$ns" get job lifecycle-hook -o json >"$dir/hook-failed.json"
		"$JQ_BIN" -e '([.status.conditions[]? | select(.type == "Failed" and .status == "True")] | length) == 1' "$dir/hook-failed.json" >/dev/null || fail "$version hook failure was not observed"
		helm_call "$version-failed-history" "$version" "$binary" history release-probe -n "$ns" -o json >"$dir/history-after-failure.json"
		"$JQ_BIN" -e '.[-1].status == "failed"' "$dir/history-after-failure.json" >/dev/null || fail "$version failed release not retained"
		kubectl_call "$version-crd-after-upgrade" get "crd/probes.$group" -o json >"$dir/crd-after-upgrade.json"
		"$JQ_BIN" -e '.metadata.annotations["matrix.scout.test/crd-version"] == "first"' "$dir/crd-after-upgrade.json" >/dev/null || fail "$version unexpectedly upgraded CRD"
		kubectl_call "$version-dependent-after-upgrade" -n "$ns" get "probes.$group" lifecycle-probe -o json >"$dir/dependent-after-upgrade.json"
		"$JQ_BIN" -e '.spec.marker == "second"' "$dir/dependent-after-upgrade.json" >/dev/null || fail "$version expected failed upgrade resource change not observed"
		helm_call "$version-rollback" "$version" "$binary" rollback release-probe 1 -n "$ns" --wait --timeout 3m >"$dir/rollback.stdout" 2>"$dir/rollback.stderr"
		capture_release "$version-lifecycle-rolled-back" "$binary" "$version" "$ns" 1
		kubectl_call "$version-dependent-restored" -n "$ns" get "probes.$group" lifecycle-probe -o json >"$dir/dependent-restored.json"
		"$JQ_BIN" -e '.spec.marker == "first"' "$dir/dependent-restored.json" >/dev/null || fail "$version rollback failed to restore dependent configuration"
		helm_call "$version-hooks-after-rollback" "$version" "$binary" get hooks release-probe -n "$ns" >"$dir/hooks-after-rollback.yaml"
		kubectl_call "$version-delete-retained-hook" -n "$ns" delete job lifecycle-hook --wait=true --timeout=30s >"$dir/hook-delete.stdout"
		kubectl_call "$version-deleted-hook-list" -n "$ns" get jobs --field-selector=metadata.name=lifecycle-hook -o json >"$dir/hooks-after-delete.json"
		"$JQ_BIN" -e '.items | length == 0' "$dir/hooks-after-delete.json" >/dev/null || fail "$version deleted hook still present"
	done
	# Transfer one field to a distinct manager, then require an actual conflict
	# during the explicit client-to-server apply transition. A failed Helm
	# command is not enough: it must fail for this conflict and preserve the field.
	local conflict_dir="$EVIDENCE_DIR/lifecycle/apply-transition"
	mkdir -p "$conflict_dir"
	cat >"$conflict_dir/foreign-field.yaml" <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: release-probe
  namespace: matrix-h3-to-h4
spec:
  replicas: 5
EOF
	kubectl_call foreign-field-owner apply --server-side --force-conflicts --field-manager=matrix-foreign -f "$conflict_dir/foreign-field.yaml" >"$conflict_dir/foreign-apply.stdout" 2>"$conflict_dir/foreign-apply.stderr"
	local conflict_exit=0
	helm_call explicit-server-conflict helm4 "$HELM4_BIN" upgrade release-probe "$EVIDENCE_DIR/chart" -n matrix-h3-to-h4 --set replicaCount=2 --server-side=true --wait --timeout 90s >"$conflict_dir/conflict.stdout" 2>"$conflict_dir/conflict.stderr" || conflict_exit=$?
	printf '%s\n' "$conflict_exit" >"$conflict_dir/conflict.exit"
	[[ "$conflict_exit" != 0 ]] || fail "expected server-side field conflict"
	grep -Eiq 'conflict' "$conflict_dir/conflict.stderr" || fail "apply transition failed for another reason"
	kubectl_call conflict-preserved-field -n matrix-h3-to-h4 get deployment release-probe --show-managed-fields=true -o json >"$conflict_dir/after-conflict.json"
	"$JQ_BIN" -e '.spec.replicas == 5' "$conflict_dir/after-conflict.json" >/dev/null || fail "failed conflict changed the foreign field"
	helm_call explicit-server-force helm4 "$HELM4_BIN" upgrade release-probe "$EVIDENCE_DIR/chart" -n matrix-h3-to-h4 --set replicaCount=2 --server-side=true --force-conflicts --wait --timeout 3m >"$conflict_dir/forced.stdout" 2>"$conflict_dir/forced.stderr"
	capture_release explicit-server-forced "$HELM4_BIN" helm4 matrix-h3-to-h4 2
}
