#!/usr/bin/env python3
"""Validate the shared Pulsar Phase 2 presentation/command boundary."""

from __future__ import annotations

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CONTRACT = ROOT / "protocol" / "pulsar-targets-inbox-presentation-v1.json"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit(message)


def main() -> None:
    value = json.loads(CONTRACT.read_text(encoding="utf-8"))
    require(value["schema_version"] == 1, "unexpected schema version")
    require(
        value["contract_id"] == "pulsar.targets-inbox-presentation.v1",
        "unexpected contract id",
    )
    require(
        value["surface_states"]
        == ["loading", "ready", "stale", "offline", "coordinator_error"],
        "surface states changed",
    )
    commands = value["commands"]
    require(
        list(commands)
        == [
            "refresh",
            "set_audience",
            "select_targets",
            "set_include_origin",
            "load_more_inbox",
            "load_more_history",
            "load_more_receipts",
            "replay_inbox",
            "dismiss_inbox",
            "delete_history",
            "report_inbox",
            "report_history",
            "mute_sender",
        ],
        "command inventory changed",
    )
    for name, command in commands.items():
        if name != "refresh":
            require(command["requires_state"] == ["ready"], f"{name} is not fail-closed")
    require(
        commands["mute_sender"]["wire_action"] == "block_actor",
        "mute must delegate to canonical block_actor",
    )
    require(
        commands["report_inbox"]["wire_object"]
        == "server-returned history_item_id"
        and commands["mute_sender"]["inbox_wire_object"]
        == "server-returned history_item_id",
        "inbox moderation lost its server-returned history capability",
    )
    require(
        value["routing"]["available_audiences_are_server_derived"] is True
        and commands["set_audience"]["requires_current_server_option"] is True,
        "audience authority changed",
    )
    target = value["target_choice"]
    require(target["selection_rule"].startswith("a command may return only"), "target authority changed")
    require(target["server_reauthorizes_on_send"] is True, "send reauthorization disabled")
    require(target["maximum_selection"] == 64, "target selection bound changed")
    require(
        value["playback"]["late_inbox_autoplay_command_exists"] is False
        and value["playback"]["missed_items_require_manual_replay"] is True
        and value["playback"]["read_or_pagination_triggers_playback"] is False,
        "late playback boundary changed",
    )
    require(
        value["localization"]["authority"] == "coordinator/internal/presentation"
        and value["localization"]["clients_select_language_only"] is True,
        "localization authority changed",
    )
    require(
        value["platform_views_deferred_to"]
        == ["TASK-260712-2nto40", "TASK-260712-cuplon"],
        "platform view task boundary changed",
    )
    print("pulsar targets/inbox presentation contract: OK")


if __name__ == "__main__":
    main()
