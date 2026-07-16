#!/usr/bin/env python3

from __future__ import annotations

import copy
import hashlib
import importlib.util
import json
import pathlib
import sys
import tempfile
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "phase2_gate_matrix", ROOT / "scripts" / "validate_phase2_gate_matrix.py",
)
assert SPEC and SPEC.loader
matrix = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(matrix)
sys.path.insert(0, str(ROOT / "scripts"))
CAMPAIGN_SPEC = importlib.util.spec_from_file_location(
    "phase2_campaign", ROOT / "scripts" / "validate_phase2_campaign.py",
)
assert CAMPAIGN_SPEC and CAMPAIGN_SPEC.loader
campaign_validator = importlib.util.module_from_spec(CAMPAIGN_SPEC)
CAMPAIGN_SPEC.loader.exec_module(campaign_validator)


class Phase2GateMatrixTests(unittest.TestCase):
    def test_frozen_matrix_is_complete_and_repository_anchored(self):
        matrix.validate(matrix.load())

    def test_contract_cannot_claim_execution(self):
        value = copy.deepcopy(matrix.load())
        value["executionStatus"] = "passed"
        with self.assertRaises(matrix.GateMatrixError):
            matrix.validate(value)

    def test_codec_no_go_cannot_be_bypassed(self):
        value = copy.deepcopy(matrix.load())
        value["productionGate"]["streamedTracksActivationAllowed"] = True
        with self.assertRaises(matrix.GateMatrixError):
            matrix.validate(value)

    def test_timing_threshold_cannot_be_lowered(self):
        value = copy.deepcopy(matrix.load())
        value["gates"]["20.5-track-start"]["threshold"]["limit"] = 6000
        with self.assertRaises(matrix.GateMatrixError):
            matrix.validate(value)

    def test_sample_count_cannot_be_reduced(self):
        value = copy.deepcopy(matrix.load())
        value["samplePlan"]["measuredSamplesPerTimedGroup"] = 10
        with self.assertRaises(matrix.GateMatrixError):
            matrix.validate(value)

    def test_synthetic_result_cannot_become_manual_pass(self):
        value = copy.deepcopy(matrix.load())
        value["gates"]["B2"]["status"] = "pass"
        with self.assertRaises(matrix.GateMatrixError):
            matrix.validate(value)

    def test_manual_task_cannot_leave_manual_epic(self):
        value = copy.deepcopy(matrix.load())
        value["gates"]["B1"]["manualTask"] = "TASK-260712-14rxuk"
        with self.assertRaises(matrix.GateMatrixError):
            matrix.validate(value)

    def test_privacy_denylist_cannot_drop_audio(self):
        value = copy.deepcopy(matrix.load())
        value["privacy"]["forbiddenArtifacts"].remove("audio bytes")
        with self.assertRaises(matrix.GateMatrixError):
            matrix.validate(value)

    def test_beta_cannot_allow_critical_incident(self):
        value = copy.deepcopy(matrix.load())
        value["samplePlan"]["beta"]["criticalIncidentsAllowed"] = 1
        with self.assertRaises(matrix.GateMatrixError):
            matrix.validate(value)

    def test_source_hash_drift_is_rejected(self):
        value = copy.deepcopy(matrix.load())
        value["sourceAnchors"][0]["sha256"] = "0" * 64
        with self.assertRaises(matrix.GateMatrixError):
            matrix.validate(value)

    def test_campaign_pass_requires_bounded_clock_and_bound_sanitization(self):
        with tempfile.TemporaryDirectory() as directory:
            campaign = pathlib.Path(directory) / "p2-20260716T120000Z-e66099448481"
            result_dir = campaign / "B5" / "explicit" / "run-001"
            result_dir.mkdir(parents=True)
            report = campaign / "sanitization-report.json"
            report.write_text('{"status":"pass"}\n', encoding="utf-8")
            report_hash = hashlib.sha256(report.read_bytes()).hexdigest()
            result = json.loads((ROOT / "acceptance/phase2/result-template-v1.json").read_text())
            result.update({
                "gateContractSha256": hashlib.sha256(matrix.MATRIX.read_bytes()).hexdigest(),
                "campaignId": campaign.name, "gateId": "B5", "status": "pass",
                "startedAt": "2026-07-16T12:00:00Z", "finishedAt": "2026-07-16T12:10:00Z",
                "operator": "Ivan Oparin", "samples": [{"case": 1, "result": "pass"}],
                "sanitizationReportSha256": report_hash,
                "artifacts": [{"path": "sanitization-report.json", "bytes": report.stat().st_size,
                               "sha256": report_hash}],
            })
            result["provenance"] = {
                "rootGitCommit": "e6609944848150c9f540582643562df409017dff",
                "coordinatorSha256": "1" * 64, "windowsMSIXSha256": "2" * 64,
                "macOSAppSha256": "3" * 64, "configurationSha256": "4" * 64,
                "fixtureLockSha256": "5" * 64,
            }
            result["environment"] = {
                "pairingOrTopology": "windows_macos", "nodes": ["windows-a", "macos-a"],
                "clockSyncSource": "recorded-ntp", "clockOffsetBeforeMS": 4,
                "clockOffsetAfterMS": -5, "networkProfile": "recorded-sufficient-network",
            }
            (result_dir / "result.json").write_text(json.dumps(result), encoding="utf-8")
            summary = campaign_validator.validate_campaign(matrix.MATRIX, campaign)
            self.assertEqual(summary, {"results": 1, "passed": 1})
            result["environment"]["clockOffsetAfterMS"] = 11
            (result_dir / "result.json").write_text(json.dumps(result), encoding="utf-8")
            with self.assertRaises(campaign_validator.CampaignError):
                campaign_validator.validate_campaign(matrix.MATRIX, campaign)


if __name__ == "__main__":
    unittest.main()
