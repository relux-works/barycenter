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
sys.path.insert(0, str(ROOT / "scripts"))
SPEC = importlib.util.spec_from_file_location(
    "phase3_gate_matrix_validator", ROOT / "scripts/validate_phase3_gate_matrix.py"
)
validator = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
sys.modules[SPEC.name] = validator
SPEC.loader.exec_module(validator)

CAMPAIGN_SPEC = importlib.util.spec_from_file_location(
    "phase3_campaign_validator", ROOT / "scripts/validate_phase3_campaign.py"
)
campaign_validator = importlib.util.module_from_spec(CAMPAIGN_SPEC)
assert CAMPAIGN_SPEC.loader
sys.modules[CAMPAIGN_SPEC.name] = campaign_validator
CAMPAIGN_SPEC.loader.exec_module(campaign_validator)


class Phase3GateMatrixTests(unittest.TestCase):
    def setUp(self) -> None:
        self.matrix = validator.load()

    def test_frozen_contract_passes_without_execution_claim(self):
        validator.validate(self.matrix)
        self.assertEqual("frozen-not-executed", self.matrix["executionStatus"])
        self.assertEqual(16, len(self.matrix["featureFlagMatrix"]))
        self.assertTrue(all("pass" not in gate["status"] for gate in self.matrix["gates"].values()))

    def test_rejects_source_drift_and_false_gate_pass(self):
        changed = copy.deepcopy(self.matrix)
        changed["sourceAnchors"][0]["sha256"] = "0" * 64
        with self.assertRaisesRegex(validator.Phase3GateMatrixError, "source anchor drift"):
            validator.validate(changed)

        changed = copy.deepcopy(self.matrix)
        changed["gates"]["C1"]["status"] = "pass"
        with self.assertRaisesRegex(validator.Phase3GateMatrixError, "falsely passed"):
            validator.validate(changed)

    def test_rejects_missing_or_misclassified_flag_permutation(self):
        changed = copy.deepcopy(self.matrix)
        changed["featureFlagMatrix"].pop()
        with self.assertRaisesRegex(validator.Phase3GateMatrixError, "sixteen"):
            validator.validate(changed)

        changed = copy.deepcopy(self.matrix)
        changed["featureFlagMatrix"][1]["classification"] = "valid"
        with self.assertRaisesRegex(validator.Phase3GateMatrixError, "automation dependency"):
            validator.validate(changed)

    def test_rejects_averaging_or_invented_environment(self):
        changed = copy.deepcopy(self.matrix)
        changed["provenance"]["failureRule"] = "average successful cells"
        with self.assertRaisesRegex(validator.Phase3GateMatrixError, "denominator"):
            validator.validate(changed)

        changed = copy.deepcopy(self.matrix)
        changed["environmentRoster"]["status"] = "complete"
        with self.assertRaisesRegex(validator.Phase3GateMatrixError, "invented"):
            validator.validate(changed)

    def test_rejects_invented_reviewer_and_weakened_beta_reset(self):
        changed = copy.deepcopy(self.matrix)
        changed["reviewRoster"][1]["owner"] = "implementation agent"
        changed["reviewRoster"][1]["status"] = "pass"
        with self.assertRaisesRegex(validator.Phase3GateMatrixError, "reviewers were invented"):
            validator.validate(changed)

        changed = copy.deepcopy(self.matrix)
        changed["beta"]["resetTriggers"].pop()
        with self.assertRaisesRegex(validator.Phase3GateMatrixError, "reset triggers"):
            validator.validate(changed)

    def test_rejects_missing_gate_owner_and_raw_evidence_export(self):
        changed = copy.deepcopy(self.matrix)
        changed["gates"]["C7"]["manualTask"] = "TASK-does-not-exist"
        with self.assertRaisesRegex(validator.Phase3GateMatrixError, "unknown gate owner"):
            validator.validate(changed)

        changed = copy.deepcopy(self.matrix)
        changed["artifactLayout"]["exportRule"] = "commit raw audio"
        with self.assertRaisesRegex(validator.Phase3GateMatrixError, "raw evidence"):
            validator.validate(changed)

    def test_campaign_validator_binds_root_flag_and_artifact_hash(self):
        with tempfile.TemporaryDirectory() as temporary:
            parent = pathlib.Path(temporary)
            campaign_id = "p3-20260717T000000Z-aaaaaaaaaaaa"
            campaign_root = parent / campaign_id
            result_root = campaign_root / "C1/0000/windows_windows/run-01"
            result_root.mkdir(parents=True)
            artifact = result_root / "metrics.csv"
            artifact.write_text("metric,value\ncycles,100\n", encoding="utf-8")
            root_commit = "a" * 40
            (campaign_root / "campaign.json").write_text(json.dumps({
                "contract": "p3-evidence-campaign.v1",
                "campaignId": campaign_id,
                "rootCommit": root_commit,
                "gateContractSHA256": hashlib.sha256(validator.MATRIX.read_bytes()).hexdigest(),
                "featureFlagId": "0000",
            }), encoding="utf-8")
            result = {
                "schemaVersion": 1,
                "contract": "p3-gate-result.v1",
                "gateContract": self.matrix["contract"],
                "campaignId": campaign_id,
                "gateId": "C1",
                "featureFlagId": "0000",
                "rootCommit": root_commit,
                "status": "pass",
                "commands": [{"argv": ["manual"], "exitCode": 0}],
                "samples": [{"cycles": 100}],
                "artifacts": [{
                    "path": artifact.relative_to(campaign_root).as_posix(),
                    "bytes": artifact.stat().st_size,
                    "sha256": hashlib.sha256(artifact.read_bytes()).hexdigest(),
                }],
                "blockers": [],
                "manualEvidence": "pass",
            }
            result_path = result_root / "result.json"
            result_path.write_text(json.dumps(result), encoding="utf-8")
            summary = campaign_validator.validate_campaign(self.matrix, campaign_root)
            self.assertEqual(1, summary["results"])

            result["featureFlagId"] = "1000"
            result_path.write_text(json.dumps(result), encoding="utf-8")
            with self.assertRaisesRegex(
                campaign_validator.Phase3CampaignError, "flag postures differ"
            ):
                campaign_validator.validate_campaign(self.matrix, campaign_root)

            result["featureFlagId"] = "0000"
            result["artifacts"][0]["sha256"] = "0" * 64
            result_path.write_text(json.dumps(result), encoding="utf-8")
            with self.assertRaisesRegex(campaign_validator.Phase3CampaignError, "hash mismatch"):
                campaign_validator.validate_campaign(self.matrix, campaign_root)


if __name__ == "__main__":
    unittest.main()
