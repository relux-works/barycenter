#!/usr/bin/env python3
"""Validate checked-in sanitized integrated capture-quality evidence."""

from __future__ import annotations

import argparse
import json
import pathlib

import run_integrated


ROOT = pathlib.Path(__file__).resolve().parents[2]
EVIDENCE_PATH = ROOT / "acceptance/phase3/capture-quality-integrated-regressions-v1.json"


def load(path: pathlib.Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--evidence", type=pathlib.Path, default=EVIDENCE_PATH)
    args = parser.parse_args()
    evidence = load(args.evidence)
    run_integrated.validate_evidence(evidence)
    print(json.dumps({
        "build": evidence["build"],
        "cells": evidence["matrix"]["cells"],
        "fixtureRuns": evidence["matrix"]["fixtureRuns"],
        "manualEvidence": evidence["decision"]["manualEvidence"],
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
