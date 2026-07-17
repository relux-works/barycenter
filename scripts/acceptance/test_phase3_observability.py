import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "phase3_observability", HERE / "validate_phase3_observability.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class Phase3ObservabilityContractTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_contract_is_current_and_source_pinned(self):
        contract.validate(self.data)

    def test_rejects_dynamic_labels_and_selector_queries(self):
        changed = copy.deepcopy(self.data)
        changed["cardinality"]["dynamicLabelsAllowed"] = True
        with self.assertRaisesRegex(contract.ContractError, "dynamic labels"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["operatorEndpoint"]["queryParameters"] = ["orbit_id"]
        with self.assertRaisesRegex(contract.ContractError, "selector queries"):
            contract.validate(changed)

    def test_rejects_false_manual_or_promotion_claim(self):
        changed = copy.deepcopy(self.data)
        changed["readiness"]["manualStatus"] = "passed"
        with self.assertRaisesRegex(contract.ContractError, "manual evidence"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["readiness"]["promotionEvidenceReady"] = "true when runtime health is green"
        with self.assertRaisesRegex(contract.ContractError, "claims promotion"):
            contract.validate(changed)

    def test_rejects_crypto_secret_or_missing_archive_binding(self):
        changed = copy.deepcopy(self.data)
        changed["cardinality"]["forbidden"].remove("cryptographic key")
        with self.assertRaisesRegex(contract.ContractError, "privacy denylist"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["evidence"]["manifestFields"].pop()
        with self.assertRaisesRegex(contract.ContractError, "archive binding"):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
