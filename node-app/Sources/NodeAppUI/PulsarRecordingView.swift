import SwiftUI

struct PulsarRecordingActiveBar: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        let presentation = PulsarLocalCapturePresentation(snapshot: model.snapshot)
        let status =
            presentation.isSelfTest
            ? copy.selfTestLabel(model.snapshot.selfTestState)
            : copy.recordingLabel(model.snapshot.recording)
        HStack(spacing: 12) {
            Label(
                status,
                systemImage: "record.circle.fill"
            )
            .font(.callout.bold())
            Text(copy.text(.recordingHelp))
                .font(.callout)
                .foregroundStyle(.secondary)
                .lineLimit(2)
            Spacer(minLength: 12)
            Button(copy.text(.captureStopLocal), action: actions.stopActiveCapture)
                .buttonStyle(.borderedProminent)
                .keyboardShortcut(".", modifiers: .command)
                .disabled(!presentation.canStop)
                .accessibilityHint(copy.text(.recordingHelp))
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(.bar)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(copy.text(.recording))
        .accessibilityValue(status)
    }
}
