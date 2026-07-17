from __future__ import annotations

import copy
import importlib.util
import json
import pathlib
import shutil
import sys
import tempfile
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("capture_quality_harness", HERE / "harness.py")
harness = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
sys.modules[SPEC.name] = harness
SPEC.loader.exec_module(harness)


class CaptureQualityHarnessTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.temporary = tempfile.TemporaryDirectory()
        cls.root = pathlib.Path(cls.temporary.name)
        cls.corpus = cls.root / "corpus"
        cls.lock_path = cls.root / "lock.json"
        cls.lock = harness.generate_corpus(cls.corpus, cls.lock_path)
        cls.candidate_dir = cls.root / "conforming"
        cls.candidate_path = harness.demo_candidate(
            cls.candidate_dir, cls.corpus, cls.lock_path, "conforming"
        )

    @classmethod
    def tearDownClass(cls) -> None:
        cls.temporary.cleanup()

    def clone_candidate(self, name: str) -> tuple[pathlib.Path, dict]:
        target = self.root / name
        shutil.copytree(self.candidate_dir, target)
        path = target / "candidate.json"
        return path, json.loads(path.read_text(encoding="utf-8"))

    @staticmethod
    def write_candidate(path: pathlib.Path, candidate: dict) -> None:
        path.write_text(json.dumps(candidate, indent=2, sort_keys=True) + "\n", encoding="utf-8")

    def replace_output(self, candidate_path: pathlib.Path, candidate: dict, case_id: str, values: list[float]) -> None:
        case = next(item for item in candidate["cases"] if item["id"] == case_id)
        path = candidate_path.parent / case["output"]
        path.write_bytes(harness.encode(values))
        case["sha256"] = harness.sha256_file(path)
        self.write_candidate(candidate_path, candidate)

    def signals(self, case_id: str) -> dict[str, list[float]]:
        fixture = next(item for item in self.lock["fixtures"] if item["id"] == case_id)
        return {
            role: harness.decode(self.corpus / record["path"], fixture["samples"])
            for role, record in fixture["files"].items()
        }

    def test_generated_lock_matches_checked_in_lock_and_conforming_bundle_passes(self):
        checked_in = json.loads(harness.LOCK_PATH.read_text(encoding="utf-8"))
        self.assertEqual(checked_in, self.lock)
        report = harness.evaluate(self.candidate_path, self.corpus, self.lock_path)
        self.assertTrue(report["passed"], report["failures"])
        self.assertEqual("not-run", report["manualEvidence"])
        self.assertEqual(14, len(report["results"]))

    def test_detects_fake_bypass(self):
        path, candidate = self.clone_candidate("bypass")
        self.replace_output(path, candidate, "far_end_only", self.signals("far_end_only")["capture"])
        report = harness.evaluate(path, self.corpus, self.lock_path)
        self.assertFalse(report["passed"])
        self.assertTrue(any("fake bypass" in item for item in report["failures"]))

        path, candidate = self.clone_candidate("double-talk-bypass")
        self.replace_output(path, candidate, "double_talk", self.signals("double_talk")["capture"])
        report = harness.evaluate(path, self.corpus, self.lock_path)
        self.assertFalse(report["passed"])
        self.assertTrue(any("double-talk ERLE" in item for item in report["failures"]))

    def test_detects_near_end_destruction(self):
        path, candidate = self.clone_candidate("destruction")
        length = len(self.signals("near_end_only")["near"])
        self.replace_output(path, candidate, "near_end_only", [0.0] * length)
        report = harness.evaluate(path, self.corpus, self.lock_path)
        self.assertFalse(report["passed"])
        self.assertTrue(any("near-end destruction" in item for item in report["failures"]))

    def test_detects_ceiling_and_clipping_violation(self):
        path, candidate = self.clone_candidate("ceiling")
        self.replace_output(path, candidate, "clipping", self.signals("clipping")["capture"])
        report = harness.evaluate(path, self.corpus, self.lock_path)
        self.assertFalse(report["passed"])
        self.assertTrue(any("peak ceiling" in item or "clipped sample" in item for item in report["failures"]))

    def test_detects_realtime_blocking(self):
        path, candidate = self.clone_candidate("realtime")
        candidate["runtime"]["callbackBlockingWaits"] = 1
        self.write_candidate(path, candidate)
        report = harness.evaluate(path, self.corpus, self.lock_path)
        self.assertFalse(report["passed"])
        self.assertIn("runtime: callback blocking wait detected", report["failures"])

    def test_detects_nondeterministic_lifecycle_and_packet_cancel(self):
        path, candidate = self.clone_candidate("lifecycle")
        route = next(item for item in candidate["cases"] if item["id"] == "route_change")
        route["events"]["generationAfter"] = route["events"]["generationBefore"]
        cancel = next(item for item in candidate["cases"] if item["id"] == "live_packet_cancel")
        cancel["events"]["packetsAfterCancel"] = 1
        self.write_candidate(path, candidate)
        report = harness.evaluate(path, self.corpus, self.lock_path)
        self.assertFalse(report["passed"])
        self.assertTrue(any("generation did not advance" in item for item in report["failures"]))
        self.assertTrue(any("packet emitted after cancel" in item for item in report["failures"]))

    def test_rejects_hash_tamper_traversal_and_manual_claim(self):
        path, candidate = self.clone_candidate("tamper")
        candidate["cases"][0]["sha256"] = "0" * 64
        self.write_candidate(path, candidate)
        with self.assertRaisesRegex(harness.HarnessError, "output hash mismatch"):
            harness.evaluate(path, self.corpus, self.lock_path)

        path, candidate = self.clone_candidate("traversal")
        candidate["cases"][0]["output"] = "../outside.f32le"
        self.write_candidate(path, candidate)
        with self.assertRaisesRegex(harness.HarnessError, "unsafe relative path"):
            harness.evaluate(path, self.corpus, self.lock_path)

        path, candidate = self.clone_candidate("wrong-cell")
        candidate["candidate"]["workflow"] = "all-workflows"
        self.write_candidate(path, candidate)
        with self.assertRaisesRegex(harness.HarnessError, "outside frozen matrix"):
            harness.evaluate(path, self.corpus, self.lock_path)

        changed_lock = copy.deepcopy(self.lock)
        changed_lock["seed"] += 1
        changed_lock_path = self.root / "untrusted-lock.json"
        changed_lock_path.write_text(
            json.dumps(changed_lock, indent=2, sort_keys=True) + "\n", encoding="utf-8"
        )
        path, candidate = self.clone_candidate("wrong-lock")
        candidate["fixtureLockSHA256"] = harness.sha256_file(changed_lock_path)
        self.write_candidate(path, candidate)
        with self.assertRaisesRegex(harness.HarnessError, "differs from the checked-in"):
            harness.evaluate(path, self.corpus, changed_lock_path)

        spec = copy.deepcopy(harness.contract())
        spec["manualBoundary"]["status"] = "pass"
        original = harness.load_json
        try:
            harness.load_json = lambda path: spec if path == harness.CONTRACT_PATH else original(path)
            with self.assertRaisesRegex(harness.HarnessError, "must not claim manual evidence"):
                harness.contract()
        finally:
            harness.load_json = original


if __name__ == "__main__":
    unittest.main()
