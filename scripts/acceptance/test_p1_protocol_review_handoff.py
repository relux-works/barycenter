import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "p1_protocol_review_handoff", HERE / "validate_p1_protocol_review_handoff.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class Phase1ProtocolReviewHandoffTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_exact_candidate_delta_and_fail_closed_boundary(self):
        contract.validate(self.data)

    def test_rejects_fabricated_independent_acceptance(self):
        for key in (
            "independentReviewComplete", "originalTaskAccepted",
            "productionOrStoreAuthorityGranted",
        ):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(contract.HandoffError, "fabricated acceptance"):
                contract.validate(changed)

    def test_rejects_invented_reviewer_identity_or_verdict(self):
        changed = copy.deepcopy(self.data)
        changed["decision"]["reviewerIdentity"] = "unknown reviewer"
        with self.assertRaisesRegex(contract.HandoffError, "identity was invented"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["decision"]["reviewerDecision"] = "approved"
        with self.assertRaisesRegex(contract.HandoffError, "verdict was invented"):
            contract.validate(changed)

    def test_rejects_candidate_or_diff_drift(self):
        changed = copy.deepcopy(self.data)
        changed["reviewRange"]["currentMainTree"] = "0" * 40
        with self.assertRaisesRegex(contract.HandoffError, "candidate tree drifted"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["reviewRange"]["nameStatusSHA256"] = "0" * 64
        with self.assertRaisesRegex(contract.HandoffError, "name/status digest drifted"):
            contract.validate(changed)

    def test_rejects_golden_inventory_drift(self):
        changed = copy.deepcopy(self.data)
        changed["protocolInventory"]["currentGoldenCount"] = 58
        with self.assertRaisesRegex(contract.HandoffError, "candidate golden count drifted"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["protocolInventory"]["additiveGoldenPaths"] = []
        with self.assertRaisesRegex(contract.HandoffError, "additive golden inventory drifted"):
            contract.validate(changed)

    def test_rejects_external_evidence_claim(self):
        for key in (
            "independentVerdictMayBeClaimed", "manualRealAppEvidenceMayBeClaimed",
            "storeSubmissionMayBeClaimed",
        ):
            changed = copy.deepcopy(self.data)
            changed["evidenceBoundary"][key] = True
            with self.assertRaisesRegex(contract.HandoffError, "external evidence fabricated"):
                contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
