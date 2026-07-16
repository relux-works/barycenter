import copy
import importlib.util
import pathlib
import sys
import unittest

HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "phase2_observability", HERE / "validate_phase2_observability.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class Phase2ObservabilityContractTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_contract_is_current_and_source_pinned(self):
        contract.validate(self.data)

    def test_rejects_dynamic_labels_and_tenant_query(self):
        changed = copy.deepcopy(self.data)
        changed["cardinality"]["dynamicLabelsAllowed"] = True
        with self.assertRaisesRegex(contract.ContractError, "dynamic labels"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["operatorEndpoint"]["queryParameters"] = ["actor_id"]
        with self.assertRaisesRegex(contract.ContractError, "tenant selectors"):
            contract.validate(changed)

    def test_rejects_false_client_timing_claim(self):
        changed = copy.deepcopy(self.data)
        changed["evidence"]["manualBoundary"][0] = "all timing is server-proven"
        with self.assertRaisesRegex(contract.ContractError, "manual timing"):
            contract.validate(changed)

    def test_rejects_parallel_accounting_inventory(self):
        changed = copy.deepcopy(self.data)
        changed["metricGroups"]["accounting"].append("duplicate_egress_total")
        with self.assertRaisesRegex(contract.ContractError, "accounting inventory"):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
