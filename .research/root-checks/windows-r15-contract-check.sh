#!/bin/sh
set -u

note=${1:-.research/260712-windows-appcontainer-capture-bridge.md}
failures=0

fail_if_present() {
    label=$1
    pattern=$2
    if rg -n -F "$pattern" "$note"; then
        echo "FAIL: $label"
        failures=$((failures + 1))
    else
        echo "PASS: $label"
    fi
}

fail_if_absent() {
    label=$1
    pattern=$2
    if rg -n -F "$pattern" "$note"; then
        echo "PASS: $label"
    else
        echo "FAIL: $label"
        failures=$((failures + 1))
    fi
}

fail_if_present "private TERMINAL is never numbered 5" 'private `TERMINAL`(5)'
fail_if_present "waiter never posts CLEANUP_READY with a pending registry" 'picker remains pending; waiter posts `WM_APP+CLEANUP_READY`'
fail_if_present "waiter never exits after a stalled operation" 'only allows the waiter to proceed to its exit'
fail_if_present "destroy retry has no finite 100-attempt cap" 'up to 100 retries at 100 ms'
fail_if_present "Cancel is not promised to produce Canceled" 'handler fires with `AsyncStatus::Canceled`'
fail_if_present "final summary does not recreate bounded quit" 'waits for terminal with 5-second per-operation timeout'
fail_if_present "thread duplicate is not lazily made at session start" 'thread duplicates `notifyEvent` to a local stack copy at session start'
fail_if_present "unknown CaptureRequestStop opId has one HRESULT policy" 'unknown/released `opId` returns `E_INVALIDARG`'

fail_if_absent "late terminal test at 6 seconds exists" 'late terminal at 6 seconds'
fail_if_absent "late terminal test at 15 seconds exists" 'late terminal at 15 seconds'
fail_if_absent "late terminal test at 29 seconds exists" 'late terminal at 29 seconds'
fail_if_absent "successful destroy defeats the watchdog" 'atomically cancel/defeat the watchdog'
fail_if_absent "CapturePrepare DuplicateHandle failure is an executable branch-table row" '| `CapturePrepare`: `DuplicateHandle` failure |'
fail_if_absent "CaptureActivate DuplicateHandle failure is an executable branch-table row" '| `CaptureActivate`: `DuplicateHandle` failure |'

if [ "$failures" -ne 0 ]; then
    echo "RESULT: FAIL ($failures contract contradictions)"
    exit 1
fi

echo "RESULT: PASS"
