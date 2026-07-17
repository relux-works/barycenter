from __future__ import annotations

import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "capture_quality_contract_validator", HERE / "validate_capture_quality_contract.py"
)
validator = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
sys.modules[SPEC.name] = validator
SPEC.loader.exec_module(validator)


class CaptureQualityContractTests(unittest.TestCase):
    def setUp(self) -> None:
        self.protocol = validator.load(validator.PROTOCOL_PATH)
        self.evidence = validator.load(validator.EVIDENCE_PATH)

    def test_frozen_contract_and_repository_boundaries_pass(self):
        validator.validate_protocol(self.protocol)
        validator.validate_evidence(self.evidence)
        validator.validate_repository_boundaries()
        validator.validate_diagnostics_surface()

    def test_rejects_missing_common_workflow(self):
        changed = copy.deepcopy(self.protocol)
        changed["workflows"] = changed["workflows"][:-1]
        with self.assertRaisesRegex(validator.CaptureQualityContractError, "workflow coverage"):
            validator.validate_protocol(changed)

    def test_rejects_speaker_acceptance_without_reference(self):
        changed = copy.deepcopy(self.protocol)
        changed["routes"]["speakerAcceptedRequires"].remove("eligible synchronized render reference")
        with self.assertRaisesRegex(validator.CaptureQualityContractError, "speaker acceptance"):
            validator.validate_protocol(changed)

    def test_rejects_ceiling_conflation_or_coordinator_control(self):
        changed = copy.deepcopy(self.protocol)
        changed["ceilings"]["receiverOutput"]["peakCeilingDBFS"] = -3.0
        with self.assertRaisesRegex(validator.CaptureQualityContractError, "receiver ceiling"):
            validator.validate_protocol(changed)

        changed = copy.deepcopy(self.protocol)
        changed["ceilings"]["inputAGC"]["coordinatorMutable"] = True
        with self.assertRaisesRegex(validator.CaptureQualityContractError, "input AGC"):
            validator.validate_protocol(changed)

    def test_rejects_silent_degraded_or_live_fallback(self):
        changed = copy.deepcopy(self.protocol)
        changed["fallback"]["degraded"] = "continue automatically"
        with self.assertRaisesRegex(validator.CaptureQualityContractError, "not explicit"):
            validator.validate_protocol(changed)

        changed = copy.deepcopy(self.protocol)
        changed["fallback"]["live_ptt"] = "automatically create a clip"
        with self.assertRaisesRegex(validator.CaptureQualityContractError, "automatic live fallback"):
            validator.validate_protocol(changed)

    def test_rejects_false_c3_or_manual_evidence(self):
        changed = copy.deepcopy(self.evidence)
        changed["decision"]["c3Accepted"] = True
        with self.assertRaisesRegex(validator.CaptureQualityContractError, "invents readiness"):
            validator.validate_evidence(changed)

        changed = copy.deepcopy(self.evidence)
        changed["manualEvidence"]["blindedListening"] = "pass"
        with self.assertRaisesRegex(validator.CaptureQualityContractError, "manual evidence invented"):
            validator.validate_evidence(changed)

    def test_rejects_averaged_or_incomplete_matrix(self):
        changed = copy.deepcopy(self.evidence)
        changed["objectiveRubric"]["rule"] = "average all platforms"
        with self.assertRaisesRegex(validator.CaptureQualityContractError, "averaged away"):
            validator.validate_evidence(changed)

        changed = copy.deepcopy(self.evidence)
        changed["matrix"] = [item for item in changed["matrix"] if item["id"] != "double_talk"]
        with self.assertRaisesRegex(validator.CaptureQualityContractError, "matrix incomplete"):
            validator.validate_evidence(changed)


if __name__ == "__main__":
    unittest.main()
