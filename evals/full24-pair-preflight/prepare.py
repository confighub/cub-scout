#!/usr/bin/env python3
"""Prepare and validate the frozen 24-case equal-evidence packet offline.

This module reads pinned repository inputs and writes a private preparation
tree. It never executes a scaffold, Claude, a provider, a client, or a grader.
"""
from __future__ import annotations

import argparse
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import tempfile
import yaml

REPO = Path(__file__).resolve().parents[2]
MANIFEST = Path("evals/benchmark-v1.json")
QUERY_PLAN = Path("evals/ordinary-tool-query-plan-v1.md")
CONTRACT = "full24-equal-evidence.v1"
EXPECTED_GROUPS = ("inventory", "attribution", "delivery_identity", "health", "prerequisites_graph", "reuse_limits")
EXPECTED_COUNTS = {group: 4 for group in EXPECTED_GROUPS}
FROZEN_SEMANTICS_SHA256 = "6fad0fdd92562792c0046bb8cf92f6c1a441035c57ad3d7a0733e801d3e47bae"
LEGACY_RECORDING_SHA256 = "dc93a5ad77af340a2d71698d2d726c3ef941a6528589e227593f973364c45e22"
FROZEN_CASE_IDS = ("INV-01", "INV-02", "INV-03", "INV-04", "ATR-01", "ATR-02", "ATR-03", "ATR-04",
                   "DEL-01", "DEL-02", "DEL-03", "DEL-04", "HLT-01", "HLT-02", "HLT-03", "HLT-04",
                   "PRE-01", "PRE-02", "PRE-03", "PRE-04", "RUL-01", "RUL-02", "RUL-03", "RUL-04")
SCALE_FILES = (
    "deployments.yaml", "namespaces.yaml", "replicasets.yaml", "pods.yaml",
    "services.yaml", "configmaps.yaml", "events.yaml",
)
LEGACY_CASES = {"INV-03", "ATR-01", "ATR-02", "ATR-03", "ATR-04"}
SCALE_CASES = {"INV-01", "INV-02"}
SCAFFOLD_SHA256 = {
    "INV-01": "6b5659d033c7a9dde3688d58521a62fc66feac3f633604a71f5d6e8cdf46e020", "INV-02": "6b5659d033c7a9dde3688d58521a62fc66feac3f633604a71f5d6e8cdf46e020",
    "INV-03": "c27a8e21114d46fb210dbda7b6a4bed1fd0e374c9cb4b69df5a2e9276a25bc65", "INV-04": "b5c5fd30ea5cd96b9aa40085dc200cc6515f1bc2ac3802316aec60aa96f40240",
    "ATR-01": "c27a8e21114d46fb210dbda7b6a4bed1fd0e374c9cb4b69df5a2e9276a25bc65", "ATR-02": "c27a8e21114d46fb210dbda7b6a4bed1fd0e374c9cb4b69df5a2e9276a25bc65",
    "ATR-03": "c27a8e21114d46fb210dbda7b6a4bed1fd0e374c9cb4b69df5a2e9276a25bc65", "ATR-04": "c27a8e21114d46fb210dbda7b6a4bed1fd0e374c9cb4b69df5a2e9276a25bc65",
    "DEL-01": "dc47e3bb17694b9c4c7fe512e03e8710532e5827cf08f5bff8f8571f844c0a54", "DEL-02": "9245db44be5ecb12c4014e3a2bc72f291698b810f9310da723a80ff43456edf6",
    "DEL-03": "d90fe8431197a29a0f6dcd82c031f6f0781b6fb55538acaadbb2232ca5d52ce0", "DEL-04": "d0753bddce4be39f6eda4d7772361bedae46b9ff9121acd6b3d7b4ee6cdd2574",
    "HLT-01": "c02c1bf87d4ebf3f54ee61c47a4812e6a9c4bb650525a2f9246bf98b5dbd7e33", "HLT-02": "0287efdd12fae8c10f491cd875cf7968a4eb43e1864453f790754aea7959cd42",
    "HLT-03": "a79579f2000147c6a48c3a2d0f9a43c6911058c4dd41c9a5cdeb028b87db3bce", "HLT-04": "469d96d37f6744ec66123511112724ab16234afbf1ca3c0901ab0df53bc2dbb9",
    "PRE-01": "81952a6be24443fdb93d895e9ad18e22969751f374f4484d5eb907077d4350c7", "PRE-02": "abef7b85f07f0432c44007c831c642a201fea68ee3a417419022124c5f283600",
    "PRE-03": "a1856f62bdca97d04a01295446c533d95ddc0d01a3436780b81a9922454642e5", "PRE-04": "d1dce1eccd89b653077381d9606f4ccd836c74746f19d7b451983d074806ab8d",
    "RUL-01": "19dadf628564b48f4fde3dd6cf098738d947974908eef7a74f39736fa3ba8069", "RUL-02": "90a465058fa1894aa0da139b0f367385dad92b8c089e4c7c0bafb6f97ae25885",
    "RUL-03": "2c878c0b0f91f7c9a9668548d4dbf23c530aceb7ffd5463d713e339aa75e8d9b", "RUL-04": "1bc18539d33ae4e6d3f1fe5a29692066c6167287322c8ee52640bc5cecb1fe86",
}
AUTHORED_CONTROLS = {
    "DEL-03": {
        "id": "del03-missing-stale-identity-input-control.v1",
        "kind": "authored-input-variant-not-a-new-question",
        "mutations": [
            {"file": "cluster/sveltos-oci-delivery-proof.yaml", "operation": "withhold_receipt_identity_fields",
             "fields": ["receipt.cluster", "receipt.release.releaseId", "receipt.release.releaseManifestDigest"]},
            {"file": "cluster/source-metadata.json", "operation": "mark_status_row_stale",
             "basis": "authored fixture clock is later than the recorded row interval; not a new observation"},
        ],
        "prohibited_inference": "Do not claim current/stable cross-artifact cluster or release identity from absent/stale identity fields.",
        "status": "authored-control-not-run",
    },
    "DEL-04": {
        "id": "del04-divergent-identity-mutable-tag-input-control.v1",
        "kind": "authored-input-variant-not-a-new-question",
        "mutations": [
            {"file": "evidence/oci-identity-lifecycle/catalog-delivery-proof.yaml", "operation": "add_authored_tag_observation",
             "values_derived_from": ["oci-evidence-chain.yaml", "catalog-delivery-proof.yaml"],
             "condition": "same mutable tag is shown at two receipt times with distinct recorded digests; no new runtime identity"},
        ],
        "prohibited_inference": "Do not collapse a mutable tag or divergent receipt digests into a single verified current artifact identity.",
        "status": "authored-control-not-run",
    },
}


class PreparationError(ValueError):
    pass


class UniqueKeyLoader(yaml.SafeLoader):
    pass


def _unique_mapping(loader, node, deep=False):
    result = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in result:
            raise PreparationError(f"duplicate YAML key: {key}")
        result[key] = loader.construct_object(value_node, deep=deep)
    return result


UniqueKeyLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, _unique_mapping)


def _yaml_load(text: str):
    return yaml.load(text, Loader=UniqueKeyLoader)


def _yaml_load_all(text: str):
    return yaml.load_all(text, Loader=UniqueKeyLoader)


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _case_index(manifest: dict) -> tuple[dict[str, dict], dict[str, str]]:
    if not isinstance(manifest, dict) or not isinstance(manifest.get("groups"), list):
        raise PreparationError("benchmark manifest has no groups array")
    if [g.get("id") for g in manifest["groups"]] != list(EXPECTED_GROUPS):
        raise PreparationError("benchmark group sequence changed")
    cases: dict[str, dict] = {}
    groups: dict[str, str] = {}
    for group in manifest["groups"]:
        if group.get("weight") != 0.1666666667 or len(group.get("cases", [])) != EXPECTED_COUNTS[group["id"]]:
            raise PreparationError("frozen group weight or case count changed")
        for case in group["cases"]:
            case_id = case.get("id")
            if not isinstance(case_id, str) or case_id in cases:
                raise PreparationError("duplicate or malformed benchmark case id")
            cases[case_id] = case
            groups[case_id] = group["id"]
    if tuple(cases) != FROZEN_CASE_IDS:
        raise PreparationError("frozen benchmark case identities/order changed")
    selected = []
    for group in manifest["groups"]:
        for case in group["cases"]:
            selected.append({key: case.get(key) for key in ("id", "existing_case", "question", "reference", "controls")}
                            | {"group": group["id"]})
    projection = {"schema": "full24-frozen-semantics.v1",
                  "groupWeights": [{"id": group["id"], "weight": group["weight"]} for group in manifest["groups"]],
                  "cases": selected}
    sem_hash = digest(json.dumps(projection, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode())
    if sem_hash != FROZEN_SEMANTICS_SHA256:
        raise PreparationError("frozen questions/references/controls/weights changed")
    return cases, groups


def _frontmatter(source: bytes) -> tuple[dict, str]:
    try:
        text = source.decode("utf-8", "strict")
    except UnicodeDecodeError:
        raise PreparationError("prompt is not UTF-8") from None
    match = re.fullmatch(r"---\n(.*?)\n---\n(.*)", text, re.DOTALL)
    if not match:
        # The two recorded-source cases use a plain prompt body and keep their
        # grant in case.yaml; all other cases have prompt frontmatter.
        return {}, text
    raw, body = match.groups()
    try:
        value = _yaml_load(raw)
    except yaml.YAMLError as exc:
        raise PreparationError(f"prompt frontmatter YAML is malformed: {exc}") from None
    if not isinstance(value, dict):
        raise PreparationError("prompt frontmatter must be a YAML mapping")
    metadata = {key: value[key] for key in ("name", "description", "tags", "max_turns", "timeout_seconds", "allowed_tools") if key in value}
    for key in ("max_turns", "timeout_seconds"):
        if key in metadata and (type(metadata[key]) is not int or metadata[key] <= 0):
            raise PreparationError(f"invalid prompt budget: {key}")
    if "tags" in metadata and (not isinstance(metadata["tags"], list) or not all(isinstance(tag, str) for tag in metadata["tags"])):
        raise PreparationError("prompt tags must be a string list")
    if "name" in metadata and not isinstance(metadata["name"], str):
        raise PreparationError("prompt name must be text")
    if "description" in metadata and not isinstance(metadata["description"], str):
        raise PreparationError("prompt description must be text")
    if "allowed_tools" in metadata:
        tools = metadata["allowed_tools"]
        if not isinstance(tools, list) or not tools or not all(isinstance(tool, str) and tool for tool in tools):
            raise PreparationError("ordinary tool grant must be a nonempty string list")
        if len(tools) != len(set(tools)):
            raise PreparationError("ordinary tool grant contains duplicates")
    if not isinstance(metadata.get("allowed_tools"), list):
        raise PreparationError("prompt does not declare ordinary tool grants")
    return metadata, body


def _strict_modules():
    paths = {
        "legacy": REPO / "evals/legacy-strict-cases/prepare.py",
        "scale": REPO / "evals/recorded-scale/prepare.py",
    }
    modules = {}
    for name, path in paths.items():
        spec = importlib.util.spec_from_file_location("full24_" + name, path)
        if spec is None or spec.loader is None:
            raise PreparationError(f"cannot load pure {name} preparer")
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        modules[name] = mod
    return modules


def _strict_variant(case_id: str, case_path: Path, metadata: dict, body: str, modules: dict) -> tuple[str, dict[str, bytes], str]:
    """Return selected prompt body and generated answer grader (oracle-only)."""
    if case_id in LEGACY_CASES:
        legacy = modules["legacy"]
        source = (case_path / "prompt.md").read_bytes()
        cfg = legacy.CASES[case_id]
        generated = legacy.strict_prompt(source, case_id, cfg["prompt_suffix"]).decode()
        _, generated_body = _frontmatter(generated.encode())
        grader_name = cfg["answer_grader"]
        grader = legacy.grader_bytes(cfg["answer"]).decode()
        return generated_body, {grader_name: grader.encode()}, legacy.CONTRACT
    if case_id in SCALE_CASES:
        scale = modules["scale"]
        prompt = body.rstrip() + "\n\n" + scale.STRICT_PROMPT_SUFFIX + "\n"
        grader_name, pattern = scale.STRICT_GRADERS[case_path.name]
        grader = ("---\ntype: regex\npattern: '" + pattern + "'\nflags: s\ntarget: last_message\n---\n").encode()
        return prompt, {grader_name: grader}, "recorded-scale-answer-line.v1"
    return body, {}, "case-declared-prospective.v1"


def _grader_definition(data: bytes) -> dict:
    try:
        text = data.decode("utf-8", "strict")
        match = re.fullmatch(r"---\n(.*?)\n---\n?(.*)", text, re.DOTALL)
        value = _yaml_load(match.group(1) if match else text)
    except (UnicodeDecodeError, yaml.YAMLError, PreparationError):
        raise PreparationError("grader definition is malformed YAML") from None
    if not isinstance(value, dict) or not isinstance(value.get("type"), str):
        raise PreparationError("grader definition does not declare a type")
    weight = value.get("weight", 1)
    if type(weight) not in (int, float) or weight <= 0:
        raise PreparationError("answer grader must have positive numeric weight")
    return {"type": value["type"], "weight": weight}


def _selected_answer_graders(case_path: Path, generated: dict[str, bytes]) -> tuple[dict[str, bytes], list[dict], list[str]]:
    if generated:
        candidates = {name: data for name, data in generated.items()}
    else:
        candidates = {p.name: p.read_bytes() for p in sorted((case_path / "graders").glob("*.md"))}
    answer = {}
    instrumentation = []
    for name, data in candidates.items():
        definition = _grader_definition(data)
        if definition["type"] == "tool_used":
            instrumentation.append(name)
        else:
            answer[name] = data
    if len(answer) != 1:
        raise PreparationError(f"{case_path.name} must select exactly one positive-weight correctness grader")
    selected = []
    for name, data in answer.items():
        definition = _grader_definition(data)
        selected.append({"name": name, "type": definition["type"], "weight": definition["weight"], "source": "generated-strict" if generated else "case-source"})
    return answer, selected, instrumentation


def _parse_static_heredocs(script: bytes) -> dict[str, bytes]:
    """Interpret only the audited mkdir + quoted static heredoc scaffold subset."""
    try:
        lines = script.decode("utf-8", "strict").splitlines()
    except UnicodeDecodeError:
        raise PreparationError("scaffold is not UTF-8") from None
    outputs: dict[str, bytes] = {}
    i = 0
    while i < len(lines):
        line = lines[i]
        if not line.strip() or line.startswith("#") or line.startswith("#!") or line == "set -euo pipefail":
            i += 1
            continue
        if line == "mkdir -p cluster":
            i += 1
            continue
        m = re.fullmatch(r"cat > cluster/([A-Za-z0-9._-]+) <<'([A-Z0-9_]+)'", line)
        if not m:
            raise PreparationError("scaffold is outside audited static-heredoc/copy subset")
        name, marker = m.groups()
        if name in outputs:
            raise PreparationError("scaffold writes duplicate evidence path")
        i += 1
        content: list[str] = []
        while i < len(lines) and lines[i] != marker:
            content.append(lines[i])
            i += 1
        if i == len(lines):
            raise PreparationError("unterminated static scaffold heredoc")
        outputs["cluster/" + name] = ("\n".join(content) + "\n").encode()
        i += 1
    if not outputs:
        raise PreparationError("static scaffold had no authored outputs")
    return outputs


def _inputs_for(case_id: str, case_path: Path, scaffold: bytes) -> dict[str, bytes]:
    # Generated checked-in source exports used by inventory/attribution cases.
    if case_id in SCALE_CASES:
        root = REPO / "evals/fixtures/scale/cluster"
        return {"cluster/" + name: (root / name).read_bytes() for name in SCALE_FILES}
    if case_id in LEGACY_CASES:
        root = REPO / "evals/fixtures/cluster"
        return {"cluster/" + name: (root / name).read_bytes() for name in SCALE_FILES}
    if case_id == "HLT-02":
        fixtures = case_path / "fixtures/2026-09-30"
        output = {"cluster/" + p.name: p.read_bytes() for p in sorted(fixtures.glob("*.json"))}
        for name in ("gitrepository.yaml", "kustomization.yaml"):
            output["cluster/" + name] = (case_path / name).read_bytes()
        return output
    fixtures = case_path / "fixtures"
    if fixtures.is_dir():
        files = sorted(p for p in fixtures.rglob("*") if p.is_file())
        if not files or any(p.is_symlink() for p in files):
            raise PreparationError(f"{case_id} fixture directory is empty or contains symlinks")
        prefix = "evidence/oci-identity-lifecycle/" if case_id == "DEL-04" else "cluster/"
        names = [p.name for p in files]
        if len(names) != len(set(names)):
            raise PreparationError(f"{case_id} fixture basenames collide under the reviewed scaffold layout")
        return {prefix + p.name: p.read_bytes() for p in files}
    # DEL-01/02 are generated only by reviewed, fully literal heredoc scripts.
    return _parse_static_heredocs(scaffold)


def _assert_scaffold_binding(case_id: str, script: bytes, inputs: dict[str, bytes]) -> None:
    if case_id not in SCAFFOLD_SHA256 or digest(script) != SCAFFOLD_SHA256[case_id]:
        raise PreparationError(f"{case_id} scaffold differs from the reviewed source pin")
    text = script.decode("utf-8", "strict")
    if "kubectl" in text or "helm " in text or "curl " in text or "https://" in text:
        # Historical commands in comments/data are not to be run, but no script
        # is executed here. Bind only source paths or static heredocs below.
        pass
    if "cat > cluster/" in text:
        parsed = _parse_static_heredocs(script)
        if parsed != inputs:
            raise PreparationError(f"{case_id} static scaffold output does not equal staged input bytes")
    elif case_id in SCALE_CASES or case_id in LEGACY_CASES:
        # Existing capture scaffolds are giant generated literals; these exports
        # are bound by their separately pinned recording manifests instead.
        if "evals/fixtures" not in text and "Generated by evals/scripts/record.py" not in text:
            raise PreparationError(f"{case_id} generated export scaffold provenance is unrecognized")
    elif not ("cp " in text or "cp --" in text or "for source in" in text):
        raise PreparationError(f"{case_id} scaffold is not recognized as copy-only")
    else:
        for rel in inputs:
            if Path(rel).name not in text and not (
                    case_id == "HLT-02" and Path(rel).suffix == ".json" and "fixtures/2026-09-30/*.json" in text) and not (
                    case_id == "PRE-01" and 'for source in "$root"/fixtures/*' in text):
                raise PreparationError(f"{case_id} scaffold does not name staged input {Path(rel).name}")


def _evidence_source(case_id: str, case_path: Path, destination: str) -> str:
    name = Path(destination).name
    if case_id in SCALE_CASES:
        return f"evals/fixtures/scale/cluster/{name}"
    if case_id in LEGACY_CASES:
        return f"evals/fixtures/cluster/{name}"
    if case_id == "HLT-02" and name in {"gitrepository.yaml", "kustomization.yaml"}:
        return f"{case_path.relative_to(REPO).as_posix()}/{name}"
    if not (case_path / "fixtures").is_dir():
        return f"{case_path.relative_to(REPO).as_posix()}/scaffold.sh#static-heredoc/{name}"
    matches = [p for p in (case_path / "fixtures").rglob("*") if p.is_file() and p.name == name]
    if len(matches) != 1:
        raise PreparationError(f"{case_id} evidence destination has ambiguous source mapping: {destination}")
    return matches[0].relative_to(REPO).as_posix()


def _safe_tree_hashes(root: Path) -> dict[str, str]:
    result = {}
    for p in sorted(root.rglob("*")):
        if p.is_symlink():
            raise PreparationError("symlink present in prepared/model-visible tree")
        if p.is_file():
            result[p.relative_to(root).as_posix()] = digest(p.read_bytes())
    return result


def _write_bytes(root: Path, relative: str, data: bytes) -> None:
    path = root / relative
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)


def _model_prompt_bytes(metadata: dict, body: str) -> bytes:
    safe_front = {k: metadata[k] for k in ("name", "description", "tags", "max_turns", "timeout_seconds", "allowed_tools") if k in metadata}
    front = "---\n" + "".join(f"{k}: {json.dumps(v, ensure_ascii=False) if isinstance(v, (str, list)) else v}\n" for k, v in safe_front.items()) + "---\n"
    return (front + body).encode()


def _validate_scale_sources() -> dict:
    """Reuse the pinned scale preparer and report validator for INV-01/02."""
    prep_spec = importlib.util.spec_from_file_location("full24_scale_prepare", REPO / "evals/recorded-scale/prepare.py")
    prep = importlib.util.module_from_spec(prep_spec)
    assert prep_spec and prep_spec.loader
    prep_spec.loader.exec_module(prep)
    prep.verify_sources()
    pre_spec = importlib.util.spec_from_file_location("full24_scale_preflight", REPO / "evals/recorded-scale/preflight.py")
    pre = importlib.util.module_from_spec(pre_spec)
    assert pre_spec and pre_spec.loader
    pre_spec.loader.exec_module(pre)
    path = REPO / "evals/fixtures/scale/cluster/deployments.yaml"
    try:
        docs = list(_yaml_load_all(path.read_text()))
    except (yaml.YAMLError, PreparationError):
        raise PreparationError("scale Deployment source YAML is malformed or has duplicate keys") from None
    objects = []
    for doc in docs:
        if isinstance(doc, dict) and doc.get("kind") == "List":
            items = doc.get("items")
            if not isinstance(items, list):
                raise PreparationError("scale Deployment list has no items array")
            objects.extend(items)
        elif doc is not None:
            objects.append(doc)
    if len(objects) != 302:
        raise PreparationError("scale source must parse as exactly 302 Deployment documents")
    identities = set()
    selected = []
    excluded = 0
    counts = {owner: 0 for owner in pre.EXPECTED_COUNTS}
    no_marker = []
    for obj in objects:
        if not isinstance(obj, dict) or obj.get("apiVersion") != "apps/v1" or obj.get("kind") != "Deployment":
            raise PreparationError("scale list contains malformed or non-Deployment object")
        meta = obj.get("metadata")
        if not isinstance(meta, dict) or not isinstance(meta.get("namespace"), str) or not isinstance(meta.get("name"), str):
            raise PreparationError("scale Deployment missing namespace/name")
        identity = (meta["namespace"], meta["name"])
        if identity in identities:
            raise PreparationError("scale source contains duplicate Deployment identity")
        identities.add(identity)
        if not meta["namespace"].startswith("team-"):
            excluded += 1
            continue
        labels = meta.get("labels") or {}
        if not isinstance(labels, dict):
            raise PreparationError("scale Deployment labels are malformed")
        if any(k.startswith(("kustomize.toolkit.fluxcd.io/", "helm.toolkit.fluxcd.io/")) for k in labels):
            owner = "Flux"
        elif "argocd.argoproj.io/instance" in labels:
            owner = "ArgoCD"
        elif labels.get("app.kubernetes.io/managed-by") == "Helm":
            owner = "Helm"
        elif "confighub.com/UnitSlug" in labels:
            owner = "ConfigHub"
        else:
            owner = "Native"
            no_marker.append(f"{identity[0]}/{identity[1]}")
        counts[owner] += 1
        selected.append({"apiVersion": "apps/v1", "kind": "Deployment", "namespace": identity[0], "name": identity[1], "owner": owner})
    if len(selected) != 300 or excluded != 2:
        raise PreparationError("scale source scope must have 300 selected and 2 excluded rows")
    report = {"schema": "map-list-recorded.v1",
              "provenance": {"objectCount": 302, "sha256": pre.DEPLOYMENT_SHA256,
                             "captureTime": "unknown", "captureCompleteness": "unknown"},
              "scope": {"apiVersion": "apps/v1", "kind": "Deployment", "namespacePrefix": "team-"},
              "selectedCount": 300, "excludedFromScopeCount": 2,
              "ownerCounts": counts, "resources": selected}
    pre.validate_report(report)
    return {"parsed": len(objects), "selected": len(selected), "excluded": excluded,
            "scope": report["scope"], "captureCompleteness": "unknown",
            "sourceManifestSha256": prep.MANIFEST_SHA256, "deploymentSha256": pre.DEPLOYMENT_SHA256}


def _validate_legacy_recording() -> dict:
    path = REPO / "evals/fixtures/recording-2026-09-30.json"
    raw = path.read_bytes()
    if digest(raw) != LEGACY_RECORDING_SHA256:
        raise PreparationError("shared legacy raw-export manifest hash mismatch")
    manifest = json.loads(raw)
    files = manifest.get("files")
    if not isinstance(files, dict):
        raise PreparationError("legacy raw-export manifest is malformed")
    entries = {}
    for name in SCALE_FILES:
        rel = f"evals/fixtures/cluster/{name}"
        source = REPO / rel
        expected = files.get(rel)
        if not source.is_file() or not isinstance(expected, str) or digest(source.read_bytes()) != expected:
            raise PreparationError(f"legacy raw export differs from pinned manifest: {rel}")
        entries[rel] = expected
    return {"path": "evals/fixtures/recording-2026-09-30.json", "sha256": digest(raw), "files": entries}


def _authored_control_payload(case_id: str, source_case: Path) -> tuple[dict, dict[str, bytes], bytes, bytes, bytes]:
    """Reconstruct the exact authored input mutation from its pinned sources."""
    spec = AUTHORED_CONTROLS[case_id]
    source_facts = {p.relative_to(source_case).as_posix(): digest(p.read_bytes())
                    for p in sorted((source_case / "fixtures").rglob("*")) if p.is_file()}
    if case_id == "DEL-03":
        onboard = (source_case / "fixtures/source-doc/onboard-excerpt.md").read_text()
        stale_status, n = re.subn(r"(?m)^eu-central-uat1\s+mer-kyverno-eu-central-uat1", "UNKNOWN UNKNOWN", onboard, count=1)
        if n != 1:
            raise PreparationError("DEL-03 status identity row changed")
        receipt_obj = _yaml_load((source_case / "fixtures/receipt/sveltos-oci-delivery-proof.yaml").read_text())
        spec_obj = receipt_obj.get("spec", {})
        variants = spec_obj.get("variants")
        if not isinstance(variants, list) or not variants or not all(isinstance(v, dict) for v in variants):
            raise PreparationError("DEL-03 receipt variants malformed")
        spec_obj["variants"] = [{"identity": "WITHHELD_BY_CONTROL",
                                 "sourceVariantSha256": digest(json.dumps(v, sort_keys=True, separators=(",", ":")).encode())}
                                for v in variants]
        receipt = yaml.safe_dump(receipt_obj, sort_keys=False, allow_unicode=True).encode()
        marker = {"schema": "authored-stale-marker.v1", "sourceRecordedAt": spec_obj.get("recordedAt"),
                  "authoredInterval": {"statusObservedAt": "2026-09-01T00:00:00Z", "evaluatedAt": "2026-10-01T00:00:00Z",
                                       "maxAgeSeconds": 86400, "elapsedSeconds": 2592000, "result": "STALE"},
                  "freshnessRule": "authored interval only; source status-row timestamp is absent",
                  "classification": "STALE_CONTROL_ONLY; no source observation time is asserted",
                  "sourceFixtureSha256": source_facts}
        files = {"status-excerpt.md": stale_status.encode(), "delivery-receipt.yaml": receipt,
                 "stale-control-marker.json": json.dumps(marker, indent=2).encode() + b"\n"}
        prompt = ("DEL-03 authored input-control variant (non-weighted, not a new frozen question). "
                  "Read the redacted status excerpt, redacted receipt and control marker. This is an "
                  "authored omission/staleness input derived from pinned source bytes, not a capture. "
                  "Treat missing identity as unknown and do not join artifacts.\n").encode()
        acceptance = {"status_cluster": "UNKNOWN", "status_revision": "UNKNOWN", "status_revision_basis": "UNKNOWN",
                      "receipt_cluster": "UNKNOWN", "runtime_sveltos_digest": "UNKNOWN", "cross_artifact_join": "UNESTABLISHED"}
    else:
        chain = (source_case / "fixtures/evidence/oci-evidence-chain.yaml").read_text()
        original = _yaml_load(chain)
        bounds = original.get("spec", {}).get("boundaries", {})
        output = bounds.get("outputOci", {})
        delivery = bounds.get("delivery", {})
        source_digest = bounds.get("source", {}).get("digest")
        if (not output.get("reference", "").endswith(":latest") or output.get("digest") != delivery.get("digest") or
                not isinstance(source_digest, str) or source_digest == output.get("digest")):
            raise PreparationError("DEL-04 source same-role identity facts changed")
        conflict = copy.deepcopy(original)
        conflict["spec"]["boundaries"]["delivery"]["digest"] = source_digest
        control = {"schema": "del04-authored-divergent-identity-input.v1", "sourceFixtureSha256": source_facts,
                   "mutableTag": "latest", "originalDeliveryDigest": delivery["digest"],
                   "authoredConflictingOutputDigest": source_digest,
                   "mutation": "authored copy changes the delivery digest for the same latest OCI reference to a different already-recorded source digest",
                   "evidenceKind": "authored-conflicting-receipt-not-a-capture"}
        files = {"source-chain-original.yaml": chain.encode(),
                 "source-chain-authored-conflict.yaml": yaml.safe_dump(conflict, sort_keys=False, allow_unicode=True).encode(),
                 "divergence-control.json": json.dumps(control, indent=2).encode() + b"\n"}
        prompt = ("DEL-04 authored input-control variant (non-weighted, not a new frozen question). "
                  "Compare the unchanged source receipt and the explicitly authored conflicting copy. "
                  "The altered receipt is a negative control, not an observation. Do not infer which "
                  "digest the mutable tag currently resolves to.\n").encode()
        acceptance = {"delivery_digest_consistency": "CONFLICT_UNRESOLVED", "current_cluster_state": "UNKNOWN",
                      "runtime_image_id": "UNKNOWN", "independent_bundle_verification": "UNKNOWN",
                      "mutable_tag_current_identity": "UNKNOWN"}
    acceptance_bytes = json.dumps(acceptance, indent=2).encode() + b"\n"
    control_bytes = json.dumps(spec | {"sourceFixtureSha256": source_facts}, indent=2).encode() + b"\n"
    return {"sourceFixtureSha256": source_facts, "id": spec["id"]}, files, prompt, acceptance_bytes, control_bytes


def prepare(out: Path) -> dict:
    if out.exists() and any(out.iterdir()):
        raise PreparationError("output directory must be empty")
    manifest_bytes = (REPO / MANIFEST).read_bytes()
    try:
        manifest = json.loads(manifest_bytes)
    except json.JSONDecodeError:
        raise PreparationError("benchmark manifest is invalid JSON") from None
    cases, groups = _case_index(manifest)
    plan = (REPO / QUERY_PLAN).read_bytes()
    modules = _strict_modules()
    legacy_source_facts = modules["legacy"].verify_source()
    legacy_recording_facts = _validate_legacy_recording()
    out.mkdir(parents=True, exist_ok=True)
    stage_root = out / "arms"
    oracle_root = out / "oracle"
    stage_root.mkdir()
    oracle_root.mkdir()
    report_cases = []
    for case_id in sorted(cases):
        case = cases[case_id]
        rel_case = case.get("existing_case")
        case_path = REPO / rel_case
        if not case_path.is_dir():
            raise PreparationError(f"{case_id} source case is missing")
        prompt_path = case_path / "prompt.md"
        grader_dir = case_path / "graders"
        scaffold_path = case_path / "scaffold.sh"
        if not prompt_path.is_file() or not grader_dir.is_dir() or not scaffold_path.is_file():
            raise PreparationError(f"{case_id} requires prompt, graders, and scaffold")
        metadata, body = _frontmatter(prompt_path.read_bytes())
        if "allowed_tools" not in metadata:
            try:
                case_docs = list(_yaml_load_all((case_path / "case.yaml").read_text(encoding="utf-8")))
            except yaml.YAMLError:
                raise PreparationError(f"{case_id} case.yaml is malformed YAML") from None
            tool_maps = [doc["allowed_tools"] for doc in case_docs if isinstance(doc, dict) and "allowed_tools" in doc]
            if len(tool_maps) != 1:
                raise PreparationError(f"{case_id} has no declared ordinary tool grant")
            tools = tool_maps[0]
            if not isinstance(tools, list) or not tools or not all(isinstance(tool, str) and tool for tool in tools) or len(tools) != len(set(tools)):
                raise PreparationError(f"{case_id} has empty/duplicate ordinary tool grants")
            metadata["allowed_tools"] = tools
        grant = metadata["allowed_tools"]
        scaffold = scaffold_path.read_bytes()
        inputs = _inputs_for(case_id, case_path, scaffold)
        _assert_scaffold_binding(case_id, scaffold, inputs)
        if not inputs:
            raise PreparationError(f"{case_id} has no bound evidence inputs")
        selected_body, generated_graders, answer_contract = _strict_variant(case_id, case_path, metadata, body, modules)
        selected_answer_data, selected_answer_graders, instrumentation_graders = _selected_answer_graders(case_path, generated_graders)
        selected_grader_facts = {name: digest(data) for name, data in selected_answer_data.items()}
        case_binding = {
            "id": case_id,
            "group": groups[case_id],
            "sourceCase": rel_case,
            "question": case["question"],
            "reference": case["reference"],
            "controls": case["controls"],
            "ordinaryToolGrant": grant,
            "budgets": {k: metadata[k] for k in ("max_turns", "timeout_seconds") if k in metadata},
            "answerContract": answer_contract,
            "selectedGeneratedGraders": selected_grader_facts,
            "selectedAnswerGraders": selected_answer_graders,
            "instrumentationGraders": instrumentation_graders,
            "scaffold": {"path": rel_case + "/scaffold.sh", "sha256": digest(scaffold), "execution": "not-executed; audited-source-interpretation-only"},
            "promptSource": {"path": rel_case + "/prompt.md", "sha256": digest(prompt_path.read_bytes())},
            "sourceCaseYaml": {"path": rel_case + "/case.yaml", "sha256": digest((case_path / "case.yaml").read_bytes())},
            "sourceGraders": {p.relative_to(case_path).as_posix(): digest(p.read_bytes())
                              for p in sorted(grader_dir.rglob("*")) if p.is_file()},
            "evidence": {p: {"source": _evidence_source(case_id, case_path, p),
                              "sha256": digest(data), "bytes": len(data)} for p, data in sorted(inputs.items())},
        }
        for arm in ("without", "with"):
            case_root = stage_root / arm / "cases" / case_id
            _write_bytes(case_root, "prompt.md", _model_prompt_bytes(metadata, selected_body))
            _write_bytes(case_root, "ordinary-tool-grant.json", json.dumps(grant, separators=(",", ":")).encode())
            for path, data in inputs.items():
                _write_bytes(case_root, path, data)
        private_case = oracle_root / case_id
        private_case.mkdir()
        for p in sorted(grader_dir.rglob("*")):
            if p.is_file():
                _write_bytes(private_case, "source-graders/" + p.relative_to(grader_dir).as_posix(), p.read_bytes())
        for name, data in selected_answer_data.items():
            _write_bytes(private_case, "selected-graders/" + name, data)
        _write_bytes(private_case, "reference.json", json.dumps({"reference": case["reference"], "controls": case["controls"]}, indent=2).encode())
        _write_bytes(private_case, "case.yaml", (case_path / "case.yaml").read_bytes())
        report_cases.append(case_binding)
    # Equal arm equality is checked over complete model-visible per-case trees.
    without_hashes = _safe_tree_hashes(stage_root / "without")
    with_hashes = _safe_tree_hashes(stage_root / "with")
    if without_hashes != with_hashes:
        raise PreparationError("baseline and treatment source evidence/prompts/grants differ")
    # Treatment-only plugin metadata and declared skill tree. No binary is staged.
    plugin_source = (REPO / ".claude-plugin/plugin.json").read_bytes()
    plugin = json.loads(plugin_source)
    # Prevent accidental use of the live-default cub-scout MCP executable in a
    # future harness. The paired runtime must bind a reviewed recorded adapter.
    plugin["mcpServers"] = {name: {"command": "__RECORDED_MCP_BINDING_PENDING__", "args": []}
                            for name in plugin.get("mcpServers", {})}
    plugin_bytes = json.dumps(plugin, indent=2).encode() + b"\n"
    _write_bytes(stage_root / "with", ".claude-plugin/plugin.json", plugin_bytes)
    mcp_binding = {"status": "pending-reviewed-recorded-adapter", "sourceSha256": digest(plugin_source),
                   "sourceDeclaration": json.loads(plugin_source).get("mcpServers", {}),
                   "stagedCommand": "__RECORDED_MCP_BINDING_PENDING__", "execution": "not-invoked"}
    _write_bytes(stage_root / "with", "recorded-mcp-binding.json", json.dumps(mcp_binding, indent=2).encode() + b"\n")
    for p in sorted((REPO / "skills").rglob("*")):
        if p.is_file() and not p.is_symlink():
            rel = p.relative_to(REPO / "skills").as_posix()
            _write_bytes(stage_root / "with", "skills/" + rel, p.read_bytes())
    plugin_hashes = _safe_tree_hashes(stage_root / "with")
    if set(with_hashes) & set(plugin_hashes) != set(with_hashes):
        raise PreparationError("treatment plugin staging removed a common model-visible file")
    controls_root = out / "authored-input-controls"
    controls_report = {}
    for case_id, spec in AUTHORED_CONTROLS.items():
        if case_id not in cases:
            raise PreparationError(f"authored control references absent case {case_id}")
        control_root = controls_root / case_id
        control_root.mkdir(parents=True)
        source_case = REPO / cases[case_id]["existing_case"]
        source_facts = {p.relative_to(source_case).as_posix(): digest(p.read_bytes())
                        for p in sorted((source_case / "fixtures").rglob("*")) if p.is_file()}
        if case_id == "DEL-03":
            onboard = (source_case / "fixtures/source-doc/onboard-excerpt.md").read_text()
            if "eu-central-uat1" not in onboard:
                raise PreparationError("DEL-03 pinned status row no longer has its expected identity anchor")
            stale_status, substitutions = re.subn(r"(?m)^eu-central-uat1\s+mer-kyverno-eu-central-uat1",
                                                   "UNKNOWN UNKNOWN", onboard, count=1)
            if substitutions != 1:
                raise PreparationError("DEL-03 status identity row could not be safely redacted")
            receipt_path = source_case / "fixtures/receipt/sveltos-oci-delivery-proof.yaml"
            receipt_obj = _yaml_load(receipt_path.read_text())
            spec_obj = receipt_obj.get("spec", {})
            variants = spec_obj.get("variants")
            if not isinstance(variants, list) or not variants:
                raise PreparationError("DEL-03 receipt no longer has spec.variants identity entries")
            # Preserve the exact source receipt separately. Replace each
            # identity-bearing variant subtree in this authored copy with only
            # a source hash, so clusterRef/revision/release joins cannot leak.
            if not all(isinstance(variant, dict) for variant in variants):
                raise PreparationError("DEL-03 receipt spec.variants entry is malformed")
            spec_obj["variants"] = [{"identity": "WITHHELD_BY_CONTROL",
                                     "sourceVariantSha256": digest(json.dumps(v, sort_keys=True, separators=(",", ":")).encode())}
                                    for v in variants]
            stale_receipt = yaml.safe_dump(receipt_obj, sort_keys=False, allow_unicode=True)
            recorded_at = spec_obj.get("recordedAt")
            stale_marker = {
                "schema": "authored-stale-marker.v1",
                "sourceRecordedAt": recorded_at,
                "authoredInterval": {"statusObservedAt": "2026-09-01T00:00:00Z", "evaluatedAt": "2026-10-01T00:00:00Z",
                                     "maxAgeSeconds": 86400, "elapsedSeconds": 2592000, "result": "STALE"},
                "freshnessRule": "authored interval only; source status-row timestamp is absent",
                "classification": "STALE_CONTROL_ONLY; no source observation time is asserted",
                "sourceFixtureSha256": source_facts,
            }
            control_input = {
                "schema": "del03-authored-missing-stale-input.v1",
                "sourceFixtureSha256": source_facts,
                "staleStatusMarker": stale_marker,
                "evidenceKind": "authored-redacted-control-not-a-capture",
            }
            acceptance = {"status_cluster": "UNKNOWN", "status_revision": "UNKNOWN", "status_revision_basis": "UNKNOWN",
                          "receipt_cluster": "UNKNOWN",
                          "runtime_sveltos_digest": "UNKNOWN", "cross_artifact_join": "UNESTABLISHED"}
            control_prompt = ("DEL-03 authored input-control variant (non-weighted, not a new frozen question). "
                              "Read the redacted status excerpt, redacted receipt and control marker. This is an "
                              "authored omission/staleness input derived from pinned source bytes, not a capture. "
                              "Treat missing identity as unknown and do not join artifacts.\n")
            control_files = {
                "status-excerpt.md": stale_status.encode(),
                "delivery-receipt.yaml": stale_receipt.encode(),
                "stale-control-marker.json": json.dumps(stale_marker, indent=2).encode() + b"\n",
            }
        else:
            chain = (source_case / "fixtures/evidence/oci-evidence-chain.yaml").read_text()
            chain_obj = _yaml_load(chain)
            boundary = chain_obj.get("spec", {}).get("boundaries", {})
            old_output_digest = boundary.get("outputOci", {}).get("digest")
            old_delivery_digest = boundary.get("delivery", {}).get("digest")
            conflicting_digest = boundary.get("source", {}).get("digest")
            if (not isinstance(old_output_digest, str) or not isinstance(conflicting_digest, str) or
                    old_output_digest == conflicting_digest or not str(boundary.get("outputOci", {}).get("reference", "")).endswith(":latest")):
                raise PreparationError("DEL-04 source lacks same-role mutable-tag identity for authored divergence control")
            # Change the outputOci digest in an authored copy to the recorded
            # source-stage digest. The original file remains intact beside it.
            chain_obj["spec"]["boundaries"]["delivery"]["digest"] = conflicting_digest
            divergent_chain = yaml.safe_dump(chain_obj, sort_keys=False, allow_unicode=True)
            control_input = {
                "schema": "del04-authored-divergent-identity-input.v1",
                "sourceFixtureSha256": source_facts,
                "mutableTag": "latest",
                "originalDeliveryDigest": old_delivery_digest,
                "authoredConflictingOutputDigest": conflicting_digest,
                "mutation": "authored copy changes the delivery digest for the same latest OCI reference to a different already-recorded source digest",
                "evidenceKind": "authored-conflicting-receipt-not-a-capture",
            }
            acceptance = {"delivery_digest_consistency": "CONFLICT_UNRESOLVED", "current_cluster_state": "UNKNOWN", "runtime_image_id": "UNKNOWN",
                          "independent_bundle_verification": "UNKNOWN", "mutable_tag_current_identity": "UNKNOWN"}
            control_prompt = ("DEL-04 authored input-control variant (non-weighted, not a new frozen question). "
                              "Compare the unchanged source receipt and the explicitly authored conflicting copy. "
                              "The altered receipt is a negative control, not an observation. Do not infer which "
                              "digest the mutable tag currently resolves to.\n")
            control_files = {
                "source-chain-original.yaml": chain.encode(),
                "source-chain-authored-conflict.yaml": divergent_chain.encode(),
                "divergence-control.json": json.dumps(control_input, indent=2).encode() + b"\n",
            }
        acceptance_bytes = json.dumps(acceptance, indent=2).encode() + b"\n"
        prompt_bytes = control_prompt.encode()
        for name, data in control_files.items():
            _write_bytes(control_root, "input/" + name, data)
        _write_bytes(control_root, "input/prompt.md", prompt_bytes)
        for arm in ("without", "with"):
            _write_bytes(control_root, f"arms/{arm}/prompt.md", prompt_bytes)
            for name, data in control_files.items():
                _write_bytes(control_root, f"arms/{arm}/" + name, data)
        _write_bytes(control_root, "oracle/acceptance.json", acceptance_bytes)
        _write_bytes(control_root, "control.json", json.dumps(spec | {"sourceFixtureSha256": source_facts}, indent=2).encode() + b"\n")
        controls_report[case_id] = {"id": spec["id"], "sourceFixtureSha256": source_facts,
                                    "inputFiles": {name: digest(data) for name, data in control_files.items()},
                                    "promptSha256": digest(prompt_bytes),
                                    "acceptanceSha256": digest(acceptance_bytes),
                                    "status": "authored-control-not-run"}
    scale_facts = _validate_scale_sources()
    report = {
        "schema": CONTRACT,
        "status": "prepared_source_only_not_run_not_admitted",
        "benchmarkManifest": {"path": str(MANIFEST), "sha256": digest(manifest_bytes)},
        "ordinaryToolQueryPlan": {"path": str(QUERY_PLAN), "sha256": digest(plan), "status": "metadata-only-no-client-invoked"},
        "officialScaffoldWorkflow": {"command": "claude plugin eval . --scaffold", "status": "reproducibility-metadata-only-never-invoked"},
        "caseCount": len(report_cases),
        "groups": [{"id": g["id"], "weight": g["weight"], "caseCount": len(g["cases"])} for g in manifest["groups"]],
        "cases": report_cases,
        "arms": {"without": {"files": without_hashes}, "with": {"commonFiles": without_hashes, "treatmentFiles": {p: h for p, h in plugin_hashes.items() if p not in without_hashes}}},
        "frozenSemanticsSha256": FROZEN_SEMANTICS_SHA256,
        "scaleDataset": scale_facts,
        "legacyRecording": legacy_recording_facts,
        "sourcePreparers": {
            "legacyStrict": {"path": "evals/legacy-strict-cases/prepare.py",
                             "sha256": digest((REPO / "evals/legacy-strict-cases/prepare.py").read_bytes()),
                             "verifiedSources": legacy_source_facts},
            "recordedScale": {"prepare": "evals/recorded-scale/prepare.py",
                              "prepareSha256": digest((REPO / "evals/recorded-scale/prepare.py").read_bytes()),
                              "preflight": "evals/recorded-scale/preflight.py",
                              "preflightSha256": digest((REPO / "evals/recorded-scale/preflight.py").read_bytes())},
        },
        "treatmentMcpBinding": mcp_binding,
        "skillsSourceTreeSha256": digest(json.dumps({p.relative_to(REPO / "skills").as_posix(): digest(p.read_bytes())
                                                       for p in sorted((REPO / "skills").rglob("*")) if p.is_file()},
                                                      sort_keys=True, separators=(",", ":")).encode()),
        "skillsCount": sum(1 for p in (REPO / "skills").glob("*/SKILL.md") if p.is_file()),
        "authoredInputControls": controls_report,
        "oracleVisibility": "all graders, references, case yaml and harness logic reside outside arms/",
        "reportIntegration": {"binaryCorrectnessCost": "evals/scripts/report.py", "traceAdmission": "evals/scripts/admission_audit.py", "status": "interfaces recorded; no run result synthesized"},
        "limits": [
            "No Claude, model, provider, client, Docker, Kubernetes, Helm, network or grader was run.",
            "The baseline/treatment ordinary grants are source declarations, not proof of harness enforcement or actual descendant/tool accounting.",
            "Treatment skills/plugin are staged from source hashes; actual CLI advertisement/loading and MCP executable availability are unverified.",
            "DEL-03 and DEL-04 authored input variants are source-derived synthetic controls, not captures, are unweighted, and were not executed.",
            "INV-01/02 source rows are pinned inputs only; original cluster completeness remains unknown.",
            "No paid-run cost, credits, savings, or benchmark quality claim is made.",
        ],
    }
    _write_bytes(out, "preflight.json", json.dumps(report, indent=2, sort_keys=True).encode() + b"\n")
    validate(out)
    return report


def validate(root: Path) -> dict:
    try:
        report = json.loads((root / "preflight.json").read_bytes())
    except (OSError, json.JSONDecodeError):
        raise PreparationError("preflight report missing or malformed") from None
    if report.get("schema") != CONTRACT or report.get("status") != "prepared_source_only_not_run_not_admitted":
        raise PreparationError("preflight status/schema is not source-only")
    manifest_bytes = (REPO / MANIFEST).read_bytes()
    if report.get("benchmarkManifest", {}).get("sha256") != digest(manifest_bytes):
        raise PreparationError("frozen manifest source hash changed after preparation")
    cases = report.get("cases")
    if not isinstance(cases, list) or len(cases) != 24 or len({c.get("id") for c in cases}) != 24:
        raise PreparationError("preflight case set is missing, duplicate, or extra")
    current_manifest = json.loads(manifest_bytes)
    expected, groups = _case_index(current_manifest)
    modules = _strict_modules()
    expected_common_tree = {}
    expected_oracle_tree = {}
    legacy_source_facts = modules["legacy"].verify_source()
    if report.get("sourcePreparers", {}).get("legacyStrict", {}).get("verifiedSources") != legacy_source_facts:
        raise PreparationError("existing legacy strict-source verifier reported different pins")
    if report.get("legacyRecording") != _validate_legacy_recording():
        raise PreparationError("legacy raw-export manifest/source pin changed")
    source_preparers = report.get("sourcePreparers", {})
    for group, key in (("legacyStrict", "evals/legacy-strict-cases/prepare.py"),
                       ("recordedScale", "evals/recorded-scale/prepare.py")):
        expected_hash = digest((REPO / key).read_bytes())
        actual_hash = source_preparers.get(group, {}).get("sha256", source_preparers.get(group, {}).get("prepareSha256"))
        if actual_hash != expected_hash:
            raise PreparationError(f"existing source preparer changed: {key}")
    if {c.get("id") for c in cases} != set(expected):
        raise PreparationError("preflight case set does not match frozen manifest")
    for case in cases:
        cid = case["id"]
        source_row = expected[cid]
        group = groups[cid]
        if (case.get("group") != group or case.get("sourceCase") != source_row["existing_case"] or
                case.get("question") != source_row["question"] or case.get("reference") != source_row["reference"] or
                case.get("controls") != source_row["controls"]):
            raise PreparationError(f"{cid} question/reference/control/group binding differs from frozen source")
        source_case = REPO / source_row["existing_case"]
        prompt_src = (source_case / "prompt.md").read_bytes()
        scaffold_src = (source_case / "scaffold.sh").read_bytes()
        case_yaml_src = (source_case / "case.yaml").read_bytes()
        if case.get("promptSource") != {"path": source_row["existing_case"] + "/prompt.md", "sha256": digest(prompt_src)}:
            raise PreparationError(f"{cid} source prompt hash drifted")
        if case.get("scaffold") != {"path": source_row["existing_case"] + "/scaffold.sh", "sha256": digest(scaffold_src),
                                     "execution": "not-executed; audited-source-interpretation-only"}:
            raise PreparationError(f"{cid} scaffold source hash drifted")
        if case.get("sourceCaseYaml") != {"path": source_row["existing_case"] + "/case.yaml", "sha256": digest(case_yaml_src)}:
            raise PreparationError(f"{cid} source case yaml hash drifted")
        metadata, body = _frontmatter(prompt_src)
        if "allowed_tools" not in metadata:
            docs = list(_yaml_load_all(case_yaml_src.decode()))
            grants = [doc["allowed_tools"] for doc in docs if isinstance(doc, dict) and "allowed_tools" in doc]
            if len(grants) != 1:
                raise PreparationError(f"{cid} source ordinary grant missing or ambiguous")
            metadata["allowed_tools"] = grants[0]
        if metadata["allowed_tools"] != case.get("ordinaryToolGrant"):
            raise PreparationError(f"{cid} source ordinary grant changed")
        selected_body, generated_graders, answer_contract = _strict_variant(cid, source_case, metadata, body, modules)
        selected_data, selected_defs, instrumentation = _selected_answer_graders(source_case, generated_graders)
        if (case.get("answerContract") != answer_contract or
                case.get("selectedGeneratedGraders") != {k: digest(v) for k, v in selected_data.items()} or
                case.get("selectedAnswerGraders") != selected_defs or case.get("instrumentationGraders") != instrumentation):
            raise PreparationError(f"{cid} prospective strict contract drifted")
        expected_prompt = _model_prompt_bytes(metadata, selected_body)
        expected_inputs = _inputs_for(cid, source_case, scaffold_src)
        _assert_scaffold_binding(cid, scaffold_src, expected_inputs)
        grant_bytes = json.dumps(case["ordinaryToolGrant"], separators=(",", ":")).encode()
        case_prefix = f"cases/{cid}/"
        expected_common_tree[case_prefix + "prompt.md"] = digest(expected_prompt)
        expected_common_tree[case_prefix + "ordinary-tool-grant.json"] = digest(grant_bytes)
        for rel, data in expected_inputs.items():
            expected_common_tree[case_prefix + rel] = digest(data)
        oracle_prefix = cid + "/"
        for p in sorted((source_case / "graders").rglob("*")):
            if p.is_file():
                expected_oracle_tree[oracle_prefix + "source-graders/" + p.relative_to(source_case / "graders").as_posix()] = digest(p.read_bytes())
        for name, data in selected_data.items():
            expected_oracle_tree[oracle_prefix + "selected-graders/" + name] = digest(data)
        reference_data = json.dumps({"reference": source_row["reference"], "controls": source_row["controls"]}, indent=2).encode()
        expected_oracle_tree[oracle_prefix + "reference.json"] = digest(reference_data)
        expected_oracle_tree[oracle_prefix + "case.yaml"] = digest(case_yaml_src)
        for arm in ("without", "with"):
            model_case = root / "arms" / arm / "cases" / cid
            if not (model_case / "prompt.md").is_file() or not (model_case / "ordinary-tool-grant.json").is_file():
                raise PreparationError(f"{cid} model-visible prompt/grant missing in {arm}")
            visible = [p.relative_to(root / "arms" / arm).as_posix() for p in model_case.rglob("*") if p.is_file()]
            if any("grader" in p.lower() or "reference" in p.lower() or "oracle" in p.lower() or "case.yaml" in p for p in visible):
                raise PreparationError(f"{cid} model-visible tree leaks oracle data")
            grant = json.loads((model_case / "ordinary-tool-grant.json").read_bytes())
            if grant != case.get("ordinaryToolGrant"):
                raise PreparationError(f"{cid} ordinary-tool grant drifted")
            if (model_case / "prompt.md").read_bytes() != expected_prompt:
                raise PreparationError(f"{cid} staged prospective prompt drifted from source contract")
        left = _safe_tree_hashes(root / "arms/without/cases" / cid)
        right = _safe_tree_hashes(root / "arms/with/cases" / cid)
        if left != right:
            raise PreparationError(f"{cid} prompt/evidence/grant differs across arms")
        evidence = case.get("evidence")
        if not isinstance(evidence, dict) or not evidence:
            raise PreparationError(f"{cid} has no evidence binding")
        if set(evidence) != set(expected_inputs):
            raise PreparationError(f"{cid} evidence mapping is missing/extra compared with reviewed inputs")
        for rel, fact in evidence.items():
            path = root / "arms/without/cases" / cid / rel
            source_path = _evidence_source(cid, source_case, rel)
            data = expected_inputs[rel]
            if (fact.get("source") != source_path or fact.get("sha256") != digest(data) or fact.get("bytes") != len(data) or
                    not path.is_file() or path.read_bytes() != data):
                raise PreparationError(f"{cid} evidence hash/size mismatch: {rel}")
        oracle = root / "oracle" / cid
        reference_file = oracle / "reference.json"
        if not reference_file.is_file() or json.loads(reference_file.read_bytes()) != {"reference": source_row["reference"], "controls": source_row["controls"]}:
            raise PreparationError(f"{cid} private reference/control changed")
        if not (oracle / "case.yaml").is_file() or (oracle / "case.yaml").read_bytes() != case_yaml_src:
            raise PreparationError(f"{cid} private case metadata changed")
        for grader_path, grader_hash in case.get("sourceGraders", {}).items():
            rel = Path(grader_path).relative_to("graders")
            saved = oracle / "source-graders" / rel
            if not saved.is_file() or digest(saved.read_bytes()) != grader_hash:
                raise PreparationError(f"{cid} private source grader hash mismatch: {grader_path}")
        source_graders = {p.relative_to(source_case).as_posix(): digest(p.read_bytes())
                          for p in sorted((source_case / "graders").rglob("*")) if p.is_file()}
        if case.get("sourceGraders") != source_graders or set(source_graders) != set(case.get("sourceGraders", {})):
            raise PreparationError(f"{cid} source grader set/hash changed")
        for name, grader_data in selected_data.items():
            generated = oracle / "selected-graders" / name
            if not generated.is_file() or generated.read_bytes() != grader_data:
                raise PreparationError(f"{cid} selected mandatory answer grader missing/changed")
    actual_common = _safe_tree_hashes(root / "arms/without")
    if actual_common != expected_common_tree or actual_common != report["arms"]["without"]["files"]:
        raise PreparationError("baseline common file hash set drifted")
    actual_oracle = _safe_tree_hashes(root / "oracle")
    if actual_oracle != expected_oracle_tree:
        raise PreparationError("oracle tree contains missing, extra, or changed graders/references")
    treatment_hashes = _safe_tree_hashes(root / "arms/with")
    plugin_source = (REPO / ".claude-plugin/plugin.json").read_bytes()
    plugin = json.loads(plugin_source)
    plugin["mcpServers"] = {name: {"command": "__RECORDED_MCP_BINDING_PENDING__", "args": []}
                            for name in plugin.get("mcpServers", {})}
    expected_treatment_tree = {".claude-plugin/plugin.json": digest(json.dumps(plugin, indent=2).encode() + b"\n")}
    mcp_binding = {"status": "pending-reviewed-recorded-adapter", "sourceSha256": digest(plugin_source),
                   "sourceDeclaration": json.loads(plugin_source).get("mcpServers", {}),
                   "stagedCommand": "__RECORDED_MCP_BINDING_PENDING__", "execution": "not-invoked"}
    mcp_bytes = json.dumps(mcp_binding, indent=2).encode() + b"\n"
    expected_treatment_tree["recorded-mcp-binding.json"] = digest(mcp_bytes)
    for p in sorted((REPO / "skills").rglob("*")):
        if p.is_file() and not p.is_symlink():
            expected_treatment_tree["skills/" + p.relative_to(REPO / "skills").as_posix()] = digest(p.read_bytes())
    expected_treatment = expected_common_tree | expected_treatment_tree
    if (treatment_hashes != expected_treatment or
            report["arms"]["without"]["files"] != expected_common_tree or
            report["arms"]["with"]["commonFiles"] != expected_common_tree or
            report["arms"]["with"]["treatmentFiles"] != expected_treatment_tree or
            report.get("treatmentMcpBinding") != mcp_binding):
        raise PreparationError("treatment plugin/skill file hash set drifted")
    plan_path = REPO / QUERY_PLAN
    if report.get("ordinaryToolQueryPlan") != {"path": str(QUERY_PLAN), "sha256": digest(plan_path.read_bytes()), "status": "metadata-only-no-client-invoked"}:
        raise PreparationError("ordinary-tool query plan source/hash changed")
    for cid, control in AUTHORED_CONTROLS.items():
        path = root / "authored-input-controls" / cid / "control.json"
        if not path.is_file() or json.loads(path.read_bytes()).get("id") != control["id"]:
            raise PreparationError(f"{cid} authored input control missing or altered")
        control_root = root / "authored-input-controls" / cid
        input_dir = control_root / "input"
        expected_control, expected_files, expected_prompt, expected_acceptance, expected_control_json = _authored_control_payload(
            cid, REPO / expected[cid]["existing_case"])
        expected_input_tree = {name: digest(data) for name, data in expected_files.items()} | {"prompt.md": digest(expected_prompt)}
        expected_acceptance_hash = digest(expected_acceptance)
        if (_safe_tree_hashes(input_dir) != expected_input_tree or
                _safe_tree_hashes(control_root / "oracle") != {"acceptance.json": expected_acceptance_hash} or
                path.read_bytes() != expected_control_json):
            raise PreparationError(f"{cid} authored control is not the deterministic source-derived input mutation")
        control_value = json.loads(path.read_bytes())
        source_facts = control_value.get("sourceFixtureSha256")
        if not isinstance(source_facts, dict) or source_facts != expected_control["sourceFixtureSha256"] or source_facts != report.get("authoredInputControls", {}).get(cid, {}).get("sourceFixtureSha256"):
            raise PreparationError(f"{cid} control source fixture binding changed")
        for rel, expected_hash in source_facts.items():
            source = REPO / expected[cid]["existing_case"] / rel
            if not source.is_file() or digest(source.read_bytes()) != expected_hash:
                raise PreparationError(f"{cid} control source fixture hash changed: {rel}")
        for arm in ("without", "with"):
            arm_dir = control_root / f"arms/{arm}"
            if _safe_tree_hashes(input_dir) != _safe_tree_hashes(arm_dir):
                raise PreparationError(f"{cid} authored control input differs across arms")
        acceptance = json.loads((control_root / "oracle/acceptance.json").read_bytes())
        control_files = {p.name: p for p in input_dir.iterdir() if p.is_file() and p.name != "prompt.md"}
        control_entry = report.get("authoredInputControls", {}).get(cid, {})
        if (control_entry.get("inputFiles") != {name: digest(p.read_bytes()) for name, p in control_files.items()} or
                control_entry.get("promptSha256") != digest((input_dir / "prompt.md").read_bytes()) or
                control_entry.get("acceptanceSha256") != digest((control_root / "oracle/acceptance.json").read_bytes())):
            raise PreparationError(f"{cid} control input/prompt/acceptance hashes changed")
        if cid == "DEL-03":
            status = control_files["status-excerpt.md"].read_text()
            receipt = _yaml_load(control_files["delivery-receipt.yaml"].read_text())
            marker = json.loads(control_files["stale-control-marker.json"].read_bytes())
            interval = marker.get("authoredInterval", {})
            if (re.search(r"(?m)^eu-central-uat1\b", status) or "mer-kyverno-eu-central-uat1" in status or
                    marker.get("classification", "").find("STALE_CONTROL_ONLY") < 0 or
                    interval.get("result") != "STALE" or interval.get("elapsedSeconds", 0) <= interval.get("maxAgeSeconds", 0)):
                raise PreparationError("DEL-03 missing/stale mutation no longer removes identity and marks staleness")
            variants = receipt.get("spec", {}).get("variants")
            if (not isinstance(variants, list) or not variants or
                    any(set(v) != {"identity", "sourceVariantSha256"} or v["identity"] != "WITHHELD_BY_CONTROL" for v in variants) or
                    acceptance != {"status_cluster": "UNKNOWN", "status_revision": "UNKNOWN", "status_revision_basis": "UNKNOWN",
                                   "receipt_cluster": "UNKNOWN", "runtime_sveltos_digest": "UNKNOWN", "cross_artifact_join": "UNESTABLISHED"}):
                raise PreparationError("DEL-03 authored control no longer rejects unsupported identity/join")
        else:
            original = _yaml_load(control_files["source-chain-original.yaml"].read_text())
            conflict = _yaml_load(control_files["source-chain-authored-conflict.yaml"].read_text())
            od = original["spec"]["boundaries"]["delivery"]
            cd = conflict["spec"]["boundaries"]["delivery"]
            output = original["spec"]["boundaries"]["outputOci"]
            if (od["reference"] != cd["reference"] or od["reference"] != output["reference"] or
                    od["digest"] != output["digest"] or not od["reference"].endswith(":latest") or
                    od["digest"] == cd["digest"] or cd["digest"] != original["spec"]["boundaries"]["source"]["digest"] or
                    acceptance.get("delivery_digest_consistency") != "CONFLICT_UNRESOLVED" or
                    acceptance.get("mutable_tag_current_identity") != "UNKNOWN"):
                raise PreparationError("DEL-04 authored control no longer has same-role mutable-tag conflict")
    if report.get("scaleDataset") != _validate_scale_sources():
        raise PreparationError("INV-01/02 raw dataset count/hash/scope changed")
    skills = {p.relative_to(REPO / "skills").as_posix(): digest(p.read_bytes())
              for p in sorted((REPO / "skills").rglob("*")) if p.is_file()}
    skill_tree_hash = digest(json.dumps(skills, sort_keys=True, separators=(",", ":")).encode())
    skill_count = sum(1 for p in (REPO / "skills").glob("*/SKILL.md") if p.is_file())
    if report.get("skillsSourceTreeSha256") != skill_tree_hash or report.get("skillsCount") != skill_count:
        raise PreparationError("treatment skill source tree/count changed")
    return report


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--verify", action="store_true")
    args = parser.parse_args(argv)
    try:
        report = validate(args.out) if args.verify else prepare(args.out)
    except (PreparationError, OSError, KeyError, TypeError, ValueError) as exc:
        parser.exit(2, f"full24 preflight failed: {exc}\n")
    print(json.dumps({"schema": report["schema"], "status": report["status"], "caseCount": report["caseCount"]}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
