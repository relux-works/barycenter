import hashlib
import http.client
import importlib.util
import json
import pathlib
import sys
import tempfile
import threading
import unittest
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor


HERE = pathlib.Path(__file__).resolve().parent


def load(name: str, filename: str):
    spec = importlib.util.spec_from_file_location(name, HERE / filename)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


contract = load("codec_spike_contract", "validate_contract.py")
generator = load("codec_spike_generator", "generate_fixtures.py")
range_harness = load("codec_spike_range", "range_harness.py")
evaluator = load("codec_spike_evaluator", "evaluate_evidence.py")


def passing_evidence(rubric: dict, real: bool = True) -> dict:
    platforms = {
        "windows_windows": ("windows", "windows"),
        "windows_macos": ("windows", "macos"),
        "macos_macos": ("macos", "macos"),
    }
    environments = []
    nodes = {}
    for pairing, pair_platforms in platforms.items():
        node_ids = [f"{pairing}-a", f"{pairing}-b"]
        nodes[pairing] = node_ids
        environments.append({
            "pairing": pairing,
            "nodes": [
                {
                    "id": node_id,
                    "platform": platform,
                    "osBuild": "test-build",
                    "arch": "amd64" if platform == "windows" else "arm64",
                    "packageSha256": "a" * 64,
                    "realHardware": real,
                }
                for node_id, platform in zip(node_ids, pair_platforms)
            ],
        })
    samples = []
    for gate in rubric["hardGates"]:
        metric = gate["metric"]
        for group in evaluator.expected_groups(metric, rubric, nodes):
            base = {"metric": metric, "success": True, "unit": gate["unit"]}
            if metric == "scheduled_skew_ms":
                base.update(pairing=group[0], fixtureId=group[1])
            elif metric in ("duration_rss_growth_mib", "duration_rss_slope_mib_per_hour"):
                base.update(pairing=group[0], nodeId=group[1])
            else:
                base.update(pairing=group[0], nodeId=group[1], fixtureId=group[2])
            for iteration in range(gate["warmups"]):
                samples.append({**base, "iteration": iteration, "warmup": True,
                                "value": gate["limit"] / 4})
            for iteration in range(gate["samples"]):
                samples.append({**base, "iteration": iteration, "warmup": False,
                                "value": gate["limit"] / 2})
    fixture_ids = (
        {item["id"] for item in rubric["fixtureClasses"]} |
        set(rubric["smokeFixtures"]) |
        {item["id"] for item in rubric["hostileFixtures"]}
    )
    return {
        "schemaVersion": 1,
        "rubric": rubric["contract"],
        "candidateId": rubric["candidates"][0]["id"],
        "claimClass": "real-packaged-hardware" if real else "repository-synthetic",
        "build": {"gitCommit": "b" * 40, "buildSha256": "c" * 64, "sbomSha256": "d" * 64},
        "corpus": {"lockSha256": "e" * 64},
        "environments": environments,
        "coverage": {
            "pairings": rubric["requiredPairings"],
            "fixtureIds": sorted(fixture_ids),
            "rangeProfiles": rubric["rangeProfiles"],
        },
        "artifacts": [
            {"kind": kind, "path": f"artifacts/{kind}.json", "bytes": 1, "sha256": "f" * 64}
            for kind in rubric["requiredArtifactKinds"]
        ],
        "samples": samples,
        "checks": [
            {"id": check_id, "passed": True, "failureCount": 0}
            for check_id in rubric["zeroFailureChecks"]
        ],
        "failures": [],
    }


class CodecSpikeContractTests(unittest.TestCase):
    def test_frozen_contract_and_generator_plan_are_complete(self):
        rubric = contract.load()
        contract.validate(rubric)
        planned = generator.recipes(rubric, smoke_only=False)
        self.assertEqual(len(planned), 12)
        self.assertEqual({item["codec"] for item in planned}, {"mp3", "aac-lc", "opus"})
        self.assertEqual(sum(item["durationSeconds"] == 7200 for item in planned), 3)

    def test_range_harness_auth_ranges_conditionals_faults_and_redaction(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            fixtures = root / "fixtures"
            fixtures.mkdir()
            payload = bytes(range(256)) * 4
            fixture = fixtures / "sample.bin"
            fixture.write_bytes(payload)
            digest = hashlib.sha256(payload).hexdigest()
            lock = root / "lock.json"
            lock.write_text(json.dumps({
                "files": [{"id": "sample", "path": fixture.name,
                           "bytes": len(payload), "sha256": digest}],
            }), encoding="utf-8")
            log = root / "requests.jsonl"
            catalog = range_harness.FixtureCatalog(fixtures, lock)
            token = "secret-token-" + "x" * 40
            target = "opaque-target"
            server = range_harness.RangeServer(("127.0.0.1", 0), catalog, token, target, log,
                                                slow_bps=100000000)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            base = f"http://127.0.0.1:{server.server_port}/v1/fixtures/sample"

            def request(profile="normal", headers=None):
                merged = {"Authorization": f"Bearer {token}", "X-Codec-Spike-Target": target}
                merged.update(headers or {})
                return urllib.request.urlopen(
                    urllib.request.Request(f"{base}?profile={profile}", headers=merged), timeout=2
                )

            try:
                with self.assertRaises(urllib.error.HTTPError) as denied:
                    urllib.request.urlopen(base, timeout=2)
                self.assertEqual(denied.exception.code, 404)
                with request(headers={"Range": "bytes=10-19"}) as response:
                    self.assertEqual(response.status, 206)
                    self.assertEqual(response.headers["Content-Range"], f"bytes 10-19/{len(payload)}")
                    etag = response.headers["ETag"]
                    self.assertEqual(response.read(), payload[10:20])
                with request(headers={"Range": "bytes=-7"}) as response:
                    self.assertEqual(response.read(), payload[-7:])
                with request(headers={"Range": "bytes=10-19", "If-Range": '"wrong"'}) as response:
                    self.assertEqual(response.status, 200)
                    self.assertEqual(response.read(), payload)
                with request("no_range", {"Range": "bytes=10-19"}) as response:
                    self.assertEqual(response.status, 200)
                    self.assertEqual(response.headers["Accept-Ranges"], "none")
                with request("corrupt_chunk") as response:
                    self.assertNotEqual(response.read(), payload)
                with request("slow_256kbit") as response:
                    self.assertEqual(response.read(), payload)
                with request("etag_flip") as first:
                    etag_first = first.headers["ETag"]
                    first.read()
                with request("etag_flip") as second:
                    self.assertNotEqual(second.headers["ETag"], etag_first)
                    second.read()
                with self.assertRaises(urllib.error.HTTPError) as revoked:
                    request("revoked")
                self.assertEqual(revoked.exception.code, 410)
                with self.assertRaises(urllib.error.HTTPError) as not_modified:
                    request(headers={"If-None-Match": etag})
                self.assertEqual(not_modified.exception.code, 304)
                with self.assertRaises(urllib.error.HTTPError) as unsatisfiable:
                    request(headers={"Range": "bytes=5000-6000"})
                self.assertEqual(unsatisfiable.exception.code, 416)
                for profile in ("truncate_body", "reset_mid_body"):
                    with self.assertRaises((http.client.IncompleteRead, http.client.RemoteDisconnected,
                                            ConnectionResetError)):
                        with request(profile) as response:
                            response.read()
                duplicate = urllib.request.Request(
                    f"{base}?profile=normal&profile=revoked",
                    headers={"Authorization": f"Bearer {token}", "X-Codec-Spike-Target": target},
                )
                with self.assertRaises(urllib.error.HTTPError) as bad_profile:
                    urllib.request.urlopen(duplicate, timeout=2)
                self.assertEqual(bad_profile.exception.code, 400)

                def fetch_part(index):
                    start = index * 32
                    with request(headers={"Range": f"bytes={start}-{start + 31}"}) as response:
                        return index, response.read()

                with ThreadPoolExecutor(max_workers=8) as pool:
                    parts = dict(pool.map(fetch_part, range(16)))
                for index, part in parts.items():
                    self.assertEqual(part, payload[index * 32:index * 32 + 32])
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
            logged = log.read_text(encoding="utf-8")
            self.assertNotIn(token, logged)
            self.assertIn('"status":206', logged)

    def test_catalog_rejects_duplicate_ids_and_symlinks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            fixture = root / "sample.bin"
            fixture.write_bytes(b"fixture")
            digest = hashlib.sha256(fixture.read_bytes()).hexdigest()
            record = {"id": "sample", "path": fixture.name, "bytes": 7, "sha256": digest}
            lock = root / "lock.json"
            lock.write_text(json.dumps({"files": [record, record]}), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "duplicate fixture"):
                range_harness.FixtureCatalog(root, lock)
            link = root / "linked.bin"
            try:
                link.symlink_to(fixture)
            except (OSError, NotImplementedError):
                return
            linked = {**record, "id": "linked", "path": link.name}
            lock.write_text(json.dumps({"files": [linked]}), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "unsafe"):
                range_harness.FixtureCatalog(root, lock)

    def test_hostile_mutations_are_deterministic_and_nonempty(self):
        base = bytes(range(64))
        self.assertEqual(generator.mutate(base, "truncate:8"), base[:8])
        self.assertEqual(generator.mutate(base, "xor:middle:0x5a"),
                         base[:32] + bytes([base[32] ^ 0x5A]) + base[33:])
        self.assertTrue(generator.mutate(base, "prefix:max-synchsafe-id3").startswith(b"ID3"))
        self.assertTrue(generator.mutate(base, "prefix:overflowing-mp4-atom").startswith(b"\xff" * 4))

    def test_evaluator_passes_complete_real_matrix_and_labels_synthetic(self):
        rubric = contract.load()
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "evidence.json"
            path.write_text(json.dumps(passing_evidence(rubric, real=True)), encoding="utf-8")
            result = evaluator.evaluate(path)
            self.assertEqual(result["status"], "pass", result["errors"])
            self.assertTrue(result["finalClaim"])

            path.write_text(json.dumps(passing_evidence(rubric, real=False)), encoding="utf-8")
            self.assertEqual(evaluator.evaluate(path)["status"], "fail")
            engineering = evaluator.evaluate(path, engineering=True)
            self.assertEqual(engineering["status"], "engineering-pass", engineering["errors"])
            self.assertFalse(engineering["finalClaim"])

    def test_evaluator_fails_on_missing_or_failed_sample(self):
        rubric = contract.load()
        evidence = passing_evidence(rubric, real=True)
        evidence["samples"].pop()
        evidence["samples"][0]["success"] = False
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "evidence.json"
            path.write_text(json.dumps(evidence), encoding="utf-8")
            result = evaluator.evaluate(path)
        self.assertEqual(result["status"], "fail")
        self.assertTrue(any("sample count mismatch" in str(metric) or "failed sample" in str(metric)
                            for metric in result["metrics"].values()))


if __name__ == "__main__":
    unittest.main()
