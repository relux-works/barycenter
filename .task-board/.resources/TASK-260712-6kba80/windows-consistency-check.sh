#!/usr/bin/env bash
# Static consistency checker for the Windows AppContainer capture-bridge note.
# Asserts the root-reviewed Rev 16 invariants against authoritative note bytes.
# Each check prints the offending lines and increments a failure counter.
# Exit non-zero if any anti-pattern survives. Run it against the final note
# BEFORE claiming consistency (R13-6 / R14-5).
#
# Usage: bash windows-consistency-check.sh [path-to-note.md]
set -uo pipefail
NOTE="${1:-.research/260712-windows-appcontainer-capture-bridge.md}"
fail=0

# The changelog/history and this checker's own regex strings legitimately quote
# the anti-patterns. We therefore scope checks to NORMATIVE lines only by
# excluding: (a) the revision-history preamble (everything before the first
# "## " top-level section), and (b) lines that are explicit negations.
# Body = normative content after the preamble.
BODY_START=$(grep -n '^## ' "$NOTE" | head -1 | cut -d: -f1)
FINAL_START=$(grep -n '^## Final answer to the task' "$NOTE" | cut -d: -f1)
body() { tail -n +"${BODY_START}" "$NOTE"; }
final_body() { tail -n +"${FINAL_START}" "$NOTE"; }

check() {
  # $1 = human label; $2 = extended-regex anti-pattern; $3 = grep -v negation filter (may be empty)
  local label="$1" pat="$2" neg="${3:-}"
  local hits
  if [ -n "$neg" ]; then
    hits=$(body | grep -nE "$pat" | grep -vE "$neg")
  else
    hits=$(body | grep -nE "$pat")
  fi
  if [ -n "$hits" ]; then
    echo "FAIL: $label"
    echo "$hits" | sed 's/^/    /'
    fail=$((fail+1))
  else
    echo "PASS: $label"
  fi
}

require() {
  local label="$1" pat="$2"
  local hits
  hits=$(body | grep -nE "$pat")
  if [ -z "$hits" ]; then
    echo "FAIL: $label"
    fail=$((fail+1))
  else
    echo "PASS: $label"
  fi
}

require_final() {
  local label="$1" pat="$2"
  local hits
  hits=$(final_body | grep -nE "$pat")
  if [ -z "$hits" ]; then
    echo "FAIL: $label"
    fail=$((fail+1))
  else
    echo "PASS: $label"
  fi
}

echo "=== Windows capture-bridge consistency check ==="
echo "note: $NOTE   (body starts at note line $BODY_START)"
echo

# R14-1: no legacy private FSM state names in normative body.
check "no legacy 'IDLE' private state (R14-1)" \
  '\bIDLE\b' 'no .IDLE. state|There is no'
check "no legacy 'STOPPED' terminal state name (R14-1: TERMINAL/SEALED)" \
  '\bSTOPPED\b'

# R14-2: no worker signaling Go's ORIGINAL notifyEvent handle; only localNotify.
check "no worker SetEvent(notifyEvent) — must be SetEvent(localNotify) (R14-2)" \
  'SetEvent\(notifyEvent\)'
# R14-2: no stale readyEvent signal object.
check "no stale readyEvent (R14-2)" \
  '\breadyEvent\b'

# R14-3: no blocking Sleep in the wndproc retry path.
check "no Sleep(...) blocking retry in wndproc (R14-3)" \
  'Sleep\([0-9]'

# R14-1/R12-1: no separate 'cancelled bit' as a live mechanism (negations,
# history, and the note's own embedded grep-fixture text are allowed).
check "no live 'cancelled bit' mechanism (R12-1/R14-1)" \
  'cancelled bit' 'no separate cancelled bit|no .cancelled. bit|previously read or wrote|grep|returns a non-zero count'

# R13-7/R14: permission_revoke is directly assigned priority/rank 3, never 2.
# Bounded proximity so a correct "...priority 2, and permission_revoke priority 3"
# line does not false-positive.
check "no permission_revoke rank/priority 2 (R13-7)" \
  'permission_revoke[^,.]{0,15}(rank|priority) 2\b'

# R13-5/R14-5: Go is the SOLE sidecar writer AFTER terminal; no helper/native writer,
# no "sidecar is durable before terminal" claim (negations / fail-closed edge rows allowed).
check "no helper/native sidecar writer (R13-5)" \
  '(capture thread|helper|native).{0,40}writes.{0,20}(sidecar|\.reason)'
check "no 'sidecar durable before terminal' claim (R13-5)" \
  '(sidecar[^.]{0,40}(durable|written|persisted)[^.]{0,20}before terminal|before terminal[^.]{0,40}(durable|written|persisted)[^.]{0,20}sidecar)' \
  'cannot write|has no sidecar|no sidecar|hasn.t been notified'

# R14-4: cancel terminal HRESULT must be ERROR_CANCELLED, never hresult=0.
check "no 'hresult=0' claim for cancel (R14-4)" \
  'cancel.{0,30}hresult ?== ?0|hresult ?== ?0.{0,30}cancel'

# R15-1: five seconds is UI/logging only. The owner never exits/posts cleanup
# with pending state, and Cancel is not promised to choose Canceled/dismiss UI.
check "no bounded per-operation graceful-quit timeout (R15-1)" \
  '5-second per-operation timeout|with a 5-second timeout|up to 100 retries'
check "no timeout-to-waiter-exit/cleanup path (R15-1)" \
  'only allows the waiter to proceed to its exit|pending.{0,80}posts `WM_APP\+CLEANUP_READY`|waiter still posts `WM_APP\+CLEANUP_READY`'
check "no overclaim that Cancel forces Canceled/dismissal (R15-1)" \
  'handler fires with `AsyncStatus::Canceled`|Completed handler fires with cancelled|cancel dismisses'
require "late terminal tests at 6/15/29 seconds exist (R15-1)" \
  'late terminal at (6|15|29) seconds'
require "never-terminal test forbids CLEANUP_READY (R15-1)" \
  'Never-terminal operation.*no `CLEANUP_READY`'

# R15-2: graceful completion and force exit are atomically exclusive.
require "watchdog/graceful exit uses atomic CAS (R15-2)" \
  'CompareAndSwap\(exitGracefulPending, exitGracefulComplete\)'
require "watchdog force commit uses atomic CAS (R15-2)" \
  'CompareAndSwap\(exitGracefulPending, exitForceCommitted\)'

# R15-3: both eager duplicate failures and lost-CAS cleanup are executable.
require "CapturePrepare DuplicateHandle failure row exists (R15-3)" \
  '\| `CapturePrepare`: `DuplicateHandle` failure \|'
require "CaptureActivate DuplicateHandle failure row exists (R15-3)" \
  '\| `CaptureActivate`: `DuplicateHandle` failure \|'
require "callback duplicate lost-CAS close row exists (R15-3)" \
  'Callback duplicate created, then `PREPARED→ACTIVATING` CAS loses'
check "capture thread never lazily duplicates at session start (R15-3)" \
  'thread duplicates `notifyEvent` to a local stack copy at session start|thread copies notifyEvent.{0,30}DuplicateHandle'
check "workers never claim to signal Go's original event handle (R15-3)" \
  '(thread|callback|worker|helper).*signals Go.s `notifyEvent`|(thread|callback|worker).*signals Go.s original' \
  'never|No worker|without using'
require "CRT thread launch handle has an exact close owner (R15-3)" \
  'successful `_beginthreadex` handle.*short-lived launch artifact|closes the returned kernel thread HANDLE exactly once'
require "early thread execution has a creator-held lifetime fence (R15-3)" \
  'local creator `shared_ptr` from allocation through registry publication'
check "raw CreateThread is not selected for the CRT-using helper" \
  'launch(es|ed|ing)? (the |a )?(capture )?thread (with|via|using) (raw )?`?CreateThread|`CreateThread` launch handle' \
  'not raw'

# R15-4: one numeric private FSM and caller-buffer-safe readiness.
check "private TERMINAL is never 5 (R15-4)" \
  'private `TERMINAL`\(5\)|TERMINAL=5'
require "private canonical FSM includes SEALED=5 TERMINAL=6 (R15-4)" \
  'STOPPING=4, SEALED=5, TERMINAL=6'
check "PREPARING has no alternative private number (R15-4)" \
  'PREPARING[` ]*=[` ]*[1-9]'
check "PREPARED has no alternative private number (R15-4)" \
  'PREPARED[` ]*=[` ]*(0|[2-9])'
check "ACTIVATING has no alternative private number (R15-4)" \
  'ACTIVATING[` ]*=[` ]*(0|1|[3-9])'
check "CAPTURING has no alternative private number (R15-4)" \
  'CAPTURING[` ]*=[` ]*(0|1|2|[4-9])'
check "STOPPING has no alternative private number (R15-4)" \
  'STOPPING[` ]*=[` ]*(0|1|2|3|[5-9])'
check "SEALED has no alternative private number (R15-4)" \
  'SEALED[` ]*=[` ]*(0|1|2|3|4|[6-9])'
check "worker never release-stores caller format.ready (R15-4)" \
  'release-stores `?format->ready=1|Set format->ready=1'
require "session-owned mtaReady copy contract exists (R15-4)" \
  'session\.mtaReady|session-owned monotonic `mtaReady=1`'
check "unknown stop ID is not E_INVALIDARG (R15-4)" \
  'unknown/released `opId` returns `E_INVALIDARG`'
require "canonical public enum exists (R15-4)" \
  '`preparing`=0, `activating`=1, `capturing`=2, `stopped`=3, `failed`=4, `cancelled`=5'
check "preparing has no alternative public number (R15-4)" \
  '`preparing`=[1-9]|preparing=[1-9]'
check "activating has no alternative public number (R15-4)" \
  '`activating`=(0|[2-9])|activating=(0|[2-9])'
check "capturing has no alternative public number (R15-4)" \
  '`capturing`=(0|1|[3-9])|capturing=(0|1|[3-9])'
check "stopped has no alternative public number (R15-4)" \
  '`stopped`=(0|1|2|[4-9])|stopped=(0|1|2|[4-9])'
check "failed has no alternative public number (R15-4)" \
  '`failed`=(0|1|2|3|[5-9])|failed=(0|1|2|3|[5-9])'
check "cancelled has no alternative public number (R15-4)" \
  '`cancelled`=(0|1|2|3|4|[6-9])|cancelled=(0|1|2|3|4|[6-9])'

# R15-5/R15-6: sidecar and priority summaries must agree with canonical rows.
require "cancel sidecar HRESULT is ERROR_CANCELLED (R15-5)" \
  'CAP_REASON_CANCEL.*ERROR_CANCELLED|ERROR_CANCELLED.*never zero'
require "discontinuity sidecar HRESULT is ERROR_INVALID_DATA (R15-5)" \
  'CAP_REASON_DISCONTINUITY.*ERROR_INVALID_DATA'
check "discontinuity never reuses overflow HRESULT (R15-5)" \
  'DISCONTINUITY.*ERROR_BUFFER_OVERFLOW|DATA_DISCONTINUITY.*ERROR_BUFFER_OVERFLOW'
check "internal failures never seal directly from CAPTURING (R11-2/R15-6)" \
  'conversion failure.*packed CAS seal `SEALED`|CAPTURING[→-]+SEALED'
check "HRESULT is not claimed to live in the 64-bit packed word (R15-5)" \
  'hresult.{0,40}populated from the sealed packed word|packed word.{0,40}contains.{0,20}hresult'
check "rank-4 tie is never called tie-free (R15-6)" \
  'wasapi_error/format_error\(4\).{0,80}no ties'
check "unresolved proof does not undercount HRESULT branches (R15-6)" \
  'run the 11 branch tests'
check "unresolved proof does not undercount FSM scenarios (R15-6)" \
  'Run all 12 deterministic transition tests'

# The handoff summary is checked independently so a correct body cannot hide a
# stale final answer.
require_final "final summary retains no-timeout waiter ownership (R15-1)" \
  'Five seconds only'
require_final "final summary retains watchdog CAS gate (R15-2)" \
  'exitGracefulPending→exitGracefulComplete'
require_final "final summary retains private FSM start (R15-4)" \
  'PREPARING=0, PREPARED=1, ACTIVATING=2, CAPTURING=3'
require_final "final summary retains private FSM terminal values (R15-4)" \
  'SEALED=5, TERMINAL=6'
require_final "final summary retains public enum start (R15-4)" \
  'preparing=0, activating=1'
require_final "final summary retains public enum terminal values (R15-4)" \
  'capturing=2, stopped=3, failed=4, cancelled=5'
require_final "final summary retains rank-4 first-installed tie (R15-6)" \
  'wasapi_error/format_error\(4, first-installed tie\)'
require_final "final summary retains cancel HRESULT (R15-5)" \
  'cancel=`0x800704C7`'
require_final "final summary keeps HRESULT outside packed word (R15-5)" \
  'packed word contains state/reason but no HRESULT'
require_final "final summary retains CRT-safe thread launcher (R15-3)" \
  'launched with `_beginthreadex`'
require_final "final summary retains early-start creator hold (R15-3)" \
  'creator hold'
require_final "final summary retains thread-handle exact close owner (R15-3)" \
  'closed exactly once by the UI helper'

echo
if [ "$fail" -eq 0 ]; then
  echo "RESULT: PASS (0 anti-patterns in normative body)"
  exit 0
else
  echo "RESULT: FAIL ($fail checks failed)"
  exit 1
fi
