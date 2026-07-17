import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "p3_automation_pre_review", HERE / "validate_p3_automation_pre_review.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class Phase3AutomationPreReviewContractTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_exact_source_inventory_and_fail_closed_gates_are_current(self):
        contract.validate(self.data)

    def test_rejects_false_independence_or_activation(self):
        changed = copy.deepcopy(self.data)
        changed["reviewer"]["independenceSatisfied"] = True
        with self.assertRaisesRegex(contract.ReviewError, "falsely claimed independence"):
            contract.validate(changed)
        for key in ("automationActivationAllowed", "c7Accepted", "phase3PromotionAllowed"):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(contract.ReviewError, "fail-closed"):
                contract.validate(changed)

    def test_rejects_manual_claim_or_source_drift(self):
        changed = copy.deepcopy(self.data)
        changed["openGates"][0]["manualEvidence"] = "pass"
        with self.assertRaisesRegex(contract.ReviewError, "manual evidence"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["sourceAnchors"][0]["sha256"] = "0" * 64
        with self.assertRaisesRegex(contract.ReviewError, "source digest mismatch"):
            contract.validate(changed)

    def test_rejects_removed_external_gate_or_open_finding(self):
        changed = copy.deepcopy(self.data)
        changed["openGates"] = changed["openGates"][:1]
        with self.assertRaisesRegex(contract.ReviewError, "open gate inventory"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["findings"] = [{"id": "P3-AUTO-001", "severity": "high", "status": "open"}]
        with self.assertRaisesRegex(contract.ReviewError, "unresolved review finding"):
            contract.validate(changed)

    def test_rejects_hidden_broad_timeout(self):
        changed = copy.deepcopy(self.data)
        changed["nonAcceptedAttempts"] = []
        with self.assertRaisesRegex(contract.ReviewError, "timeout was hidden"):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
