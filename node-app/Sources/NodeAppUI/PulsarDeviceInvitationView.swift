import SwiftUI

struct PulsarDeviceInvitationView: View {
    @Bindable var model: PulsarDeviceInvitationModel
    let locale: PulsarShellLocale
    let actions: PulsarShellActions

    var body: some View {
        let copy = PulsarDeviceInvitationCopy(locale: locale)
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                Label(copy.title, systemImage: "desktopcomputer.and.arrow.down")
                    .font(.title2.bold())
                Text(copy.intro)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)

                authorization(copy)

                if model.snapshot.isGenerating {
                    ProgressView(copy.generating)
                        .accessibilityLabel(copy.generating)
                }

                if let invitation = model.snapshot.invitation,
                    let code = model.visibleCode
                {
                    invitationCard(invitation, code: code, copy: copy)
                }

                feedback(copy)
            }
            .frame(maxWidth: 640, alignment: .leading)
            .padding(32)
            .frame(maxWidth: .infinity, alignment: .center)
        }
        .navigationTitle(copy.title)
        .onDisappear {
            if model.snapshot.invitation != nil || model.snapshot.isGenerating {
                actions.hideDeviceInvitation()
            }
        }
    }

    @ViewBuilder
    private func authorization(_ copy: PulsarDeviceInvitationCopy) -> some View {
        switch model.snapshot.authorization {
        case .notChecked:
            Button(copy.refreshAuthorization) {
                actions.refreshDeviceInvitationAuthorization()
            }
        case .checking:
            ProgressView(copy.checking)
                .accessibilityLabel(copy.checking)
        case .authorizedPrimary:
            VStack(alignment: .leading, spacing: 10) {
                Label(copy.authorized, systemImage: "checkmark.shield.fill")
                    .accessibilityElement(children: .combine)
                if model.snapshot.invitation == nil && !model.snapshot.isGenerating {
                    Button(copy.generate) {
                        actions.generateDeviceInvitation()
                    }
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut("i", modifiers: [.command, .shift])
                }
            }
        case .unavailable(let failure):
            VStack(alignment: .leading, spacing: 10) {
                Label(copy.failure(failure), systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(.secondary)
                    .accessibilityElement(children: .combine)
                Button(copy.refreshAuthorization) {
                    actions.refreshDeviceInvitationAuthorization()
                }
            }
        }
    }

    private func invitationCard(
        _ invitation: PulsarDeviceInvitationMetadata,
        code: String,
        copy: PulsarDeviceInvitationCopy
    ) -> some View {
        GroupBox(copy.invitationReady) {
            VStack(alignment: .leading, spacing: 12) {
                Text(code)
                    .font(.system(.title2, design: .monospaced, weight: .semibold))
                    .privacySensitive()
                    .accessibilityHidden(true)
                Text(copy.codeVisible)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                LabeledContent(copy.intendedRole, value: copy.companion)
                HStack {
                    Text(copy.expires)
                    Spacer()
                    Text(
                        invitation.expiresAt,
                        format: .dateTime.year().month().day().hour().minute())
                }
                Text(copy.oneTimeWarning)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                HStack {
                    Button(copy.copyCode) {
                        actions.copyDeviceInvitation()
                    }
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut("c", modifiers: [.command, .shift])
                    Button(copy.hideCode) {
                        actions.hideDeviceInvitation()
                    }
                    .keyboardShortcut("h", modifiers: [.command, .shift])
                }
            }
            .padding(.vertical, 6)
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(copy.invitationReady)
        .accessibilityHint(copy.codeVisible)
    }

    @ViewBuilder
    private func feedback(_ copy: PulsarDeviceInvitationCopy) -> some View {
        switch model.snapshot.feedback {
        case .copied(let autoClearAt):
            VStack(alignment: .leading, spacing: 4) {
                Label(copy.copied, systemImage: "clipboard.fill")
                Text(autoClearAt, format: .dateTime.hour().minute().second())
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            .accessibilityElement(children: .combine)
        case .hidden:
            Label(copy.hidden, systemImage: "eye.slash.fill")
                .accessibilityElement(children: .combine)
        case .expired:
            Label(copy.expired, systemImage: "clock.badge.exclamationmark.fill")
                .accessibilityElement(children: .combine)
        case .failure(let failure):
            Label(copy.failure(failure), systemImage: "exclamationmark.triangle.fill")
                .accessibilityElement(children: .combine)
        case nil:
            EmptyView()
        }
    }
}
