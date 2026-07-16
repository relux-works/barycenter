import importlib.util
import pathlib
import sys
import tempfile
import unittest


HERE = pathlib.Path(__file__).resolve().parent


def load(name: str, filename: str):
    spec = importlib.util.spec_from_file_location(name, HERE / filename)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


metrics = load("acceptance_metrics", "evaluate_metrics.py")
harness = load("acceptance_harness", "run_automated.py")
readiness = load("phase1_readiness", "validate_phase1_readiness.py")
targets_inbox = load("targets_inbox_contract", "../validate_targets_inbox_contract.py")


class AcceptanceHarnessTests(unittest.TestCase):
    def test_targets_inbox_contract_reuses_phase1_and_blocks_late_autoplay(self):
        contract = targets_inbox.load()
        targets_inbox.validate(contract)
        self.assertFalse(contract["reuse"]["parallelACLAllowed"])
        self.assertFalse(contract["inboxEligibility"]["autoPlayOnReconnect"])
        self.assertFalse(contract["replay"]["lateAutoPlayAllowed"])
        self.assertEqual(contract["moderation"]["reportImmediateGlobalEffect"], "none")

    def test_targets_inbox_contract_rejects_partial_mixed_version_create(self):
        contract = targets_inbox.load()
        contract["mixedVersion"]["partialCreateAllowed"] = True
        with self.assertRaisesRegex(targets_inbox.ContractError, "partial mixed-version"):
            targets_inbox.validate(contract)

    def test_targets_inbox_contract_rejects_acl_and_autoplay_expansion(self):
        contract = targets_inbox.load()
        contract["targetSnapshot"]["laterMemberExpansionAllowed"] = True
        with self.assertRaisesRegex(targets_inbox.ContractError, "snapshot ACL widened"):
            targets_inbox.validate(contract)

        contract = targets_inbox.load()
        contract["replay"]["lateAutoPlayAllowed"] = True
        with self.assertRaisesRegex(targets_inbox.ContractError, "late autoplay"):
            targets_inbox.validate(contract)

    def test_targets_inbox_contract_rejects_report_driven_denial_of_service(self):
        contract = targets_inbox.load()
        contract["moderation"]["reportImmediateGlobalEffect"] = "quarantine"
        with self.assertRaisesRegex(targets_inbox.ContractError, "global side effects"):
            targets_inbox.validate(contract)

    def test_sanitize_paths_and_secrets(self):
        source = f"{harness.ROOT}/x Authorization: Bearer abc token=def password:ghi"
        clean = harness.sanitize(source)
        self.assertNotIn(str(harness.ROOT), clean)
        self.assertNotIn("abc", clean)
        self.assertNotIn("def", clean)
        self.assertNotIn("ghi", clean)
        self.assertIn("<repo>", clean)

    def test_nearest_rank_p95(self):
        self.assertEqual(metrics.nearest_rank_p95(list(range(1, 31))), 29)

    def test_metrics_fail_closed_on_missing_samples_and_failures(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "metrics.csv"
            path.write_text(
                "kind,value,warmup,success\n"
                "stop_to_audible_ms,1000,false,true\n"
                "scheduled_skew_ms,20,false,false\n"
                "peak_memory_mib,200,false,true\n",
                encoding="utf-8",
            )
            result = metrics.evaluate(path)
        self.assertEqual(result["status"], "fail")
        self.assertEqual(result["failedSampleCount"], 1)

    def test_metrics_pass_complete_fixture(self):
        rows = ["kind,value,warmup,success"]
        rows.extend("stop_to_audible_ms,1,true,true" for _ in range(3))
        rows.extend("scheduled_skew_ms,1,true,true" for _ in range(3))
        rows.extend(f"stop_to_audible_ms,{1000 + index},false,true" for index in range(30))
        rows.extend(f"scheduled_skew_ms,{20 + index},false,true" for index in range(30))
        rows.append("peak_memory_mib,249,false,true")
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "metrics.csv"
            path.write_text("\n".join(rows) + "\n", encoding="utf-8")
            result = metrics.evaluate(path)
        self.assertEqual(result["status"], "pass")

    def test_frozen_contract_matches_modules_ci_and_manual_boundary(self):
        pins = harness.load_pins()
        root = harness.ROOT
        for module in (root / "coordinator/go.mod", root / "pulsar-win/go.mod"):
            self.assertIn(f"\ngo {pins['go']['version']}\n", module.read_text(encoding="utf-8"))
        workflow = (root / ".github/workflows/ci.yml").read_text(encoding="utf-8")
        self.assertNotIn("runs-on: ubuntu-latest", workflow)
        self.assertNotIn("runs-on: windows-latest", workflow)
        self.assertEqual(workflow.count("fetch-depth: 0"), 4)
        for runner in pins["githubHostedRunners"].values():
            self.assertIn(f"runs-on: {runner}", workflow)
        topology = harness.json.loads(
            (root / "acceptance/fixtures/phase1-topology.json").read_text(encoding="utf-8")
        )
        self.assertFalse(topology["productionDataAllowed"])
        self.assertEqual(len(topology["installations"]), 2)
        manual = harness.json.loads(
            (root / "acceptance/templates/manual-result.json").read_text(encoding="utf-8")
        )
        self.assertEqual(manual["status"], "manual-required")

    def test_contract_subprocess_cannot_dirty_checkout_with_bytecode(self):
        command = harness.suite_commands("swift", None, {})[0]
        self.assertEqual(command.name, "acceptance-contract-tests")
        self.assertEqual(command.env, {"PYTHONDONTWRITEBYTECODE": "1"})
        self.assertIn("scripts/codec_spike/test_codec_spike.py", command.argv)
        self.assertIn("scripts/codec_spike/test_independent_supply_review.py", command.argv)
        self.assertIn("scripts/acceptance/test_stream_performance_review.py", command.argv)
        self.assertIn("scripts/acceptance/test_air_migration_review.py", command.argv)
        self.assertIn("scripts/acceptance/test_target_security_review.py", command.argv)
        self.assertIn("scripts/acceptance/test_phase2_observability.py", command.argv)
        self.assertIn("scripts/acceptance/test_p2_root_review.py", command.argv)
        self.assertIn("scripts/acceptance/test_phase2_engineering_handoff.py", command.argv)

    def test_wack_runner_fails_closed_on_noninteractive_execution(self):
        source = (harness.ROOT / "scripts/acceptance/run_wack.ps1").read_text(encoding="utf-8")
        for contract in (
            "IsInRole",
            "UserInteractive",
            "SessionId -eq 0",
            "appcert.exe",
            "-appxpackagepath",
            "-reportoutputpath",
            "operatorReviewRequired = $true",
        ):
            self.assertIn(contract, source)

    def test_phase1_readiness_handoff_is_fail_closed_and_complete(self):
        data = readiness.load_strict(
            readiness.ROOT / "acceptance/phase1-engineering-readiness.json"
        )
        readiness.validate(data)


if __name__ == "__main__":
    unittest.main()
