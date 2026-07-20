#!/usr/bin/env python3

import copy
import importlib.util
import pathlib
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "macos_encrypted_media_client_path",
    HERE / "validate_macos_encrypted_media_client_path.py",
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class MacEncryptedMediaClientPathAcceptanceTests(unittest.TestCase):
    def test_authoritative_packet(self):
        MODULE.validate(*MODULE.load())

    def test_runtime_enablement_fails_closed(self):
        protocol, evidence = MODULE.load()
        changed = copy.deepcopy(protocol)
        changed["runtime_wired"] = True
        with self.assertRaisesRegex(MODULE.MacEncryptedMediaClientPathError, "runtime"):
            MODULE.validate(changed, evidence)

    def test_capability_advertisement_fails_closed(self):
        protocol, evidence = MODULE.load()
        changed = copy.deepcopy(protocol)
        changed["capability_advertised"] = True
        with self.assertRaisesRegex(MODULE.MacEncryptedMediaClientPathError, "capability"):
            MODULE.validate(changed, evidence)

    def test_implicit_history_fails_closed(self):
        protocol, evidence = MODULE.load()
        changed = copy.deepcopy(protocol)
        changed["recovery"]["history_included_by_default"] = True
        with self.assertRaisesRegex(MODULE.MacEncryptedMediaClientPathError, "history"):
            MODULE.validate(changed, evidence)

    def test_evidence_consent_removal_fails_closed(self):
        protocol, evidence = MODULE.load()
        changed = copy.deepcopy(protocol)
        changed["report_modes"][1]["separate_confirmation_required"] = False
        with self.assertRaisesRegex(MODULE.MacEncryptedMediaClientPathError, "consent"):
            MODULE.validate(changed, evidence)


if __name__ == "__main__":
    unittest.main()
