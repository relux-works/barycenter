import Foundation
import Testing
@testable import NodeAppUI

@Suite("Pulsar macOS shell")
struct PulsarShellModelTests {
    @Test("English and Russian catalogs cover every shell key")
    func catalogsAreComplete() {
        for locale in PulsarShellLocale.allCases {
            let copy = PulsarShellCopy(locale: locale)
            for key in PulsarShellText.allCases {
                #expect(!copy.text(key).trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
            for section in PulsarShellSection.allCases {
                #expect(!copy.title(for: section).isEmpty)
            }
        }
        #expect(PulsarShellCopy(locale: .en).text(.connectionOnline) !=
                PulsarShellCopy(locale: .ru).text(.connectionOnline))
    }

    @Test("Canonical platform vocabulary is consumed by both shell locales")
    func canonicalPlatformVocabulary() throws {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let data = try Data(contentsOf:
            root.appendingPathComponent("assets/localization/platform-copy.json"))
        let contract = try #require(
            JSONSerialization.jsonObject(with: data) as? [String: [String: String]])
        let bindings: [(PulsarShellLocale, String, [String: PulsarShellText])] = [
            (.en, "en", [
                "create": .create, "join": .join, "try_locally": .tryLocally,
                "routing": .routing, "history": .history, "report": .report,
                "integrations": .integrations, "spotify_optional": .spotifyOptional,
                "telegram_optional": .telegramOptional,
            ]),
            (.ru, "ru", [
                "create": .create, "join": .join, "try_locally": .tryLocally,
                "routing": .routing, "history": .history, "report": .report,
                "integrations": .integrations, "spotify_optional": .spotifyOptional,
                "telegram_optional": .telegramOptional,
            ]),
        ]
        for (locale, language, keys) in bindings {
            let copy = PulsarShellCopy(locale: locale)
            for (contractKey, shellKey) in keys {
                #expect(copy.text(shellKey) == contract[language]?[contractKey])
            }
        }

        let main = try String(contentsOf:
            root.appendingPathComponent("node-app/Sources/NodeApp/main.swift"), encoding: .utf8)
        #expect(!main.contains("SpotifyHelp.presentHowToSound()"))
        for language in ["en", "ru"] {
            let plist = try String(contentsOf:
                root.appendingPathComponent("assets/macos/\(language).lproj/InfoPlist.strings"),
                encoding: .utf8)
            for key in ["microphone_usage", "local_network_usage", "apple_events_usage"] {
                #expect(plist.contains(try #require(contract[language]?[key])))
            }
        }
    }

    @Test("Every connection and recording state has non-color semantics")
    func stateSemanticsAreTextAndSymbolBased() {
        let copy = PulsarShellCopy(locale: .en)
        let connections: [PulsarConnectionState] = [
            .unpaired, .reconnecting, .online, .degraded("output unavailable"),
        ]
        for state in connections {
            #expect(!copy.connectionLabel(state).isEmpty)
            #expect(!copy.connectionSymbol(state).isEmpty)
        }

        let recordings: [PulsarRecordingState] = [
            .unavailable, .idle, .recording, .processing, .failed("permission denied"),
        ]
        for state in recordings {
            #expect(!copy.recordingLabel(state).isEmpty)
            #expect(!copy.recordingSymbol(state).isEmpty)
        }
        for state in [
            PulsarRecordingShortcutState.inactive, .registered, .conflict, .unavailable, .suspended,
        ] {
            #expect(!copy.recordingShortcutLabel(state).isEmpty)
        }
    }

    @MainActor
    @Test("Unpaired and degraded snapshots preserve shell navigation")
    func navigationSurvivesConnectionChanges() {
        let model = PulsarShellModel(locale: .en)
        #expect(model.snapshot.connection == .unpaired)
        #expect(PulsarShellSection.allCases == [
            .home, .airs, .inbox, .create, .join, .tryLocally, .history, .settings,
        ])

        model.selectedSection = .tryLocally
        model.updateConnection(.degraded("coordinator unavailable"), identity: "example · home")
        #expect(model.selectedSection == .tryLocally)
        #expect(model.snapshot.connection.isPaired)

        model.setRecording(.recording, available: true)
        model.selectedSection = .settings
        #expect(model.snapshot.recording == .recording)
        #expect(model.selectedSection == .settings)
    }

    @MainActor
    @Test("Runtime updates clamp volume and preserve unrelated shell state")
    func runtimeProjectionIsBounded() {
        let history = PulsarHistoryItem(
            id: "hi_public",
            title: "Voice message",
            detail: "Delivered",
            occurredAt: Date(timeIntervalSince1970: 1_700_000_000)
        )
        let model = PulsarShellModel(snapshot: .init(history: [history], volume: 80))

        model.updateRuntime(
            routeName: "Studio Display",
            nowPlaying: "Track — Artist",
            playbackState: "playing",
            dndMode: .messagesOnly,
            volume: 140
        )

        #expect(model.snapshot.routeName == "Studio Display")
        #expect(model.snapshot.nowPlaying == "Track — Artist")
        #expect(model.snapshot.playbackState == "playing")
        #expect(model.snapshot.dndMode == .messagesOnly)
        #expect(model.snapshot.volume == 100)
        #expect(model.snapshot.history == [history])
    }

    @MainActor
    @Test("Self-test projection clamps meter and preserves the complete file review")
    func selfTestProjection() {
        let model = PulsarShellModel(snapshot: .init(selfTestAvailable: true))
        let review = PulsarLocalFileReview(
            filename: "voice.wav",
            format: "wav",
            durationMs: 5_000,
            sizeBytes: 12_345,
            audience: ["this_pulsar"],
            deliveryModes: ["overlay", "interrupt"],
            rightsReminder: "Only audio you may share.",
            serverValidationRequired: true,
            rejection: nil)

        model.updateSelfTest(state: .recording, meter: 1.5, draftAvailable: false)
        model.setRecordingMeter(-0.5)
        model.setCaptureDevices([
            .init(id: "mic-1", name: "Studio Mic", isDefault: true),
        ], selectedDeviceID: "mic-1")
        model.setLocalFileReview(review)
        #expect(model.snapshot.selfTestState == .recording)
        #expect(model.snapshot.selfTestMeter == 1)
        #expect(model.snapshot.localFileReview == review)
        #expect(model.snapshot.localFileReview?.isEligible == true)
        #expect(!model.snapshot.localDraftAvailable)
        #expect(model.snapshot.recordingMeter == 0)
        #expect(model.snapshot.captureDevices.first?.name == "Studio Mic")
        #expect(model.snapshot.selectedCaptureDeviceID == "mic-1")
        for locale in PulsarShellLocale.allCases {
            #expect(!PulsarShellCopy(locale: locale).selfTestLabel(.recording).isEmpty)
        }
    }

    @MainActor
    @Test("Shortcut projection keeps configured chord and honest availability separate")
    func shortcutProjection() {
        let model = PulsarShellModel()
        model.setRecordingShortcut(.controlOptionSpace, state: .conflict)
        #expect(model.snapshot.recordingShortcut == .controlOptionSpace)
        #expect(model.snapshot.recordingShortcut.displayValue == "⌃⌥Space")
        #expect(model.snapshot.recordingShortcutState == .conflict)
        #expect(PulsarRecordingShortcutChoice.allCases.count == 4)
    }

    @Test("DND labels map the frozen wire values in both languages")
    func dndWireValuesStayStable() {
        #expect(PulsarDNDMode.allowAll.rawValue == "allow_all")
        #expect(PulsarDNDMode.messagesOnly.rawValue == "messages_only")
        #expect(PulsarDNDMode.mutedUntil.rawValue == "muted_until")
        for locale in PulsarShellLocale.allCases {
            let copy = PulsarShellCopy(locale: locale)
            for mode in PulsarDNDMode.allCases {
                #expect(!copy.dndLabel(mode).isEmpty)
            }
        }
    }

    @MainActor
    @Test("Stable action object forwards every shell capability seam")
    func actionsForwardWithoutOwningFeatureState() {
        var calls: [String] = []
        let actions = PulsarShellActions(
            createOrbit: { calls.append("create") },
            joinOrbit: { calls.append("join") },
            tryLocally: { calls.append("self-test") },
            setDND: { calls.append($0.rawValue) },
            setVolume: { calls.append("volume:\($0)") },
            toggleRecording: { calls.append("record") },
            cancelRecording: { calls.append("cancel-record") },
            setCaptureDevice: { calls.append("input:\($0 ?? "default")") },
            setRecordingShortcut: { calls.append("shortcut:\($0.rawValue)") },
            playBuiltinCue: { calls.append("cue") },
            recordFiveSeconds: { calls.append("five") },
            reviewLocalFile: { calls.append("review:\($0.lastPathComponent)") },
            acceptLocalFile: { calls.append("accept:\($0.lastPathComponent)") },
            deleteLocalDraft: { calls.append("delete") },
            closeSelfTest: { calls.append("close") },
            sendDraft: {
                calls.append("send:\($0):\($1.rawValue):\($2.rawValue):rights=\($3)")
            },
            deleteOutgoingDraft: { calls.append("delete-outbox:\($0)") },
            refreshPhaseOneData: { calls.append("refresh-data") },
            historyAction: { calls.append("history:\($0):\($1.action.rawValue)") },
            submitCreateOrbit: { calls.append("create-api:\($0)") },
            submitJoinOrbit: { calls.append("join-api:\($0)") },
            exportRecovery: { calls.append("export-recovery") },
            refreshAirs: { calls.append("air-refresh") },
            createAir: { calls.append("air-create:\($0)") },
            consumeAirInvite: { calls.append("air-consume:\($0)") },
            confirmAirJoin: { calls.append("air-confirm:\($0):\($1)") },
            declineAirJoin: { calls.append("air-decline:\($0)") },
            issueAirInvite: { calls.append("air-invite:\($0):\($1.rawValue)") },
            withdrawAirInvite: { calls.append("air-withdraw") },
            hideAirInvite: { calls.append("air-hide") },
            activateAir: { calls.append("air-activate:\($0)") },
            deactivateAir: { calls.append("air-deactivate:\($0)") },
            leaveAir: { calls.append("air-leave:\($0)") },
            dissolveAir: { calls.append("air-dissolve:\($0)") },
            replaceAirPolicy: { calls.append("air-policy:\($0):\($1.revision)") }
        )

        actions.createOrbit()
        actions.joinOrbit()
        actions.tryLocally()
        actions.setDND(.messagesOnly)
        actions.setVolume(45)
        actions.toggleRecording()
        actions.cancelRecording()
        actions.setCaptureDevice("mic-1")
        actions.setRecordingShortcut(.controlShiftR)
        let file = URL(fileURLWithPath: "/tmp/voice.wav")
        actions.playBuiltinCue()
        actions.recordFiveSeconds()
        actions.reviewLocalFile(file)
        actions.acceptLocalFile(file)
        actions.deleteLocalDraft()
        actions.closeSelfTest()
        actions.sendDraft(
            "draft-1", route: .ownBarycenter, delivery: .overlay,
            rightsAcknowledged: true)
        actions.deleteOutgoingDraft("draft-1")
        actions.refreshPhaseOneData()
        actions.performHistoryAction("history-1", action: .blockActor)
        actions.performHistoryAction(
            "history-2",
            request: .init(action: .report, reason: .harassment, details: "evidence"))
        actions.submitCreateOrbit(title: "Family")
        actions.submitJoinOrbit(code: "ABCDEFGH")
        actions.exportRecovery()
        actions.refreshAirs()
        actions.createAir(title: "Friends")
        actions.consumeAirInvite(code: "secret")
        actions.confirmAirJoin("opaque-air", activate: true)
        actions.declineAirJoin("opaque-air")
        actions.issueAirInvite("opaque-air", role: .member)
        actions.withdrawAirInvite()
        actions.hideAirInvite()
        actions.activateAir("opaque-air")
        actions.deactivateAir("opaque-air")
        actions.leaveAir("opaque-air")
        actions.dissolveAir("opaque-air")
        actions.replaceAirPolicy("opaque-air", policy: .init(
            revision: 4, invite: .ownerPrimary, overlay: .disabled,
            queue: .primaryCompanion, replace: .airAdminPrimary))

        #expect(calls == [
            "create", "join", "self-test", "messages_only", "volume:45", "record",
            "cancel-record", "input:mic-1", "shortcut:control_shift_r", "cue", "five", "review:voice.wav",
            "accept:voice.wav", "delete", "close",
            "send:draft-1:own_barycenter:overlay:rights=true", "delete-outbox:draft-1",
            "refresh-data", "history:history-1:block_actor", "history:history-2:report", "create-api:Family",
            "join-api:ABCDEFGH", "export-recovery", "air-refresh", "air-create:Friends",
            "air-consume:secret", "air-confirm:opaque-air:true", "air-decline:opaque-air",
            "air-invite:opaque-air:member", "air-withdraw", "air-hide",
            "air-activate:opaque-air", "air-deactivate:opaque-air", "air-leave:opaque-air",
            "air-dissolve:opaque-air", "air-policy:opaque-air:4",
        ])
    }

    @MainActor
    @Test("Air projection keeps opaque handles out of labels and one current room explicit")
    func airProjectionAndLifecycleSeams() throws {
        let current = PulsarAirItem(
            id: "air_OPAQUE", title: "Family Air", status: "active", revision: 3,
            membershipID: "aim_OPAQUE", membershipStatus: .joined,
            membershipRevision: 2, role: .owner, memberCount: 2,
            activeMemberCount: 2, onlinePulsarCount: 3, barycenterCapacity: 8,
            onlinePulsarCapacity: 20,
            policy: .init(
                revision: 1, invite: .airAdminPrimary, overlay: .primaryCompanion,
                queue: .primaryCompanion, replace: .airAdminPrimary), isCurrent: true)
        let saved = PulsarAirItem(
            id: "air_OTHER", title: "Studio Air", status: "parked", revision: 1,
            membershipID: "aim_OTHER", membershipStatus: .joined,
            membershipRevision: 1, role: .member, memberCount: 3,
            activeMemberCount: 0, onlinePulsarCount: 0, barycenterCapacity: 8,
            onlinePulsarCapacity: 20, policy: current.policy, isCurrent: false)
        let model = PulsarShellModel()
        model.setAirState(.init(saved: [current, saved], failure: "active_air_changed"))
        #expect(model.snapshot.airs.current?.title == "Family Air")
        #expect(model.snapshot.airs.saved.count == 2)
        #expect(model.snapshot.airs.saved[1].role == .member)
        let secret = PulsarAirInviteSecret(
            airID: current.id, inviteID: "ai_OPAQUE", revision: 1,
            airTitle: current.title, code: "secret-canary", expiresAt: .now)
        #expect(!String(describing: secret).contains("secret-canary"))
        #expect(!String(reflecting: secret).contains("secret-canary"))

        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let view = try String(
            contentsOf: root.appendingPathComponent("Sources/NodeAppUI/PulsarAirView.swift"),
            encoding: .utf8)
        for seam in [
            ".confirmationDialog(", ".privacySensitive()", ".accessibilityLabel(copy.oneTimeCode)",
            "SecureField(copy.inviteCode", "pendingAction = .init(kind: .switchAir",
            "pendingAction = .init(kind: .leave", "pendingAction = .init(kind: .dissolve",
        ] {
            #expect(view.contains(seam))
        }
        #expect(!view.contains("Text(air.id)"))
        #expect(!view.contains("Text(air.membershipID)"))
        #expect(!view.contains("accessibilityLabel(air.id)"))
    }

    @MainActor
    @Test("Moderation reasons and privacy-safe action outcomes cover EN and RU")
    func moderationPresentationContract() {
        let model = PulsarShellModel()
        let foreign = PulsarHistoryItem(
            id: "hi_foreign", title: "Foreign", detail: "played", occurredAt: .now,
            direction: "received", allowedActions: [.report, .blockActor])
        let owned = PulsarHistoryItem(
            id: "hi_owned", title: "Owned", detail: "accepted", occurredAt: .now,
            direction: "sent", allowedActions: [.delete, .replay])
        model.setHistory([foreign, owned])
        #expect(model.snapshot.history[0].allowedActions.contains(.report))
        #expect(!model.snapshot.history[0].allowedActions.contains(.delete))
        #expect(model.snapshot.history[1].allowedActions.contains(.delete))
        #expect(!model.snapshot.history[1].allowedActions.contains(.report))
        model.setPhaseOneActionState(outcome: "report_received", failure: nil)
        #expect(model.snapshot.phaseOneActionOutcome == "report_received")
        #expect(model.snapshot.phaseOneFailure == nil)
        model.setPhaseOneActionState(outcome: nil, failure: "coordinator_unavailable")
        #expect(model.snapshot.phaseOneActionOutcome == nil)
        #expect(model.snapshot.phaseOneFailure == "coordinator_unavailable")
        for locale in PulsarShellLocale.allCases {
            let copy = PulsarShellCopy(locale: locale)
            for reason in PulsarModerationReason.allCases {
                #expect(!copy.moderationReasonLabel(reason).isEmpty)
            }
            for code in [
                "media_deleted", "report_received", "report_already_received",
                "sender_blocked", "sender_already_blocked", "action_not_allowed",
                "history_action_unavailable", "coordinator_unavailable", "unauthorized",
                "forbidden", "insufficient_capability",
            ] {
                let message = copy.historyActionMessage(code)
                #expect(!message.isEmpty)
                #expect(!message.contains("hi_"))
                #expect(!message.contains("rp_"))
                #expect(!message.contains("bl_"))
            }
        }
        #expect(PulsarModerationReason.allCases.map(\.rawValue) == [
            "spam", "harassment", "illegal", "sexual_content", "violence", "other",
        ])
    }

    @MainActor
    @Test("Phase 1 projection keeps canonical labels separate from opaque identifiers")
    func phaseOneProjection() {
        let model = PulsarShellModel()
        let draft = PulsarOutgoingDraft(
            id: "opaque-draft-id",
            title: "Pulsar recording",
            state: .retryableFailure,
            route: .ownBarycenter,
            requestedDelivery: .overlay,
            effectiveDelivery: .afterCurrent,
            downgradeReason: "capability downgrade",
            status: "accepted",
            failureCode: "coordinator unavailable",
            localBytesRetained: false)
        model.setPhaseOneData(
            presenceSummary: "1/2 online · 1 ready",
            outgoingDrafts: [draft],
            failure: "coordinator unavailable")
        model.setIdentityOperation(.recoveryExportRequired(""))
        #expect(model.snapshot.outgoingDrafts == [draft])
        #expect(model.snapshot.phaseOneFailure == "coordinator unavailable")
        #expect(model.snapshot.identityOperation == .recoveryExportRequired(""))
        for locale in PulsarShellLocale.allCases {
            let copy = PulsarShellCopy(locale: locale)
            for route in PulsarRouteTarget.allCases { #expect(!copy.routeLabel(route).isEmpty) }
            for delivery in PulsarDeliveryMode.allCases {
                #expect(!copy.deliveryLabel(delivery).isEmpty)
            }
        }
    }

    @Test("Escape is foreground-scoped and hidden recording has an explicit menu cancel")
    func escapeAndHiddenCancelBoundary() throws {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let window = try String(contentsOf:
            root.appendingPathComponent("node-app/Sources/NodeAppUI/PulsarMainWindow.swift"),
            encoding: .utf8)
        let menu = try String(contentsOf:
            root.appendingPathComponent("node-app/Sources/NodeApp/StatusMenu.swift"),
            encoding: .utf8)
        #expect(window.contains(".onExitCommand"))
        #expect(window.contains("actions.cancelRecording()"))
        #expect(menu.contains("#selector(cancelRecording)"))
        #expect(menu.contains("copy.text(.cancelRecording)"))
        #expect(!window.contains("addGlobalMonitorForEvents"))
        #expect(!menu.contains("addGlobalMonitorForEvents"))
    }

    @Test("History moderation controls retain SwiftUI accessibility and authorization seams")
    func historyModerationSourceContract() throws {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let window = try String(
            contentsOf: root.appendingPathComponent("Sources/NodeAppUI/PulsarMainWindow.swift"),
            encoding: .utf8)
        let composition = try String(
            contentsOf: root.appendingPathComponent("Sources/NodeApp/MacPhaseOneAppComposition.swift"),
            encoding: .utf8)
        for seam in [
            "item.allowedActions.contains(.report)",
            ".accessibilityLabel(copy.text(.reportReason))",
            ".accessibilityLabel(copy.text(.reportDetails))",
            ".confirmationDialog(",
            "boundedReportDetails",
        ] {
            #expect(window.contains(seam))
        }
        for seam in [
            "item.allowedActions.contains(request.action)",
            "client.reportHistoryItem(",
            "model.setPhaseOneActionState(outcome: outcome, failure: nil)",
            "failure: \"action_not_allowed\"",
        ] {
            #expect(composition.contains(seam))
        }
        #expect(!window.contains("Text(item.id)"))
        #expect(!window.contains("accessibilityLabel(item.id)"))
    }
}
