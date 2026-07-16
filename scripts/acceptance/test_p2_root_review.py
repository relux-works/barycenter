import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "p2_root_review", HERE / "validate_p2_root_review.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class Phase2RootReviewContractTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_exact_candidate_inventory_and_open_gates_are_current(self):
        contract.validate(self.data)

    def test_rejects_production_or_beta_acceptance(self):
        for key in ("phase2ProductionAccepted", "phase2PromotionAllowed", "betaAllowed"):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(contract.ReviewError, "fail-closed"):
                contract.validate(changed, verify_manifest=False)

    def test_rejects_binary_hash_without_accepted_codec(self):
        changed = copy.deepcopy(self.data)
        changed["acceptedArtifacts"]["productionBuildSha256"] = "0" * 64
        with self.assertRaisesRegex(contract.ReviewError, "unaccepted production artifact"):
            contract.validate(changed, verify_manifest=False)

    def test_rejects_closed_manual_evidence_or_lower_open_count(self):
        changed = copy.deepcopy(self.data)
        changed["manualAndExternalHolds"]["observabilityCampaignEvidenceComplete"] = True
        with self.assertRaisesRegex(contract.ReviewError, "manual observability"):
            contract.validate(changed, verify_manifest=False)
        changed = copy.deepcopy(self.data)
        changed["manualAndExternalHolds"]["openHighFindingCount"] = 12
        with self.assertRaisesRegex(contract.ReviewError, "open high count"):
            contract.validate(changed, verify_manifest=False)


if __name__ == "__main__":
    unittest.main()
