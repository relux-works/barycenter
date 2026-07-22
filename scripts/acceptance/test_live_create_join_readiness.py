import copy
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import validate_live_create_join_readiness as readiness  # noqa: E402


def valid_manifest() -> dict:
    return {
        "schemaVersion": 1,
        "task": readiness.TASK_ID,
        "status": "pass",
        "manualEvidence": "not-run",
        "manualPassClaimed": False,
        "manualRowsPassed": [],
        "coordinator": {
            "endpoint": "https://barycenter.relux.works",
            "probeMethod": "GET-only",
            "health": {
                "httpStatus": 200,
                "status": "ok",
                "version": readiness.COORDINATOR_VERSION,
                "orbits": 3,
                "nodesConnected": 0,
            },
            "routes": copy.deepcopy(readiness.ROUTE_CODES),
        },
        "macos": {
            "installedPath": "/Applications/Pulsar.app",
            "sourceCommit": readiness.MAC_SOURCE,
            "bundleIdentifier": "works.relux.pulsar",
            "version": "0.3.0",
            "build": "946",
            "architecture": "x86_64",
            "signature": {
                "codesignVerified": True,
                "localTestOnly": True,
                "notarized": False,
                "gatekeeperExitCode": 3,
            },
            "hashes": copy.deepcopy(readiness.MAC_HASHES),
            "tests": {
                "swiftTestExitCode": 0,
                "releaseBuildExitCode": 0,
                "focusedFirstRunExitCode": 0,
            },
            "package": {"archiveIntegrityExitCode": 0},
            "ordinaryLaunch": {
                "openExitCode": 0,
                "parentPID": 1,
                "finishedLaunching": True,
                "visibleWindowCount": 1,
                "responsive": True,
                "newCrashReports": 0,
            },
            "firstRun": {"credentialsPresent": False, "createJoinContract": True},
        },
        "windows": {
            "host": "DESKTOP-3PBO632",
            "osBuild": "10.0.19045.0",
            "sourceCommit": readiness.WINDOWS_SOURCE,
            "packageFullName": (
                "ReluxWorksLLC.PulsarBarycenter_0.1.20.0_x64__q036g2bzd7ngc"
            ),
            "version": "0.1.20.0",
            "packageStatus": "Ok",
            "signatureKind": "Developer",
            "packageArchiveRehashedNow": False,
            "hashes": copy.deepcopy(readiness.WINDOWS_HASHES),
            "installedComponentsRehashedNow": True,
            "unpaired": {
                "protectedCredentialsPresent": False,
                "legacyCredentialsPresent": False,
            },
            "ordinaryLaunch": {
                "desktopShortcut": True,
                "parentProcess": "explorer.exe",
                "sessionId": 1,
                "responding": True,
                "hung": False,
                "applicationCrashEvents": 0,
                "appModelRemovalEvents": 0,
            },
            "joinSurface": {
                "navigationAutomationId": "3003",
                "navigationControlType": "ControlType.Pane",
                "inputAutomationId": "3027",
                "actionAutomationId": "3010",
                "navigationInvoked": True,
                "navigationNativeClass": "Button",
                "navigationNativeClickCompleted": True,
                "inputVisible": True,
                "inputEnabled": True,
                "inputKeyboardFocusable": False,
                "inputValuePatternAvailable": False,
                "inputControlType": "ControlType.Pane",
                "inputNativeClass": "Edit",
                "inputNativeVisible": True,
                "inputNativeEnabled": True,
                "inputNativeTabStop": True,
                "actionVisible": True,
                "actionEnabled": True,
                "actionControlType": "ControlType.Pane",
                "actionInvokePatternAvailable": False,
                "actionNativeClass": "Button",
                "actionNativeVisible": True,
                "actionNativeEnabled": True,
                "uiaSemanticStatus": "unexpected-pane-no-patterns",
                "invitationEntered": False,
                "joinActionInvoked": False,
            },
        },
        "ownerHandoff": {
            "task": readiness.OWNER_TASK_ID,
            "owner": "Ivan Oparin",
            "terminalRequired": False,
            "terminalCommands": [],
            "screens": [
                {"platform": "macos", "actions": copy.deepcopy(readiness.MAC_ACTIONS)},
                {
                    "platform": "windows",
                    "actions": copy.deepcopy(readiness.WINDOWS_ACTIONS),
                },
            ],
            "report": "visible_result_only",
        },
        "limitations": sorted(readiness.LIMITATIONS),
    }


class LiveCreateJoinReadinessTests(unittest.TestCase):
    def test_valid_manifest_is_accepted(self):
        readiness.validate(valid_manifest())

    def test_manual_pass_is_rejected(self):
        data = valid_manifest()
        data["manualPassClaimed"] = True
        with self.assertRaisesRegex(ValueError, "manual PASS"):
            readiness.validate(data)

    def test_route_or_candidate_drift_is_rejected(self):
        data = valid_manifest()
        data["coordinator"]["routes"]["consumeInvite"] = 404
        with self.assertRaisesRegex(ValueError, "route registration"):
            readiness.validate(data)

        data = valid_manifest()
        data["windows"]["hashes"]["executable"] = "0" * 64
        with self.assertRaisesRegex(ValueError, "accepted candidate"):
            readiness.validate(data)

    def test_real_join_action_is_rejected(self):
        data = valid_manifest()
        data["windows"]["joinSurface"]["invitationEntered"] = True
        with self.assertRaisesRegex(ValueError, "entered an invitation"):
            readiness.validate(data)

        data = valid_manifest()
        data["windows"]["joinSurface"]["joinActionInvoked"] = True
        with self.assertRaisesRegex(ValueError, "performed the real Join"):
            readiness.validate(data)

    def test_windows_uia_anomaly_must_be_exactly_disclosed(self):
        data = valid_manifest()
        data["windows"]["joinSurface"]["inputKeyboardFocusable"] = True
        with self.assertRaisesRegex(ValueError, "focus anomaly"):
            readiness.validate(data)

        data = valid_manifest()
        data["windows"]["joinSurface"]["actionControlType"] = "ControlType.Button"
        with self.assertRaisesRegex(ValueError, "action UIA type"):
            readiness.validate(data)

    def test_owner_scope_is_exactly_two_no_terminal_screens(self):
        data = valid_manifest()
        data["ownerHandoff"]["terminalCommands"] = ["ssh mbpro-win"]
        with self.assertRaisesRegex(ValueError, "terminal commands"):
            readiness.validate(data)

        data = valid_manifest()
        data["ownerHandoff"]["screens"].append({"platform": "manual", "actions": []})
        with self.assertRaisesRegex(ValueError, "exactly two screens"):
            readiness.validate(data)

    def test_windows_probe_navigates_but_cannot_submit(self):
        source = (HERE / "windows_create_join_readiness.ps1").read_text(encoding="utf-8")
        self.assertIn("ClickButton($joinNavigationHandle, 2000)", source)
        self.assertNotIn("ClickButton($joinActionHandle", source)
        self.assertNotIn(".SetValue(", source)
        self.assertIn("invitationEntered = $false", source)
        self.assertIn("joinActionInvoked = $false", source)
        self.assertIn("Get-CredentialPosture", source)


if __name__ == "__main__":
    unittest.main()
