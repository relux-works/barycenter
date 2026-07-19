#!/usr/bin/env python3

import importlib.util
import pathlib
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "e2ee_report_moderation_export",
    HERE / "validate_e2ee_report_moderation_export.py",
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class E2EEReportModerationExportTests(unittest.TestCase):
    def test_authoritative_packet(self):
        MODULE.validate(MODULE.load())

    def test_production_enablement_fails_closed(self):
        contract = MODULE.load()
        contract["decision"]["productionEnabled"] = True
        with self.assertRaisesRegex(MODULE.E2EEReportModerationError, "production-dark"):
            MODULE.validate(contract)

    def test_plaintext_claim_fails_closed(self):
        contract = MODULE.load()
        contract["decision"]["coordinatorStoresPlaintextEvidence"] = True
        with self.assertRaisesRegex(MODULE.E2EEReportModerationError, "production-dark"):
            MODULE.validate(contract)

    def test_bound_drift_fails_closed(self):
        contract = MODULE.load()
        contract["bounds"]["evidenceBytes"] += 1
        with self.assertRaisesRegex(MODULE.E2EEReportModerationError, "resource bounds"):
            MODULE.validate(contract)

    def test_manual_evidence_fails_closed(self):
        contract = MODULE.load()
        contract["manualEvidence"]["realAppModerationFlow"] = "passed"
        with self.assertRaisesRegex(MODULE.E2EEReportModerationError, "manual evidence"):
            MODULE.validate(contract)


if __name__ == "__main__":
    unittest.main()
