from __future__ import annotations

import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("transport_model", HERE / "transport_model.py")
model = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
sys.modules[SPEC.name] = model
SPEC.loader.exec_module(model)


class TransportModelTests(unittest.TestCase):
    def test_model_is_deterministic_and_budget_bounded(self):
        first = model.build_artifact(frames=3000)
        second = model.build_artifact(frames=3000)
        self.assertEqual(first, second)
        self.assertTrue(all(profile["budgetModelPass"] for profile in first["profiles"]))

    def test_tcp_exposes_hol_not_application_loss(self):
        result = model.simulate_transport(frame_ms=20, transport="wss-tcp", frames=5000)
        self.assertGreater(result["network"]["tcpHolEvents"], 0)
        self.assertEqual(result["network"]["applicationDelivered"], 5000)
        self.assertEqual(result["network"]["fecRecovered"], 0)
        self.assertEqual(result["network"]["plcConcealed"], 0)
        self.assertEqual(result["network"]["arrivalReorders"], 0)

    def test_datagram_exposes_loss_to_fec_and_plc(self):
        result = model.simulate_transport(frame_ms=20, transport="quic-datagram", frames=5000)
        self.assertEqual(result["network"]["tcpHolEvents"], 0)
        self.assertLess(result["network"]["applicationDelivered"], 5000)
        self.assertGreater(result["network"]["fecRecovered"], 0)
        self.assertGreater(result["network"]["plcConcealed"], 0)
        self.assertGreater(result["network"]["arrivalReorders"], 0)

    def test_slow_recipient_drops_oldest_without_fast_recipient_loss(self):
        result = model.queue_isolation()
        self.assertTrue(result["isolated"])
        self.assertEqual(result["fastRecipient"]["droppedOldest"], 0)
        self.assertGreater(result["slowRecipient"]["droppedOldest"], 0)
        self.assertLessEqual(result["slowRecipient"]["maxDepth"], result["capacityFrames"])

    def test_invalid_inputs_fail_closed(self):
        for frame_ms, transport in ((5, "wss-tcp"), (20, "udp")):
            with self.assertRaises(ValueError):
                model.simulate_transport(frame_ms=frame_ms, transport=transport)
        with self.assertRaises(ValueError):
            model.queue_isolation(capacity=0)


if __name__ == "__main__":
    unittest.main()
