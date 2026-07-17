from __future__ import annotations

import copy
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import validate_integrated as validator  # noqa: E402


class CaptureQualityIntegratedEvidenceTests(unittest.TestCase):
    def setUp(self) -> None:
        self.evidence = validator.load(validator.EVIDENCE_PATH)

    def test_checked_in_evidence_passes_without_manual_claims(self):
        validator.run_integrated.validate_evidence(self.evidence)
        self.assertFalse(self.evidence["decision"]["c3Accepted"])
        self.assertEqual("not-run", self.evidence["decision"]["manualEvidence"])

    def test_rejects_false_c3_acceptance(self):
        changed = copy.deepcopy(self.evidence)
        changed["decision"]["c3Accepted"] = True
        with self.assertRaisesRegex(validator.run_integrated.IntegratedError, "manual/C3"):
            validator.run_integrated.validate_evidence(changed)

    def test_rejects_missing_cell_or_fixture(self):
        changed = copy.deepcopy(self.evidence)
        changed["adapters"][0]["cells"].pop()
        with self.assertRaisesRegex(validator.run_integrated.IntegratedError, "cell count"):
            validator.run_integrated.validate_evidence(changed)

        changed = copy.deepcopy(self.evidence)
        changed["matrix"]["fixtures"].pop()
        with self.assertRaisesRegex(validator.run_integrated.IntegratedError, "matrix identity"):
            validator.run_integrated.validate_evidence(changed)

    def test_rejects_ceiling_inversion_and_callback_blocking(self):
        changed = copy.deepcopy(self.evidence)
        changed["ceilings"]["captureInputDBFS"] = -1.0
        with self.assertRaisesRegex(validator.run_integrated.IntegratedError, "ceilings"):
            validator.run_integrated.validate_evidence(changed)

        changed = copy.deepcopy(self.evidence)
        changed["adapters"][0]["runtime"]["callbackBlockingWaits"] = 1
        with self.assertRaisesRegex(validator.run_integrated.IntegratedError, "callback blocking"):
            validator.run_integrated.validate_evidence(changed)

    def test_rejects_failed_command_and_private_metadata(self):
        changed = copy.deepcopy(self.evidence)
        changed["commands"][0]["exitCode"] = 1
        with self.assertRaisesRegex(validator.run_integrated.IntegratedError, "command coverage"):
            validator.run_integrated.validate_evidence(changed)

        changed = copy.deepcopy(self.evidence)
        changed["device_name"] = "private microphone"
        with self.assertRaisesRegex(validator.run_integrated.IntegratedError, "forbidden metadata"):
            validator.run_integrated.validate_evidence(changed)

    def test_rejects_audio_retention(self):
        changed = copy.deepcopy(self.evidence)
        changed["retention"]["syntheticAudioRetained"] = True
        with self.assertRaisesRegex(validator.run_integrated.IntegratedError, "must not be retained"):
            validator.run_integrated.validate_evidence(changed)


if __name__ == "__main__":
    unittest.main()
