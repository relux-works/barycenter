import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "phase2_engineering_handoff", HERE / "validate_phase2_engineering_handoff.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class Phase2EngineeringHandoffTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_snapshot_flags_quotas_gates_and_pending_work_are_current(self):
        contract.validate(self.data)

    def test_rejects_production_promotion_or_beta_claim(self):
        for key in ("phase2ProductionAccepted", "phase2PromotionAllowed", "betaAccepted"):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(contract.HandoffError, "false acceptance"):
                contract.validate(changed)

    def test_rejects_production_hash_or_rollout_expansion(self):
        changed = copy.deepcopy(self.data)
        changed["artifacts"]["productionBuildSha256"] = "0" * 64
        with self.assertRaisesRegex(contract.HandoffError, "unaccepted production hash"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["rolloutOrder"][4]["allowedNow"] = True
        with self.assertRaisesRegex(contract.HandoffError, "production rollout"):
            contract.validate(changed)

    def test_rejects_missing_manual_gate_or_invented_flag(self):
        changed = copy.deepcopy(self.data)
        changed["gateIndex"]["B7"]["manualTasks"] = []
        with self.assertRaisesRegex(contract.HandoffError, "manual owner"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["featureAuthorities"]["targetsInboxRights"]["runtimeFlag"] = "targets"
        with self.assertRaisesRegex(contract.HandoffError, "invented target"):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
