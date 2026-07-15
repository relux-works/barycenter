import hashlib
import http.client
import importlib.util
import json
import pathlib
import sqlite3
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
stream_contract = load("codec_spike_stream_contract", "stream_contract.py")
license_audit = load("codec_spike_license_audit", "validate_license_audit.py")
bundled_probe = load("codec_spike_bundled_probe", "inventory_bundled_probe.py")


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
    def test_bundled_probe_contract_is_complete_and_fails_closed(self):
        probe = bundled_probe.load_contract()
        bundled_probe.validate_contract(probe)
        self.assertEqual(len(probe["smokeFixtures"]), 6)
        self.assertEqual(
            {item["id"] for item in probe["platformMatrix"]},
            {"macos-arm64", "windows-amd64", "windows-arm64"},
        )
        self.assertFalse(probe["package"]["runtimeExecutableDownload"])
        self.assertFalse(probe["package"]["renderCallbackCallsDecoder"])

        for mutation, expected in (
            (("package", "runtimeExecutableDownload", True), "runtime executable download"),
            (("package", "decoderProcessOwnsNetwork", True), "decoder owns network"),
            (("decision", "shipping", "approved"), "shipping decision"),
        ):
            tampered = json.loads(json.dumps(probe))
            section, field, value = mutation
            tampered[section][field] = value
            with self.assertRaisesRegex(ValueError, expected):
                bundled_probe.validate_contract(tampered)

        tampered = json.loads(json.dumps(probe))
        tampered["platformMatrix"].pop()
        with self.assertRaisesRegex(ValueError, "architecture matrix"):
            bundled_probe.validate_contract(tampered)

    def test_exact_license_audit_is_complete_and_fail_closed(self):
        audit = license_audit.load()
        license_audit.validate(audit)
        candidates = {item["id"]: item for item in audit["candidates"]}
        self.assertEqual(candidates["pure-go-composite-v1"]["classification"], "rejected")
        self.assertEqual(candidates["native-canonical-aac-v1"]["classification"],
                         "shippable-with-obligations")
        tampered = json.loads(json.dumps(audit))
        tampered["components"][-1]["forbiddenConfigure"].remove("--enable-gpl")
        with self.assertRaisesRegex(ValueError, "license tripwire"):
            license_audit.validate(tampered)

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

    def test_stream_variant_manifest_range_client_and_uniform_acl(self):
        stream_contract.validate_contract(stream_contract.load_contract())
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            fixtures = root / "fixtures"
            fixtures.mkdir()
            payload = bytes(range(256)) * 20
            fixture = fixtures / "aac.m4a"
            fixture.write_bytes(payload)
            digest = hashlib.sha256(payload).hexdigest()
            lock = root / "lock.json"
            lock.write_text(json.dumps({"files": [{
                "id": "canonical-aac", "path": fixture.name,
                "bytes": len(payload), "sha256": digest,
            }]}), encoding="utf-8")
            manifest = stream_contract.build_variant_manifest({
                "id": "canonical-aac", "mediaId": "media-fixture-1",
                "codec": "aac-lc", "container": "m4a-faststart", "mime": "audio/mp4",
                "rateMode": "cbr", "bitrateBPS": 160000, "durationMS": 120000,
            }, payload, chunk_size=1024)
            self.assertEqual(manifest["storageKey"], "stream/v1/" + digest)
            self.assertEqual(stream_contract.seek_offset(manifest, 21500), 0)
            self.assertLessEqual(max(
                b["timeMS"] - a["timeMS"]
                for a, b in zip(manifest["seekMap"], manifest["seekMap"][1:])
            ), 10000)
            catalog_db = root / "stream-variants.sqlite3"
            lock_with_recipe = json.loads(lock.read_text(encoding="utf-8"))
            lock_with_recipe["files"][0].update({
                "recipe": {"codec": "aac-lc", "container": "m4a-faststart",
                           "rateMode": "cbr", "bitrateKbps": 160, "durationSeconds": 120},
                "probe": {"sampleRateHz": 48000, "channels": 2, "durationSeconds": 120},
            })
            lock.write_text(json.dumps(lock_with_recipe), encoding="utf-8")
            catalog = stream_contract.materialize_catalog(fixtures, lock, catalog_db, chunk_size=1024)
            self.assertEqual(len(catalog), 1)
            connection = sqlite3.connect(catalog_db)
            try:
                row = connection.execute("""SELECT codec,container,size_bytes,etag,status,
                    chunk_manifest_json,seek_map_json FROM stream_variants""").fetchone()
            finally:
                connection.close()
            self.assertEqual(row[:5], ("aac-lc", "m4a-faststart", len(payload), manifest["etag"], "ready"))
            self.assertEqual(len(json.loads(row[5])), 5)
            self.assertEqual(json.loads(row[6])[0], {"timeMS": 0, "offset": 0})

            token = "stream-secret-" + "z" * 40
            target = "immutable-target-snapshot"
            server = range_harness.RangeServer(
                ("127.0.0.1", 0), range_harness.FixtureCatalog(fixtures, lock),
                token, target, root / "range.jsonl", slow_bps=100000000,
            )
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            url = f"http://127.0.0.1:{server.server_port}/v1/fixtures/canonical-aac"
            try:
                first = stream_contract.fetch_chunk(url, token, target, manifest, 0)
                self.assertEqual(first, payload[:1024])
                with self.assertRaises(stream_contract.AccessRevoked):
                    stream_contract.fetch_chunk(url, token, "another-tenant", manifest, 0)
                with self.assertRaises(stream_contract.VersionChanged):
                    stream_contract.fetch_chunk(url + "?profile=etag_flip", token, target, manifest, 0)
                    stream_contract.fetch_chunk(url + "?profile=etag_flip", token, target, manifest, 0)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
            log = (root / "range.jsonl").read_text(encoding="utf-8")
            self.assertNotIn(token, log)
            self.assertNotIn(target, log)

    def test_stream_cache_is_bounded_restart_safe_private_and_revocable(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory) / "cache"
            secret = b"installation-secret-32-bytes-minimum-value"
            cache = stream_contract.BoundedPrivateChunkCache(
                root, secret, capacity=96, per_variant=64, pin_capacity=64, max_chunk=32,
            )
            a = b"a" * 32
            b = b"b" * 32
            c = b"c" * 32
            da, db, dc = map(stream_contract.digest_bytes, (a, b, c))
            key_a = cache.put("tenant-a", "variant", '"etag"', 0, a, da)
            key_other = cache.put("tenant-b", "variant", '"etag"', 0, b, db)
            self.assertNotEqual(key_a, key_other)
            self.assertFalse(any("tenant" in path.name or "variant" in path.name
                                 for path in root.glob("*.chunk")))
            cache.put("tenant-a", "variant", '"etag"', 1, b, db)
            cache.put("tenant-a", "variant", '"etag"', 2, c, dc)
            self.assertLessEqual(cache.total_bytes(), 96)
            with cache.open("tenant-a", "variant", '"etag"', 2, dc) as pinned:
                self.assertEqual(pinned.read(), c)
                self.assertEqual(cache.invalidate("tenant-a", "variant"), 2)
                with cache.open("tenant-a", "variant", '"etag"', 2, dc) as denied:
                    self.assertIsNone(denied)
                with self.assertRaises(stream_contract.AccessRevoked):
                    cache.put("tenant-a", "variant", '"new-etag"', 0, a, da)
                pinned.seek(0)
                self.assertEqual(pinned.read(), c)
            with cache.open("tenant-a", "variant", '"etag"', 2, dc) as removed:
                self.assertIsNone(removed)
            cache.close()

            restarted = stream_contract.BoundedPrivateChunkCache(
                root, secret, capacity=96, per_variant=64, pin_capacity=64, max_chunk=32,
            )
            def read_restarted(_index):
                with restarted.open("tenant-b", "variant", '"etag"', 0, db) as reused:
                    return reused.read()
            with ThreadPoolExecutor(max_workers=8) as pool:
                self.assertEqual(list(pool.map(read_restarted, range(16))), [b] * 16)
            with self.assertRaises(stream_contract.AccessRevoked):
                restarted.put("tenant-a", "variant", '"restart-etag"', 0, a, da)
            chunk_path = next(root.glob("*.chunk"))
            chunk_path.write_bytes(b"x" * chunk_path.stat().st_size)
            restarted.close()
            outside = pathlib.Path(directory) / "outside.chunk"
            outside.write_bytes(b"must-survive")
            connection = sqlite3.connect(root / "index.sqlite3")
            try:
                connection.execute("""INSERT INTO chunks VALUES(
                    ?,?,?,?,?,?,?,?,?,?)""", (
                    "d" * 64, "n" * 64, "v" * 64, "e" * 64, 99, 12,
                    stream_contract.digest_bytes(b"must-survive"), "../outside.chunk", 99, 0,
                ))
                connection.commit()
            finally:
                connection.close()
            repaired = stream_contract.BoundedPrivateChunkCache(
                root, secret, capacity=96, per_variant=64, pin_capacity=64, max_chunk=32,
            )
            self.assertEqual(repaired.total_bytes(), 0)
            self.assertEqual(outside.read_bytes(), b"must-survive")
            with self.assertRaises(stream_contract.ContractError):
                repaired.put("tenant", "variant", '"etag"', 0, b"x" * 33,
                             stream_contract.digest_bytes(b"x" * 33))
            repaired.close()
            link = pathlib.Path(directory) / "cache-link"
            try:
                link.symlink_to(root, target_is_directory=True)
            except (OSError, NotImplementedError):
                return
            with self.assertRaises(stream_contract.ContractError):
                stream_contract.BoundedPrivateChunkCache(link, secret)

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
