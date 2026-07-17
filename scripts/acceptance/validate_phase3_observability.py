#!/usr/bin/env python3
"""Fail-closed validation for the Phase 3 observability evidence contract."""

import hashlib
import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
PATH = ROOT / "acceptance/phase3/observability-contract-v1.json"
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
    require(contract.get("contract") == "p3-observability-health-evidence.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-2uo81g", "wrong task")

    endpoint = contract.get("operatorEndpoint", {})
    require(endpoint.get("method") == "GET", "operator view is not read-only")
    require(endpoint.get("path") == "/v1/moderation/phase3-observability", "endpoint drifted")
    require(endpoint.get("authentication") == "moderation bearer with list capability", "operator authorization drifted")
    require(endpoint.get("queryParameters") == [], "selector queries entered export API")
    require(endpoint.get("cacheControl") == "no-store", "operator cache policy drifted")

    health = contract.get("publicHealth", {})
    require(len(health.get("fields", [])) == 6, "public health shape drifted")
    require("disabled optional" in health.get("disabledRule", ""), "flag-off health rule missing")
    require("never proves" in health.get("promotionRule", ""), "health claims promotion evidence")

    window = contract.get("window", {})
    require(window.get("seconds") == 86400 and window.get("rolling") is True, "window drifted")
    require("monotonic" in window.get("captureFreshnessClock", ""), "capture clock scope missing")
    require("manual campaign" in window.get("clientTimingClock", ""), "manual timing clock missing")

    groups = contract.get("metricGroups", {})
    require(len(groups.get("livePTT", [])) == 18, "live PTT metric inventory incomplete")
    require(len(groups.get("captureQuality", [])) == 17, "capture metric inventory incomplete")
    require(groups.get("e2eeMedia") == ["state", "epoch_samples", "revocation_samples", "secret_material_exposed"], "crypto public metric inventory drifted")
    require(len(groups.get("automationAttempts", [])) == 8, "automation attempt metrics incomplete")
    require(len(groups.get("automationFeature", [])) == 8, "automation feature metrics incomplete")
    require(len(groups.get("provenance", [])) == 5, "provenance inventory incomplete")

    state = contract.get("featureState", {})
    require("deferred_unavailable" in state.get("e2eeMedia", ""), "deferred E2EE state missing")
    require("automation enabled without soundboard" in state.get("invalidCombination", ""), "invalid flag combination missing")
    readiness = contract.get("readiness", {})
    require(readiness.get("promotionEvidenceReady", "").startswith("always false"), "runtime view claims promotion")
    require(readiness.get("manualStatus") == "not_run", "manual evidence was invented")
    require(len(readiness.get("missingEvidence", [])) == 6, "missing evidence inventory drifted")

    cardinality = contract.get("cardinality", {})
    require(cardinality.get("dynamicLabelsAllowed") is False, "dynamic labels allowed")
    forbidden = cardinality.get("forbidden", [])
    require(len(forbidden) >= 20 and "raw actor id" in forbidden and "cryptographic key" in forbidden and "filesystem path" in forbidden, "privacy denylist incomplete")
    privacy = contract.get("privacy", {})
    require("domain-separated" in privacy.get("environmentBinding", ""), "environment hash boundary missing")
    require("prohibited" in privacy.get("secretRule", ""), "secret rule missing")

    alerts = contract.get("alerts", [])
    require(len(alerts) == 6, "alert inventory incomplete")
    require({item.get("id") for item in alerts} == {
        "live_ptt_runtime_missing", "live_ptt_prohibited_audio_retention",
        "capture_quality_telemetry_missing", "automation_enabled_without_soundboard",
        "automation_all_scopes_emergency_disabled", "build_environment_provenance_missing",
    }, "alert identifiers drifted")

    evidence = contract.get("evidence", {})
    require("$MODERATION_LIST_TOKEN" in evidence.get("query", ""), "authenticated export recipe missing")
    require(len(evidence.get("manifestFields", [])) == 8, "archive binding inventory incomplete")
    boundary = evidence.get("manualBoundary", [])
    require(len(boundary) == 4 and "real apps" in boundary[0], "manual client boundary missing")
    require("EPIC-260714-th54l3" in boundary[1] and "not_run" in boundary[1], "manual epic boundary missing")
    require("deferred E2EE" in boundary[3], "deferred E2EE boundary missing")

    anchors = contract.get("sourceAnchors", [])
    require(len(anchors) == 6, "source anchor inventory incomplete")
    for item in anchors:
        path = ROOT / item["path"]
        require(path.is_file(), f"missing source: {item['path']}")
        require(SHA256.fullmatch(item.get("sha256", "")), "malformed source digest")
        require(hashlib.sha256(path.read_bytes()).hexdigest() == item["sha256"], f"source drift: {item['path']}")

    store_source = (ROOT / "coordinator/internal/store/phase3_observability.go").read_text(encoding="utf-8")
    require("Phase3ObservabilityWindow = 24 * time.Hour" in store_source, "rolling window drifted")
    require("GetAuthorizedPhase3AutomationObservability" in store_source, "authorized store projection missing")
    require("principal_label" not in store_source and "cue_label" not in store_source, "unbounded automation label entered projection")

    http_source = (ROOT / "coordinator/cmd/duet-coordinator/phase3_observability_http.go").read_text(encoding="utf-8")
    require("r.URL.RawQuery != \"\"" in http_source, "operator endpoint permits selector queries")
    require("CredentialTokenHash" not in http_source, "credential hash entered serialization source")
    require('State: "deferred_unavailable"' in http_source, "E2EE status is not honest")
    require('PromotionEvidenceReady: false' in http_source, "runtime view can claim promotion")
    require('w.Header().Set("Cache-Control", "no-store")' in http_source, "no-store header missing")

    routes = (ROOT / "coordinator/cmd/duet-coordinator/onboarding.go").read_text(encoding="utf-8")
    require(endpoint["path"] in routes, "operator route missing")
    main_source = (ROOT / "coordinator/cmd/duet-coordinator/main.go").read_text(encoding="utf-8")
    require("addPhase3Health" in main_source, "public Phase 3 health integration missing")


if __name__ == "__main__":
    data = load()
    validate(data)
    print(json.dumps({
        "alerts": 6,
        "anchors": 6,
        "contract": data["contract"],
        "dynamicLabels": False,
        "manualStatus": "not_run",
        "status": "pass",
    }, sort_keys=True))
