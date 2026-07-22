#!/usr/bin/env python3
"""Fail-closed validation for the live Mac-create/Windows-join handoff."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
HEX40 = re.compile(r"^[0-9a-f]{40}$")
HEX64 = re.compile(r"^[0-9a-f]{64}$")

TASK_ID = "TASK-260722-1zv67l"
OWNER_TASK_ID = "TASK-260721-ryk8c0"
COORDINATOR_VERSION = "git-3565c1e1ca0511168026ec2ba72440d23fb1317f"
MAC_SOURCE = "fb807e1caa40ebb7d206d983e234b626f4457945"
WINDOWS_SOURCE = "76f09a4d8be693d57cd5d47b9b9e5ac06196519c"

MAC_HASHES = {
    "nodeApp": "a862bfd563ef9956527ad5704e290966b8d8922cea3dbdd54cee2097f53fbabd",
    "goLibrespot": "a6a6808104129b18e2b660526e4d44c8d1731d89f2e62ea6a2cce30e09c7d61f",
    "infoPlist": "885b001d33a76ccf95e554e568594d9ae6037459592c45692dbf5d48ca429308",
    "reviewArchive": "87313d3a64821aebf76b4e8d993041819cd7f9f3df20082d7f95c6383cad6c67",
}
WINDOWS_HASHES = {
    "packageArchive": "f74b5c8d6f8c86443f8c1b64715977be1b0183c39e7fc4dde7567c957b958348",
    "executable": "0a77f53f026b77dd6abc3b265f18a8d32744847ca23571e97ddd999cc17a0042",
    "goLibrespot": "1967b76fc6e8e91763cea10c1cac1bb5f97cdb08a6100bdb27c9a01470cf84ca",
    "captureDLL": "8c1657d035ab738559c91c4c8468d6a4ba663a80dc96aab8951cc4c2d3b52c2f",
}
ROUTE_CODES = {
    "create": 400,
    "deviceInvite": 400,
    "consumeInvite": 400,
    "unknownControl": 404,
}
MAC_ACTIONS = [
    "open_create",
    "create",
    "save_recovery",
    "open_devices",
    "generate_invitation",
    "copy_code",
]
WINDOWS_ACTIONS = ["open_join", "paste_invitation", "join_securely"]
LIMITATIONS = {
    "macos-local-signature-not-notarized",
    "windows-local-developer-signature",
    "manual-create-invite-join-not-run",
    "hardware-audio-not-run",
}


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def load_strict(path: Path) -> dict:
    def no_duplicates(pairs: list[tuple[str, object]]) -> dict:
        result: dict[str, object] = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON key: {key}")
            result[key] = value
        return result

    return json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=no_duplicates)


def require_hashes(actual: dict, expected: dict[str, str], label: str) -> None:
    require(actual == expected, f"{label} hashes do not identify the accepted candidate")
    for name, value in actual.items():
        require(bool(HEX64.fullmatch(value)), f"{label}.{name} is not SHA-256")


def validate(data: dict) -> None:
    require(data.get("schemaVersion") == 1, "unsupported schemaVersion")
    require(data.get("task") == TASK_ID, "wrong readiness task")
    require(data.get("status") == "pass", "readiness status must be pass")
    require(data.get("manualEvidence") == "not-run", "manual evidence was inferred")
    require(data.get("manualPassClaimed") is False, "manual PASS must remain false")
    require(data.get("manualRowsPassed") == [], "manual rows cannot be pre-checked")

    coordinator = data["coordinator"]
    require(coordinator["endpoint"] == "https://barycenter.relux.works", "wrong coordinator")
    require(coordinator["health"]["httpStatus"] == 200, "coordinator health is not HTTP 200")
    require(coordinator["health"]["status"] == "ok", "coordinator status is not ok")
    require(coordinator["health"]["version"] == COORDINATOR_VERSION, "coordinator version drift")
    require(coordinator["health"]["orbits"] == 3, "live orbit count drift")
    require(coordinator["health"]["nodesConnected"] >= 0, "invalid connected-node count")
    require(coordinator["routes"] == ROUTE_CODES, "onboarding route registration drift")
    require(coordinator["probeMethod"] == "GET-only", "coordinator probe was mutating")

    mac = data["macos"]
    require(mac["installedPath"] == "/Applications/Pulsar.app", "wrong Mac install path")
    require(mac["sourceCommit"] == MAC_SOURCE, "Mac source drift")
    require(bool(HEX40.fullmatch(mac["sourceCommit"])), "invalid Mac source hash")
    require(mac["bundleIdentifier"] == "works.relux.pulsar", "wrong Mac bundle identifier")
    require(mac["version"] == "0.3.0" and mac["build"] == "946", "Mac version drift")
    require(mac["architecture"] == "x86_64", "Mac architecture drift")
    require(mac["signature"]["codesignVerified"] is True, "Mac codesign did not verify")
    require(mac["signature"]["localTestOnly"] is True, "Mac local-test boundary missing")
    require(mac["signature"]["notarized"] is False, "Mac notarization was inferred")
    require(mac["signature"]["gatekeeperExitCode"] == 3, "unexpected Gatekeeper boundary")
    require_hashes(mac["hashes"], MAC_HASHES, "macos")
    require(mac["tests"]["swiftTestExitCode"] == 0, "Swift tests did not pass")
    require(mac["tests"]["releaseBuildExitCode"] == 0, "Swift release build did not pass")
    require(mac["tests"]["focusedFirstRunExitCode"] == 0, "first-run UI contract did not pass")
    require(mac["package"]["archiveIntegrityExitCode"] == 0, "Mac archive integrity failed")
    launch = mac["ordinaryLaunch"]
    require(launch["openExitCode"] == 0, "ordinary Mac launch failed")
    require(launch["parentPID"] == 1, "Mac app was not launched by LaunchServices")
    require(launch["finishedLaunching"] is True, "Mac app did not finish launching")
    require(launch["visibleWindowCount"] >= 1, "Mac app has no visible product window")
    require(launch["responsive"] is True, "Mac app is not responsive")
    require(launch["newCrashReports"] == 0, "Mac launch produced a crash report")
    require(mac["firstRun"]["credentialsPresent"] is False, "Mac is no longer first-run")
    require(mac["firstRun"]["createJoinContract"] is True, "Mac Create/Join shell not proven")

    windows = data["windows"]
    require(windows["host"] == "DESKTOP-3PBO632", "wrong Windows host")
    require(windows["osBuild"] == "10.0.19045.0", "wrong Windows build")
    require(windows["sourceCommit"] == WINDOWS_SOURCE, "Windows source drift")
    require(bool(HEX40.fullmatch(windows["sourceCommit"])), "invalid Windows source hash")
    require(
        windows["packageFullName"]
        == "ReluxWorksLLC.PulsarBarycenter_0.1.20.0_x64__q036g2bzd7ngc",
        "installed Windows package drift",
    )
    require(windows["version"] == "0.1.20.0", "Windows version drift")
    require(windows["packageStatus"] == "Ok", "Windows package status is not Ok")
    require(windows["signatureKind"] == "Developer", "Windows signature boundary drift")
    require(windows["packageArchiveRehashedNow"] is False, "unavailable MSIX was rehashed by claim")
    require_hashes(windows["hashes"], WINDOWS_HASHES, "windows")
    require(windows["installedComponentsRehashedNow"] is True, "Windows files were not rehashed")
    require(windows["unpaired"]["protectedCredentialsPresent"] is False, "Windows is paired")
    require(windows["unpaired"]["legacyCredentialsPresent"] is False, "legacy credentials remain")
    win_launch = windows["ordinaryLaunch"]
    require(win_launch["desktopShortcut"] is True, "Windows shortcut launch was not used")
    require(win_launch["parentProcess"] == "explorer.exe", "Windows app parent is not Explorer")
    require(win_launch["sessionId"] == 1, "Windows app is outside the interactive session")
    require(win_launch["responding"] is True and win_launch["hung"] is False, "Windows app unhealthy")
    require(win_launch["applicationCrashEvents"] == 0, "Windows application crash recorded")
    require(win_launch["appModelRemovalEvents"] == 0, "Windows package removal recorded")
    join = windows["joinSurface"]
    require(join["navigationAutomationId"] == "3003", "wrong Join navigation control")
    require(join["inputAutomationId"] == "3027", "wrong invitation input control")
    require(join["actionAutomationId"] == "3010", "wrong Join action control")
    require(join["navigationInvoked"] is True, "Join screen was not opened")
    require(join["navigationNativeClass"] == "Button", "Join navigation is not a native Button")
    require(join["navigationNativeClickCompleted"] is True, "native Join navigation failed")
    require(join["inputVisible"] is True and join["inputEnabled"] is True, "Join input unavailable")
    require(join["inputNativeClass"] == "Edit", "Join input is not a native Edit")
    require(join["inputNativeVisible"] is True, "native Join input is not visible")
    require(join["inputNativeEnabled"] is True, "native Join input is not enabled")
    require(join["inputNativeTabStop"] is True, "native Join input is not keyboard reachable")
    require(join["actionVisible"] is True and join["actionEnabled"] is True, "Join action unavailable")
    require(join["actionNativeClass"] == "Button", "Join action is not a native Button")
    require(join["actionNativeVisible"] is True, "native Join action is not visible")
    require(join["actionNativeEnabled"] is True, "native Join action is not enabled")
    require(
        join["uiaSemanticStatus"] == "unexpected-pane-no-patterns",
        "UIA semantic anomaly was not disclosed",
    )
    require(join["navigationControlType"] == "ControlType.Pane", "Join UIA type drift")
    require(join["inputControlType"] == "ControlType.Pane", "input UIA type drift")
    require(join["actionControlType"] == "ControlType.Pane", "action UIA type drift")
    require(join["inputKeyboardFocusable"] is False, "input UIA focus anomaly drift")
    require(join["inputValuePatternAvailable"] is False, "input UIA pattern anomaly drift")
    require(join["actionInvokePatternAvailable"] is False, "action UIA pattern anomaly drift")
    require(join["invitationEntered"] is False, "readiness probe entered an invitation")
    require(join["joinActionInvoked"] is False, "readiness probe performed the real Join")

    handoff = data["ownerHandoff"]
    require(handoff["task"] == OWNER_TASK_ID, "wrong owner task")
    require(handoff["owner"] == "Ivan Oparin", "wrong owner")
    require(handoff["terminalRequired"] is False, "owner handoff requires a terminal")
    require(handoff["terminalCommands"] == [], "terminal commands leaked into owner handoff")
    require(len(handoff["screens"]) == 2, "owner handoff must contain exactly two screens")
    require(handoff["screens"][0]["platform"] == "macos", "first owner screen must be macOS")
    require(handoff["screens"][0]["actions"] == MAC_ACTIONS, "Mac owner actions drift")
    require(handoff["screens"][1]["platform"] == "windows", "second owner screen must be Windows")
    require(handoff["screens"][1]["actions"] == WINDOWS_ACTIONS, "Windows owner actions drift")
    require(handoff["report"] == "visible_result_only", "owner return request expanded")
    require(set(data["limitations"]) == LIMITATIONS, "readiness limitations drift")


def validate_repository_provenance() -> None:
    for revision in (MAC_SOURCE, WINDOWS_SOURCE):
        subprocess.run(
            ["git", "cat-file", "-e", f"{revision}^{{commit}}"],
            cwd=ROOT,
            check=True,
            capture_output=True,
        )
    subprocess.run(
        ["git", "diff", "--quiet", f"{MAC_SOURCE}..HEAD", "--", "node-app", "assets"],
        cwd=ROOT,
        check=True,
        capture_output=True,
    )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("path", type=Path)
    args = parser.parse_args()
    path = args.path if args.path.is_absolute() else ROOT / args.path
    validate(load_strict(path))
    validate_repository_provenance()
    print(f"live Create/Join readiness valid: {path}")


if __name__ == "__main__":
    main()
