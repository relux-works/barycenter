from __future__ import annotations

import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "validate_codec_transport", HERE / "validate_codec_transport.py"
)
contract = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
sys.modules[SPEC.name] = contract
SPEC.loader.exec_module(contract)


class CodecTransportDecisionTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_current_no_go_decision_is_valid(self):
        contract.validate(self.data)

    def test_rejects_false_production_acceptance(self):
        changed = copy.deepcopy(self.data)
        changed["decision"]["productionLivePttAllowed"] = True
        with self.assertRaisesRegex(contract.DecisionError, "false production acceptance"):
            contract.validate(changed)

    def test_rejects_unproved_package_acceptance(self):
        changed = copy.deepcopy(self.data)
        changed["packageGates"]["windows"]["status"] = "pass"
        with self.assertRaisesRegex(contract.DecisionError, "unproved package gate"):
            contract.validate(changed)

    def test_rejects_widened_payload_or_changed_profile(self):
        changed = copy.deepcopy(self.data)
        changed["codecProfile"]["maxEncodedPayloadBytes"] = 1275
        with self.assertRaisesRegex(contract.DecisionError, "codec profile drifted"):
            contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["codecProfile"]["frameMs"] = 10
        with self.assertRaisesRegex(contract.DecisionError, "codec profile drifted"):
            contract.validate(changed)

    def test_rejects_evidence_hash_drift(self):
        changed = copy.deepcopy(self.data)
        changed["evidence"]["transportModel"]["sha256"] = "0" * 64
        with self.assertRaisesRegex(contract.DecisionError, "transport model receipt changed"):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
