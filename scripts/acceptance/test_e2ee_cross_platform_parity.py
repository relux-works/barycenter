import copy
import importlib.util
import pathlib
import sys
import unittest
from unittest import mock


ROOT = pathlib.Path(__file__).resolve().parents[2]
MODULE_PATH = ROOT / "scripts/e2ee_review/validate_cross_platform_vectors.py"
SPEC = importlib.util.spec_from_file_location("e2ee_cross_platform_vectors", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class E2EECrossPlatformParityTests(unittest.TestCase):
    def test_authoritative_repository_vectors_match(self):
        result = MODULE.validate()
        self.assertEqual(result["status"], "pass")
        self.assertEqual(result["manualInteroperability"], "not-run")

    def test_live_wire_drift_fails_closed(self):
        original = MODULE.load

        def changed(path):
            value = copy.deepcopy(original(path))
            if path.startswith("protocol/windows-e2ee-live"):
                value["opaqueFrame"]["encodedHex"] += "00"
            return value

        with mock.patch.object(MODULE, "load", side_effect=changed):
            with self.assertRaisesRegex(MODULE.CrossPlatformParityError, "live parity"):
                MODULE.validate()

    def test_send_fixture_drift_fails_closed(self):
        original = MODULE.load

        def changed(path):
            value = copy.deepcopy(original(path))
            if path.startswith("protocol/windows-protected-media-send"):
                value["ciphertextSHA256"] = "0" * 64
            return value

        with mock.patch.object(MODULE, "load", side_effect=changed):
            with self.assertRaisesRegex(MODULE.CrossPlatformParityError, "send parity"):
                MODULE.validate()

    def test_playback_and_client_drift_fail_closed(self):
        original = MODULE.load
        for target, field, expected in (
            ("protocol/windows-protected-media-playback", "chunks", "playback parity"),
            ("protocol/windows-encrypted-media-client", "commands", "client parity"),
        ):
            def changed(path, target=target, field=field):
                value = copy.deepcopy(original(path))
                if path.startswith(target):
                    value[field] = []
                return value

            with mock.patch.object(MODULE, "load", side_effect=changed):
                with self.assertRaisesRegex(MODULE.CrossPlatformParityError, expected):
                    MODULE.validate()


if __name__ == "__main__":
    unittest.main()
