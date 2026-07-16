#!/usr/bin/env python3

from __future__ import annotations

import copy
import importlib.util
import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "targets_inbox_rollout",
    ROOT / "scripts" / "validate_targets_inbox_rollout_handoff.py",
)
assert SPEC and SPEC.loader
rollout = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(rollout)


class TargetsInboxRolloutHandoffTests(unittest.TestCase):
    def test_handoff_is_complete_and_repository_anchored(self):
        rollout.validate(rollout.load())

    def test_manual_rollout_cannot_be_claimed(self):
        value = copy.deepcopy(rollout.load())
        value["executionStatus"] = "executed"
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)

    def test_streamed_tracks_cannot_be_enabled_early(self):
        value = copy.deepcopy(rollout.load())
        value["mixedVersion"]["targetedTrackBeforeStreamStory"] = "enabled"
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)

    def test_destructive_rollback_cannot_be_enabled(self):
        value = copy.deepcopy(rollout.load())
        value["rollback"]["downMigrationAllowed"] = True
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)

    def test_downstream_task_must_exist(self):
        value = copy.deepcopy(rollout.load())
        value["downstream"][0]["task"] = "TASK-does-not-exist"
        with self.assertRaises(rollout.HandoffError):
            rollout.validate(value)


if __name__ == "__main__":
    unittest.main()
