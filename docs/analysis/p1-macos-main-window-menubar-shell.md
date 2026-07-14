# Phase 1 macOS main-window and menu-bar shell

- Task: `TASK-260712-1c04pk`
- Scope: best-effort macOS 14 code, deterministic unit/static evidence
- Manual evidence: `EPIC-260714-th54l3`

## Information architecture

The shell has one shared observable projection and two presentations. The main
window uses a native SwiftUI `NavigationSplitView`; the status item remains an
AppKit `NSStatusItem` because the existing executable owns an AppKit lifecycle,
Sparkle controller, onboarding window, signal handlers and long-lived audio
runtime. A minimal `NSHostingController` is the only bridge. Migrating the
whole process to a SwiftUI `App` solely to obtain `WindowGroup` would change
those lifecycle guarantees and is outside this shell task.

| Surface | Always reachable paths | Runtime projection | Later integration seam |
|---|---|---|---|
| Main window | Home, Create, Join, Try locally, History, Settings | paired/reconnecting/online/degraded, route, now playing, DND, volume and recording state | capture, self-test, HTTP history/presence and settings persistence |
| Menu bar | Open window, Create, Join, Try locally, recording, DND, output, re-pair, policy/support, update and quit | the same shared connection, DND and recording snapshot plus authoritative player status | capture command and richer coordinator data |
| Main menu | Window and Actions commands | action enablement follows paired and recording capability state | no second command implementation |

The Home screen keeps the three primary actions above status. Presence,
routing and now-playing are separate accessible cards. Local controls and the
history preview follow them. Create, Join, Try locally, History and Settings
also have dedicated destinations, so an empty or unavailable feature is never
mistaken for a missing navigation path.

## State and failure behavior

The UI uses text plus an SF Symbol for every connection and recording state;
color is never the only signal. English and Russian catalogs cover every shell
key and every sidebar destination. Dynamic runtime strings remain data, not
localization keys.

| State | Main window | Menu bar | Actions deliberately retained |
|---|---|---|---|
| Unpaired | explicit “Not paired”/“Не подключён” banner and pairing help | open/create/join/local/settings remain present | Create, Join, Try locally, History, Settings |
| Reconnecting/degraded | visible text-and-symbol warning | reconnecting/attention text | local navigation, output, settings, DND state visibility |
| Recording | `record.circle.fill`, explicit recording text and Stop wording | Stop replaces Start | Stop remains enabled even if ordinary recording availability drops |
| Capture/self-test not wired yet | visible destination and explanatory unavailable copy | command remains discoverable; fake success is forbidden | later tasks enable the existing seams |

`muted_until` is rendered if received, but the shell does not invent a mute
deadline: that Picker row is disabled until a later settings task supplies an
explicit duration. `allow_all` and `messages_only` call the existing durable
node-local DND API. Volume calls the existing player control. Coordinator
health is true only after `welcome` on the current socket and is cleared on
stop/reconnect, rather than being hard-coded true.

## Keyboard and accessibility contract

| Shortcut | Action |
|---|---|
| `⌘0` | open/raise the main window |
| `⌘1` | Create |
| `⌘2` | Join |
| `⇧⌘T` | Try locally |
| `⇧⌘R` | start/stop recording when the capability is available |
| `⇧⌘D` | toggle local DND between allow-all and messages-only when paired |
| `⌘,` | Settings |
| `⌘Q` | Quit |

Primary interactions are `Button`, `Picker` and `Slider`, not tap gestures.
Status groups combine their labels for VoiceOver, decorative meaning is not
encoded only by color, text uses system styles, and the split view/window have
minimum rather than fixed content sizes. The status item has an accessibility
description and the hosted window has an accessibility label.

## Automated evidence and honest boundary

`NodeAppUITests` proves complete EN/RU key coverage, stable DND wire values,
text-and-symbol semantics for every connection/recording state, navigation
survival across unpaired/degraded/recording states, and bounded runtime
projection. Release compilation covers the SwiftUI/AppKit bridge and executable
wiring. The existing full hosted `swift test` job is authoritative because the
standalone local Command Line Tools image does not ship the Swift Testing
module used by the repository's tests.

No screenshot, live VoiceOver traversal, real menu interaction, microphone,
audible output, packaged application, signing/notarization or physical macOS
hardware result is claimed here. Those checks remain in the manual-test epic.
