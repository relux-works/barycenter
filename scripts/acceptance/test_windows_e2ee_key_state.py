#!/usr/bin/env python3

import importlib.util
import pathlib
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "windows_e2ee_key_state",
    HERE / "validate_windows_e2ee_key_state.py",
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class WindowsE2EEKeyStateAcceptanceTests(unittest.TestCase):
    def test_authoritative_packet(self):
        MODULE.validate(MODULE.load())

    def test_production_enablement_fails_closed(self):
        contract = MODULE.load()
        contract["decision"]["productionEnabled"] = True
        with self.assertRaisesRegex(MODULE.WindowsE2EEKeyStateError, "production-dark"):
            MODULE.validate(contract)

    def test_dpapi_scope_drift_fails_closed(self):
        contract = MODULE.load()
        contract["dpapiPolicy"]["localMachine"] = True
        with self.assertRaisesRegex(MODULE.WindowsE2EEKeyStateError, "DPAPI policy"):
            MODULE.validate(contract)

    def test_cross_process_lock_drift_fails_closed(self):
        contract = MODULE.load()
        contract["durabilityPolicy"]["crossProcessExclusiveLock"] = False
        with self.assertRaisesRegex(MODULE.WindowsE2EEKeyStateError, "durability policy"):
            MODULE.validate(contract)

    def test_manual_evidence_fails_closed(self):
        contract = MODULE.load()
        contract["manualEvidence"]["nativeCurrentUserDPAPI"] = "passed"
        with self.assertRaisesRegex(MODULE.WindowsE2EEKeyStateError, "manual evidence"):
            MODULE.validate(contract)


if __name__ == "__main__":
    unittest.main()
