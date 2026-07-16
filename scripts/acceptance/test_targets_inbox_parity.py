#!/usr/bin/env python3

from __future__ import annotations

import copy
import importlib.util
import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "targets_inbox_parity",
    ROOT / "scripts" / "validate_targets_inbox_parity_regressions.py",
)
assert SPEC and SPEC.loader
parity = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(parity)


class TargetsInboxParityEvidenceTests(unittest.TestCase):
    def test_manifest_is_complete_and_anchors_exist(self):
        parity.validate(parity.load())

    def test_missing_invariant_fails_closed(self):
        value = copy.deepcopy(parity.load())
        value["evidence"][0]["invariants"] = []
        with self.assertRaises(parity.EvidenceError):
            parity.validate(value)

    def test_manual_result_cannot_be_promoted(self):
        value = copy.deepcopy(parity.load())
        value["manualEvidence"][0]["status"] = "pass"
        with self.assertRaises(parity.EvidenceError):
            parity.validate(value)

    def test_track_runtime_cannot_be_claimed_before_downstream_story(self):
        value = copy.deepcopy(parity.load())
        value["sharedSurfaceFixture"]["targetedTrackPolicy"] = "queue"
        with self.assertRaises(parity.EvidenceError):
            parity.validate(value)

    def test_missing_executable_anchor_fails_closed(self):
        value = copy.deepcopy(parity.load())
        value["evidence"][0]["anchors"][0]["symbol"] = "TestDoesNotExist"
        with self.assertRaises(parity.EvidenceError):
            parity.validate(value)


if __name__ == "__main__":
    unittest.main()
