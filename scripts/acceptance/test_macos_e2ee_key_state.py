#!/usr/bin/env python3

import importlib.util
import pathlib
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "macos_e2ee_key_state",
    HERE / "validate_macos_e2ee_key_state.py",
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class MacE2EEKeyStateAcceptanceTests(unittest.TestCase):
    def test_authoritative_packet(self):
        MODULE.validate(MODULE.load())

    def test_production_enablement_fails_closed(self):
        contract = MODULE.load()
        contract["decision"]["productionEnabled"] = True
        with self.assertRaisesRegex(MODULE.MacE2EEKeyStateError, "production-dark"):
            MODULE.validate(contract)

    def test_keychain_policy_drift_fails_closed(self):
        contract = MODULE.load()
        contract["keychainPolicy"]["synchronizable"] = True
        with self.assertRaisesRegex(MODULE.MacE2EEKeyStateError, "Keychain policy"):
            MODULE.validate(contract)

    def test_bound_drift_fails_closed(self):
        contract = MODULE.load()
        contract["bounds"]["cachedContentKeys"] += 1
        with self.assertRaisesRegex(MODULE.MacE2EEKeyStateError, "bounds"):
            MODULE.validate(contract)

    def test_manual_evidence_fails_closed(self):
        contract = MODULE.load()
        contract["manualEvidence"]["physicalKeychainAccessibility"] = "passed"
        with self.assertRaisesRegex(MODULE.MacE2EEKeyStateError, "manual evidence"):
            MODULE.validate(contract)


if __name__ == "__main__":
    unittest.main()
