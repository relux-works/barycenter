import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "phase3_engineering_handoff", HERE / "validate_phase3_engineering_handoff.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class Phase3EngineeringHandoffTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_exact_snapshot_gate_disclosure_and_pending_indexes_are_current(self):
        contract.validate(self.data)

    def test_rejects_promotion_beta_manual_or_independent_claim(self):
        for key in (
            "phase3PromotionAllowed", "betaAccepted", "manualEvidenceClaimed",
            "independentReviewClaimed",
        ):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(contract.HandoffError, "false acceptance"):
                contract.validate(changed)

    def test_rejects_production_hash_or_capability_activation(self):
        changed = copy.deepcopy(self.data)
        changed["artifacts"]["productionBuildSha256"] = "0" * 64
        with self.assertRaisesRegex(contract.HandoffError, "unaccepted production hash"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["capabilityRecommendations"]["live_ptt"]["productionActivationAllowed"] = True
        with self.assertRaisesRegex(contract.HandoffError, "capability activation allowed"):
            contract.validate(changed)

    def test_rejects_missing_gate_manual_task_or_e2ee_hold(self):
        changed = copy.deepcopy(self.data)
        del changed["gateIndex"]["C7"]
        with self.assertRaisesRegex(contract.HandoffError, "gate index incomplete"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["manualEpic"]["tasks"]["phase3Release"].remove("TASK-260712-1actom")
        with self.assertRaisesRegex(contract.HandoffError, "manual task inventory"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["deferredE2EE"]["tasks"].remove("TASK-260712-aniuyy")
        with self.assertRaisesRegex(contract.HandoffError, "deferred E2EE inventory"):
            contract.validate(changed)

    def test_rejects_release_authority_or_e2ee_claim(self):
        changed = copy.deepcopy(self.data)
        changed["releaseAuthorityGranted"] = True
        with self.assertRaisesRegex(contract.HandoffError, "release authority"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["capabilityRecommendations"]["e2ee_media"]["e2eeClaimAllowed"] = True
        with self.assertRaisesRegex(contract.HandoffError, "E2EE claim allowed"):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
