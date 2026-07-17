import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "p3_root_review", HERE / "validate_p3_root_review.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class Phase3RootReviewContractTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_exact_candidate_inventory_and_boundaries_are_current(self):
        contract.validate(self.data)

    def test_rejects_production_beta_manual_or_e2ee_acceptance(self):
        for key in (
            "phase3ProductionAccepted", "phase3PromotionAllowed", "betaAllowed",
            "manualEvidenceClaimed", "e2eeAccepted",
        ):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(contract.ReviewError, "fail-closed"):
                contract.validate(changed, verify_manifest=False)

    def test_rejects_unaccepted_binary_and_closed_manual_hold(self):
        changed = copy.deepcopy(self.data)
        changed["acceptedArtifacts"]["productionBuildSha256"] = "0" * 64
        with self.assertRaisesRegex(contract.ReviewError, "unaccepted production artifact"):
            contract.validate(changed, verify_manifest=False)
        changed = copy.deepcopy(self.data)
        changed["openHolds"]["manualRealAppHardwareStatus"] = "pass"
        with self.assertRaisesRegex(contract.ReviewError, "manual evidence"):
            contract.validate(changed, verify_manifest=False)

    def test_rejects_unresolved_high_finding_or_false_independence(self):
        changed = copy.deepcopy(self.data)
        changed["findings"][0]["status"] = "open"
        with self.assertRaisesRegex(contract.ReviewError, "unresolved critical/high"):
            contract.validate(changed, verify_manifest=False)
        changed = copy.deepcopy(self.data)
        changed["reviewer"]["independentReviewer"] = True
        with self.assertRaisesRegex(contract.ReviewError, "claimed independence"):
            contract.validate(changed, verify_manifest=False)


if __name__ == "__main__":
    unittest.main()
