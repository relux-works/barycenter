import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "p3_final_engineering_audit", HERE / "validate_p3_final_engineering_audit.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class Phase3FinalEngineeringAuditTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_exact_post_root_diff_reviews_holds_and_pending_boundaries(self):
        contract.validate(self.data)

    def test_rejects_epic_production_store_promotion_or_beta_claim(self):
        for key in (
            "originalEpicComplete", "productionAccepted", "storeSubmissionAllowed",
            "promotionAllowed", "betaAccepted",
        ):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(contract.AuditError, "false final acceptance"):
                contract.validate(changed)

    def test_rejects_manual_independent_e2ee_or_release_claim(self):
        for key in (
            "manualEvidenceClaimed", "independentReviewClaimed", "e2eeAccepted",
            "releaseAuthorityGranted",
        ):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(contract.AuditError, "false final acceptance"):
                contract.validate(changed)

    def test_rejects_capability_promotion_or_activation(self):
        changed = copy.deepcopy(self.data)
        changed["capabilityDecisions"]["live_ptt"]["promotionDecision"] = "promote"
        with self.assertRaisesRegex(contract.AuditError, "capability promoted"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["capabilityDecisions"]["automation"]["productionActivationAllowed"] = True
        with self.assertRaisesRegex(contract.AuditError, "capability activation allowed"):
            contract.validate(changed)

    def test_rejects_fabricated_raw_beta_or_external_evidence(self):
        changed = copy.deepcopy(self.data)
        changed["evidenceBoundary"]["rawC1ThroughC7ArtifactsCommitted"] = True
        with self.assertRaisesRegex(contract.AuditError, "external evidence fabricated"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["betaContinuity"]["acceptedConsecutiveDays"] = 7
        with self.assertRaisesRegex(contract.AuditError, "beta day fabricated"):
            contract.validate(changed)

    def test_rejects_post_root_inventory_or_dependency_escape(self):
        changed = copy.deepcopy(self.data)
        changed["postRootReviewDelta"]["changedPathsNoRenames"] = 58
        with self.assertRaisesRegex(contract.AuditError, "post-root path count"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["postRootReviewDelta"]["dependencyLockChangedPaths"] = ["go.mod"]
        with self.assertRaisesRegex(contract.AuditError, "unreviewed dependency"):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
