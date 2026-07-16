#!/usr/bin/env python3
"""Validate the shared Phase 2 long-track presentation/control model."""

from __future__ import annotations

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CONTRACT = ROOT / "protocol" / "pulsar-stream-track-ui-model-v1.json"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit(message)


def main() -> None:
    value = json.loads(CONTRACT.read_text(encoding="utf-8"))
    require(value["schema_version"] == 1, "unexpected schema version")
    require(value["contract_id"] == "pulsar.stream-track-ui-model.v1", "unexpected contract id")
    require(value["limits"] == {
        "maximum_file_bytes": 524_288_000,
        "maximum_duration_ms": 7_200_000,
        "maximum_targets": 64,
        "maximum_title_utf8_bytes": 512,
        "maximum_report_utf8_bytes": 2_000,
    }, "candidate limits changed")
    require(value["surface_states"] == ["loading", "ready", "stale", "offline", "coordinator_error"], "surface states changed")
    require(value["draft_phases"] == ["retained", "uploading", "uploaded", "processing", "ready", "failed"], "draft phases changed")
    require(value["playback_phases"] == ["idle", "queued", "loading", "ready", "playing", "paused", "seeking", "rebuffering", "ended", "failed"], "playback phases changed")
    require(value["audiences"] == ["current_air", "explicit"], "audiences changed")
    require(value["insertions"] == ["queue", "replace"], "insertions changed")
    require(value["actions"] == ["accept_policy", "upload", "retry", "delete", "queue", "replace", "pause", "seek", "resume", "report"], "actions changed")
    require(value["progress"]["fields_are_never_substituted_for_each_other"] is True, "progress fields can be conflated")
    require(value["draft"]["local_bytes_survive_stale_offline_and_missing-server replacement"] is True, "outage can discard draft")
    require(value["draft"]["local_bytes_auto_delete_before_persisted-server-confirmation"] is False, "draft can be deleted early")
    require(value["draft"]["confirmed_delete_requires_exact_local_id_echo"] is True, "delete confirmation lost exact local binding")
    require(value["draft"]["client_mime_can_mark_ready"] is False, "client MIME can manufacture readiness")
    require(value["selection"]["content_policy_must_be_current"] is True, "policy gate changed")
    require(value["generation"]["stale_replacement_is_discarded"] is True, "generation fence changed")
    require(value["commands"]["delete"]["does_not_optimistically_remove_draft"] is True, "delete became destructive")
    require(value["localization"]["authority"] == "coordinator/internal/presentation" and value["localization"]["clients_select_language_only"] is True, "localization authority changed")
    labels = value["localization"]["labels"]
    expected = len(value["draft_phases"]) + len(value["playback_phases"]) + len(value["actions"]) + len(value["failure_codes"])
    require(len(labels) == expected, "localized label inventory changed")
    require(value["platform_views_deferred_to"] == ["TASK-260712-3lximx", "TASK-260712-2psvhu"], "view boundary changed")
    print("pulsar stream-track UI model contract: OK")


if __name__ == "__main__":
    main()
