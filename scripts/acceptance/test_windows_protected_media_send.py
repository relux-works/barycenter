#!/usr/bin/env python3

import importlib.util
import pathlib
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "windows_protected_media_send", HERE / "validate_windows_protected_media_send.py",
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class WindowsProtectedMediaSendAcceptanceTests(unittest.TestCase):
    def test_authoritative_packet(self):
        MODULE.validate(MODULE.load())

    def test_production_enablement_fails_closed(self):
        contract = MODULE.load()
        contract["decision"]["productionEnabled"] = True
        with self.assertRaisesRegex(MODULE.WindowsProtectedMediaSendError, "production-dark"):
            MODULE.validate(contract)

    def test_runtime_wiring_fails_closed(self):
        contract = MODULE.load()
        contract["decision"]["runtimeHTTPWired"] = True
        with self.assertRaisesRegex(MODULE.WindowsProtectedMediaSendError, "production-dark"):
            MODULE.validate(contract)

    def test_bounds_drift_fails_closed(self):
        contract = MODULE.load()
        contract["bounds"]["chunkBytes"] += 1
        with self.assertRaisesRegex(MODULE.WindowsProtectedMediaSendError, "bounds"):
            MODULE.validate(contract)

    def test_manual_evidence_fails_closed(self):
        contract = MODULE.load()
        contract["manualEvidence"]["signedMSIX"] = "passed"
        with self.assertRaisesRegex(MODULE.WindowsProtectedMediaSendError, "manual evidence"):
            MODULE.validate(contract)


if __name__ == "__main__":
    unittest.main()
