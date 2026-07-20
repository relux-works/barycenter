#!/usr/bin/env python3

import importlib.util
import pathlib
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "e2ee_recovery_device_transfer",
    HERE / "validate_e2ee_recovery_device_transfer.py",
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class E2EERecoveryDeviceTransferTests(unittest.TestCase):
    def test_authoritative_packet(self):
        MODULE.validate(MODULE.load())

    def test_production_enablement_fails_closed(self):
        contract = MODULE.load()
        contract["decision"]["productionEnabled"] = True
        with self.assertRaisesRegex(MODULE.E2EERecoveryError, "production-dark"):
            MODULE.validate(contract)

    def test_default_history_claim_fails_closed(self):
        contract = MODULE.load()
        contract["decision"]["historicalKeysByDefault"] = True
        with self.assertRaisesRegex(MODULE.E2EERecoveryError, "production-dark"):
            MODULE.validate(contract)

    def test_resource_bound_drift_fails_closed(self):
        contract = MODULE.load()
        contract["bounds"]["historyMaxReads"] += 1
        with self.assertRaisesRegex(MODULE.E2EERecoveryError, "resource bounds"):
            MODULE.validate(contract)

    def test_manual_evidence_fails_closed(self):
        contract = MODULE.load()
        contract["manualEvidence"]["physicalDeviceTransfer"] = "passed"
        with self.assertRaisesRegex(MODULE.E2EERecoveryError, "manual evidence"):
            MODULE.validate(contract)


if __name__ == "__main__":
    unittest.main()
