import pathlib
import sys
import tempfile
import unittest


HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import run_desktop_ui as harness  # noqa: E402


class DesktopUIAcceptanceTests(unittest.TestCase):
    def test_sources_are_regular_hashed_files(self):
        records = harness.source_records()
        self.assertEqual(len(records), len(harness.SOURCE_PATHS))
        self.assertGreaterEqual(len(records), 15)
        for record in records:
            self.assertRegex(record["sha256"], r"^[0-9a-f]{64}$")
            self.assertGreater(record["bytes"], 0)
            self.assertNotIn(".temp/", record["path"])

    def test_commands_cover_platform_tests_cross_builds_and_release(self):
        with tempfile.TemporaryDirectory() as directory:
            commands = harness.commands(
                pathlib.Path(directory),
                {"GOTOOLCHAIN": "go1.25.12"},
                {"DEVELOPER_DIR": "/Applications/Xcode.app/Contents/Developer"},
            )
        names = [command.name for command in commands]
        for required in (
            "desktop-contract-tests",
            "windows-vet",
            "windows-tests",
            "windows-race",
            "windows-cross-vet-amd64",
            "windows-cross-build-amd64",
            "windows-cross-build-arm64",
            "windows-gui-build-amd64",
            "windows-gui-build-arm64",
            "swift-tests",
            "swift-release-build",
        ):
            self.assertIn(required, names)
        release = next(command for command in commands if command.name == "swift-release-build")
        self.assertEqual(release.argv, ("xcrun", "swift", "build", "-c", "release"))

    def test_pe_parser_accepts_gui_and_rejects_console(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "fixture.exe"
            image = bytearray(512)
            image[:2] = b"MZ"
            image[0x3C:0x40] = (0x80).to_bytes(4, "little")
            image[0x80:0x84] = b"PE\0\0"
            image[0x80 + 24 + 68:0x80 + 24 + 70] = (2).to_bytes(2, "little")
            path.write_bytes(image)
            self.assertEqual(harness.pe_subsystem(path), 2)
            image[0x80 + 24 + 68:0x80 + 24 + 70] = (3).to_bytes(2, "little")
            path.write_bytes(image)
            with self.assertRaisesRegex(RuntimeError, "expected GUI"):
                harness.pe_subsystem(path)

    def test_manual_boundary_is_explicit_in_source(self):
        source = pathlib.Path(harness.__file__).read_text(encoding="utf-8")
        self.assertIn('"manualEvidence": "not-run"', source)
        self.assertIn("No physical DPI, Retina, Narrator, VoiceOver", source)
        self.assertIn("not a notarized application bundle", source)


if __name__ == "__main__":
    unittest.main()
