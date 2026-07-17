#!/usr/bin/env python3

import importlib.util
import pathlib
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "e2ee_protocol_key_lifecycle",
    HERE / "validate_e2ee_protocol_key_lifecycle.py",
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class E2EEProtocolKeyLifecycleTests(unittest.TestCase):
    def test_authoritative_packet(self):
        MODULE.validate(MODULE.load())

    def test_production_enablement_fails_closed(self):
        contract = MODULE.load()
        contract["decision"]["capabilityAdvertised"] = True
        with self.assertRaisesRegex(MODULE.E2EEProtocolContractError, "no-go"):
            MODULE.validate(contract)

    def test_selected_suite_fails_closed(self):
        contract = MODULE.load()
        contract["vocabulary"]["productionSuites"] = ["unreviewed"]
        with self.assertRaisesRegex(MODULE.E2EEProtocolContractError, "suite"):
            MODULE.validate(contract)


if __name__ == "__main__":
    unittest.main()
