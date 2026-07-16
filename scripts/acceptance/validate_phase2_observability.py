#!/usr/bin/env python3
"""Fail-closed validation for the Phase 2 observability contract."""

import hashlib
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[2]
PATH = ROOT / "acceptance/phase2/observability-contract-v1.json"
SHA256 = re.compile(r"[0-9a-f]{64}")


class ContractError(ValueError):
    pass


def require(value, message):
    if not value:
        raise ContractError(message)


def load(path=PATH):
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract):
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p2-observability-quota-view.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-qi81vf", "wrong task")
    endpoint = contract.get("operatorEndpoint", {})
    require(endpoint.get("method") == "GET", "operator view is not read-only")
    require(endpoint.get("path") == "/v1/moderation/phase2-observability", "endpoint drifted")
    require(endpoint.get("queryParameters") == [], "tenant selectors entered export API")
    health = contract.get("publicHealth", {})
    require("disabled Phase 2 dependency" in health.get("disabledRule", ""), "flag-off rule missing")
    require(len(health.get("phase2Fields", [])) == 4, "public Phase 2 health shape drifted")
    window = contract.get("window", {})
    require(window.get("seconds") == 86400 and window.get("rolling") is True, "window drifted")
    require(window.get("serverTimingClock") == "coordinator wall-clock Unix milliseconds; negative samples are excluded", "server clock drifted")
    require(window.get("clientTimingClock") == "process monotonic clock", "client clock drifted")
    groups = contract.get("metricGroups", {})
    require(len(groups.get("accounting", [])) == 14, "canonical accounting inventory incomplete")
    require(len(groups.get("processing", [])) == 7 and "processor_failures_24h" in groups["processing"], "processing inventory incomplete")
    require(groups.get("timing") == ["release_to_ready", "track_start", "start_skew", "seek_to_audio"], "timing inventory drifted")
    require("inbox_reasons" in groups.get("delivery", []), "offline reasons missing")
    cardinality = contract.get("cardinality", {})
    require(cardinality.get("dynamicLabelsAllowed") is False, "dynamic labels allowed")
    forbidden = cardinality.get("forbidden", [])
    require(len(forbidden) >= 12 and "raw actor id" in forbidden and "filename" in forbidden and "bearer token" in forbidden, "privacy denylist incomplete")
    require(len(contract.get("alerts", [])) == 6, "alert inventory incomplete")
    evidence = contract.get("evidence", {})
    require("$MODERATION_LIST_TOKEN" in evidence.get("query", ""), "authenticated export recipe missing")
    boundary = evidence.get("manualBoundary", [])
    require(len(boundary) == 3 and "seek-to-audio" in boundary[0], "manual timing boundary missing")
    require("no-go" in boundary[2], "codec no-go boundary missing")
    anchors = contract.get("sourceAnchors", [])
    require(len(anchors) == 6, "source anchor inventory incomplete")
    for item in anchors:
        path = ROOT / item["path"]
        require(path.is_file(), f"missing source: {item['path']}")
        require(SHA256.fullmatch(item.get("sha256", "")), "malformed source digest")
        require(hashlib.sha256(path.read_bytes()).hexdigest() == item["sha256"], f"source drift: {item['path']}")
    store_source = (ROOT / "coordinator/internal/store/phase2_observability.go").read_text(encoding="utf-8")
    require("streamAccountingSnapshot(q, now)" in store_source, "parallel accounting replaced canonical snapshot")
    require("client_evidence_required" in store_source, "missing honest client evidence boundary")
    require("Phase2ObservabilityWindow = 24 * time.Hour" in store_source, "rolling window drifted")
    http_source = (ROOT / "coordinator/cmd/duet-coordinator/stream_accounting_http.go").read_text(encoding="utf-8")
    require("r.URL.RawQuery != \"\"" in http_source, "operator endpoint permits selector queries")
    require("applyPhase2RuntimeReadiness" in http_source, "runtime readiness integration missing")
    require("st.Phase2HealthSnapshot(now)" in http_source, "public health uses full observability query")
    routes = (ROOT / "coordinator/cmd/duet-coordinator/onboarding.go").read_text(encoding="utf-8")
    require(endpoint["path"] in routes, "operator route missing")


if __name__ == "__main__":
    data = load()
    validate(data)
    print(json.dumps({"contract": data["contract"], "anchors": 6, "metricGroups": 6,
                      "dynamicLabels": False, "status": "pass"}, sort_keys=True))
