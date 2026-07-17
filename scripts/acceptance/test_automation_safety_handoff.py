import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "automation_safety_handoff", HERE / "validate_automation_safety_handoff.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class AutomationSafetyHandoffTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_complete_repository_matrix_and_manual_boundary(self):
        contract.validate(self.data)

    def test_rejects_false_c7_or_production_acceptance(self):
        for key in ("c7Accepted", "productionPromotionAllowed"):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(contract.AutomationHandoffError, "false acceptance"):
                contract.validate(changed)

    def test_rejects_missing_policy_coverage_or_manual_gap(self):
        changed = copy.deepcopy(self.data)
        del changed["coverage"]["dnd-block-air"]
        with self.assertRaisesRegex(contract.AutomationHandoffError, "coverage matrix"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["manualAcceptance"]["remainingGaps"].remove("signed-macos-app")
        with self.assertRaisesRegex(contract.AutomationHandoffError, "manual gap inventory"):
            contract.validate(changed)

    def test_rejects_unsafe_rollback_or_invented_client_result(self):
        changed = copy.deepcopy(self.data)
        changed["rollbackOrder"][-1]["action"] = "drop-automation-tables"
        with self.assertRaisesRegex(contract.AutomationHandoffError, "unsafe predecessor"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["clientMatrix"]["windows"]["audibleOutput"] = "pass"
        with self.assertRaisesRegex(contract.AutomationHandoffError, "manual client result"):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
