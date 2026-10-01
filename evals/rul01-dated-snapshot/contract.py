"""Offline validator for the RUL-01 dated-snapshot fixture contract."""
from datetime import datetime, timezone
import hashlib
import json
import math
from pathlib import Path
import re

ROOT = Path(__file__).parent
RAW = ROOT / "fixtures" / "after-pod.json"
RECEIPT = ROOT / "fixtures" / "capture-time-receipt.json"
CLOCKS = ROOT / "fixtures" / "test-clocks.json"
EXPECTED_SHA = "f264204dc296d06590bc357691a1b95db08222f4ac68b3c38935d6ef200832c3"


class ContractError(ValueError):
    pass


def _json(data):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ContractError("duplicate JSON key")
            result[key] = value
        return result
    try:
        return json.loads(data, object_pairs_hook=pairs, parse_constant=lambda _: (_ for _ in ()).throw(ContractError("non-finite JSON number")))
    except (UnicodeDecodeError, ValueError) as exc:
        raise ContractError("invalid JSON") from exc


def _time(value):
    if not isinstance(value, str) or re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z", value) is None:
        raise ContractError("timestamp must be a string")
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ContractError("invalid timestamp") from exc
    if parsed.tzinfo is None or parsed.utcoffset() != timezone.utc.utcoffset(parsed):
        raise ContractError("timestamp must be UTC")
    return parsed


def validate(raw_bytes=None, receipt_bytes=None, clocks_bytes=None):
    """Return facts only when every byte/time/identity binding is valid."""
    raw_bytes = RAW.read_bytes() if raw_bytes is None else raw_bytes
    receipt_bytes = RECEIPT.read_bytes() if receipt_bytes is None else receipt_bytes
    clocks_bytes = CLOCKS.read_bytes() if clocks_bytes is None else clocks_bytes
    receipt, clocks, pod = _json(receipt_bytes), _json(clocks_bytes), _json(raw_bytes)
    if not isinstance(receipt, dict) or not isinstance(clocks, dict):
        raise ContractError("receipt and clock inputs must be JSON objects")
    source, request = receipt.get("source"), receipt.get("request")
    if receipt.get("schema") != "rul01.capture-time-binding.v1" or not isinstance(source, dict) or not isinstance(request, dict):
        raise ContractError("unrecognized receipt")
    scope = receipt.get("captureScope")
    if not isinstance(scope, dict) or scope.get("atomicSnapshot") is not False:
        raise ContractError("capture scope is not the recorded non-atomic observation")
    digest = hashlib.sha256(raw_bytes).hexdigest()
    if type(source.get("rawBytes")) is not int:
        raise ContractError("raw byte count must be an integer")
    if (source.get("rawFile"), source.get("rawBytes"), source.get("rawSha256")) != ("after-pod.json", len(raw_bytes), digest) or digest != EXPECTED_SHA:
        raise ContractError("raw evidence binding mismatch")
    if (source.get("captureSourceCommit"), source.get("captureHelperSha256"), source.get("sourceProvenanceSha256")) != (
        "4b113710948882eda501e14aacca2d5cec1168ae", "ec4f7f8e9d2d2ca6257abc5029f2e01b33203794a5782c22ba13895bedd31230", "472314079b74bff1a374d5e55eb44a12db2e2ffeaa189b30dc659423bc95398a"):
        raise ContractError("source provenance mismatch")
    start, end = _time(request.get("startedAt")), _time(request.get("endedAt"))
    if type(request.get("httpStatus")) is not int:
        raise ContractError("HTTP status must be an integer")
    if (request.get("phase"), request.get("method"), request.get("path"), request.get("httpStatus")) != (
        "after", "GET", "/api/v1/namespaces/pre02-node-selector/pods/scout-pre02-selector", 200):
        raise ContractError("request identity mismatch")
    precision = request.get("loggedTimestampPrecisionSeconds")
    if type(precision) is not int or precision != 1:
        raise ContractError("timestamp precision must be the recorded integer second")
    elapsed = request.get("elapsedSeconds")
    if (request.get("startedAt"), request.get("endedAt"), request.get("timeMeaning")) != (
        "2026-10-01T06:03:25Z", "2026-10-01T06:03:25Z",
        "request logging timestamps for this response; not a timestamp for an atomic multi-response snapshot"):
        raise ContractError("request timestamp binding mismatch")
    if isinstance(elapsed, bool) or not isinstance(elapsed, (int, float)):
        raise ContractError("invalid request timing")
    try:
        elapsed_seconds = float(elapsed)
    except (OverflowError, ValueError):
        raise ContractError("invalid request timing") from None
    if not math.isfinite(elapsed_seconds) or abs(elapsed_seconds - 0.01204633410088718) > 1e-12 or (end-start).total_seconds() > elapsed_seconds + 1:
        raise ContractError("invalid request timing")
    if not isinstance(pod, dict):
        raise ContractError("raw response is not a Kubernetes object")
    meta = pod.get("metadata")
    if (pod.get("apiVersion"), pod.get("kind"), meta.get("namespace") if isinstance(meta, dict) else None,
            meta.get("name") if isinstance(meta, dict) else None, meta.get("uid") if isinstance(meta, dict) else None) != (
        "v1", "Pod", "pre02-node-selector", "scout-pre02-selector", "b5fa45bb-3aa4-4ec2-aa97-c329ea564317"):
        raise ContractError("resource identity mismatch")
    status = pod.get("status")
    if not isinstance(status, dict):
        raise ContractError("raw Pod status is missing")
    conditions = status.get("conditions")
    scheduled = [c for c in conditions or [] if isinstance(c, dict) and c.get("type") == "PodScheduled"]
    if len(scheduled) != 1 or scheduled[0].get("status") != "True":
        raise ContractError("recorded scheduling condition is not uniquely true")
    creation = _time(meta.get("creationTimestamp"))
    transition = _time(scheduled[0].get("lastTransitionTime"))
    if clocks.get("schema") != "rul01.authored-test-clocks.v1" or clocks.get("classification") != "explicit authored as-of inputs; neither capture times nor current time":
        raise ContractError("invalid test-clock declaration")
    entries = clocks.get("clocks")
    if not isinstance(entries, list) or len(entries) != 2 or any(not isinstance(c, dict) for c in entries) or [c.get("id") for c in entries] != ["test_clock_1", "test_clock_2"]:
        raise ContractError("test clocks missing or reordered")
    if [c.get("asOf") for c in entries] != ["2026-10-01T06:08:25Z", "2026-10-01T06:13:25Z"]:
        raise ContractError("authored test clock binding mismatch")
    parsed = [_time(c.get("asOf")) for c in entries]
    age = [int((clock-end).total_seconds()) for clock in parsed]
    if age != [300, 600]:
        raise ContractError("test clock binding mismatch")
    return {"sha256": digest, "startedAt": start.isoformat().replace("+00:00", "Z"),
            "endedAt": end.isoformat().replace("+00:00", "Z"), "creationTimestamp": creation.isoformat().replace("+00:00", "Z"),
            "conditionTransition": transition.isoformat().replace("+00:00", "Z"), "agesSeconds": age,
            "identity": "v1|Pod|pre02-node-selector|scout-pre02-selector",
            "uid": meta["uid"], "scheduled": "POD_SCHEDULED_TRUE", "currentState": "UNKNOWN"}


def validate_or_unknown(raw_bytes=None, receipt_bytes=None, clocks_bytes=None):
    """Return explicit unknown facts instead of guessing when binding fails."""
    try:
        return {"evidence_binding": "VERIFIED", **validate(raw_bytes, receipt_bytes, clocks_bytes)}
    except (ContractError, OSError):
        return {"evidence_binding": "UNKNOWN", "identity": "UNKNOWN", "uid": "UNKNOWN",
                "scheduled": "UNKNOWN", "startedAt": "UNKNOWN", "endedAt": "UNKNOWN",
                "creationTimestamp": "UNKNOWN", "conditionTransition": "UNKNOWN",
                "agesSeconds": ["UNKNOWN", "UNKNOWN"], "sha256": "UNKNOWN", "currentState": "UNKNOWN"}


if __name__ == "__main__":
    print(json.dumps(validate(), sort_keys=True))
