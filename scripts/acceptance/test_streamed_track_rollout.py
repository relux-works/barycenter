#!/usr/bin/env python3

from __future__ import annotations

import copy
import importlib.util
import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "streamed_track_rollout",
    ROOT / "scripts" / "validate_streamed_track_rollout_handoff.py",
)
assert SPEC and SPEC.loader
rollout = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(rollout)


class StreamedTrackRolloutHandoffTests(unittest.TestCase):
    def test_handoff_is_complete_and_repository_anchored(self):
        rollout.validate(rollout.load())

    def test_documentation_cannot_claim_execution(self):
        value = copy.deepcopy(rollout.load())
        value["executionStatus"] = "executed"
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)

    def test_current_no_go_cannot_be_enabled(self):
        value = copy.deepcopy(rollout.load())
        value["productionActivation"]["activationAllowedNow"] = True
        value["productionActivation"]["selectedProfile"] = "aac-production"
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)

    def test_cache_bounds_cannot_drift(self):
        value = copy.deepcopy(rollout.load())
        value["bounds"]["installationCacheBytes"] += 1
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)

    def test_unsafe_mixed_version_fallback_is_rejected(self):
        value = copy.deepcopy(rollout.load())
        value["mixedVersion"]["clipFallbackAllowed"] = True
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)

    def test_plain_report_cannot_become_global_delete(self):
        value = copy.deepcopy(rollout.load())
        value["revocation"]["plainReport"] = "globally revoke every target"
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)

    def test_destructive_rollback_is_rejected(self):
        value = copy.deepcopy(rollout.load())
        value["rollback"]["downMigrationAllowed"] = True
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)

    def test_manual_evidence_cannot_be_claimed(self):
        value = copy.deepcopy(rollout.load())
        value["manualBoundary"][0]["status"] = "passed"
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)


if __name__ == "__main__":
    unittest.main()
