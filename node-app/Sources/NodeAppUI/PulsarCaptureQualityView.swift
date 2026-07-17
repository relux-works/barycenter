import SwiftUI

struct PulsarCaptureQualityControls: View {
    @Bindable var model: PulsarShellModel
    let actions: PulsarShellActions

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        let presentation = PulsarCaptureQualityPresentation(snapshot: model.snapshot)
        GroupBox(copy.text(.captureQuality)) {
            VStack(alignment: .leading, spacing: 12) {
                Picker(copy.text(.captureMode), selection: modeBinding) {
                    ForEach(PulsarCaptureQualityMode.allCases) { mode in
                        Text(copy.captureQualityModeLabel(mode)).tag(mode)
                    }
                }
                .pickerStyle(.segmented)
                .disabled(presentation.isActive)
                .accessibilityHint(copy.captureQualityReason(presentation.reason))

                Toggle(copy.text(.allowDegradedCapture), isOn: consentBinding)
                    .disabled(presentation.isActive)
                    .accessibilityHint(copy.text(.degradedCaptureHelp))

                PulsarCaptureQualityStatus(
                    presentation: presentation,
                    copy: copy)

                if presentation.requiresDegradedConsent || presentation.mode == .speaker {
                    Label(
                        copy.captureQualityReason(presentation.reason),
                        systemImage: "exclamationmark.triangle.fill")
                        .font(.callout)
                        .foregroundStyle(.orange)
                        .accessibilityElement(children: .combine)
                }

                Text(copy.text(.degradedCaptureHelp))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.vertical, 4)
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(copy.text(.captureQuality))
    }

    private var modeBinding: Binding<PulsarCaptureQualityMode> {
        Binding(
            get: { model.snapshot.captureQualityMode },
            set: {
                actions.setCaptureQuality(
                    $0, degradedConsent: model.snapshot.captureQualityDegradedConsent)
            })
    }

    private var consentBinding: Binding<Bool> {
        Binding(
            get: { model.snapshot.captureQualityDegradedConsent },
            set: {
                actions.setCaptureQuality(
                    model.snapshot.captureQualityMode, degradedConsent: $0)
            })
    }
}

private struct PulsarCaptureQualityStatus: View {
    let presentation: PulsarCaptureQualityPresentation
    let copy: PulsarShellCopy

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(
                copy.captureQualityLabel(presentation.quality),
                systemImage: qualitySymbol)
                .font(.headline)
                .foregroundStyle(qualityStyle)
                .accessibilityValue(copy.captureQualityReason(presentation.reason))

            LabeledContent(
                copy.text(.captureResolvedRoute),
                value: resolvedModeLabel)
            LabeledContent(
                copy.text(.captureProcessorState),
                value: copy.captureLifecycleLabel(presentation.lifecycle))
            LabeledContent(copy.text(.captureAEC)) {
                Text(copy.captureEffectLabel(presentation.aec))
            }
            LabeledContent(copy.text(.captureNS)) {
                Text(copy.captureEffectLabel(presentation.ns))
            }
            LabeledContent(copy.text(.captureAGC)) {
                Text(copy.captureEffectLabel(presentation.agc))
            }
            LabeledContent(copy.text(.captureInputCeiling)) {
                Text(presentation.inputCeilingDBFS, format: .number.precision(.fractionLength(0)))
                Text("dBFS")
            }
            LabeledContent(copy.text(.receiverOutputCeiling)) {
                Text(presentation.outputCeilingDBFS, format: .number.precision(.fractionLength(0)))
                Text("dBFS")
            }
            Text(copy.text(.captureCeilingHelp))
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .accessibilityElement(children: .contain)
    }

    private var resolvedModeLabel: String {
        copy.captureResolvedModeLabel(presentation.resolvedMode)
    }

    private var qualitySymbol: String {
        switch presentation.quality {
        case "accepted": "checkmark.shield.fill"
        case "degraded": "exclamationmark.triangle.fill"
        case "unsupported": "xmark.shield.fill"
        default: "waveform.badge.mic"
        }
    }

    private var qualityStyle: Color {
        switch presentation.quality {
        case "accepted": .green
        case "degraded": .orange
        case "unsupported": .red
        default: .primary
        }
    }
}

struct PulsarCaptureActiveBar: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        let presentation = PulsarCaptureQualityPresentation(snapshot: model.snapshot)
        HStack(spacing: 12) {
            Label(
                copy.captureQualityLabel(presentation.quality),
                systemImage: "record.circle.fill")
                .font(.callout.bold())
            Text(copy.captureQualityReason(presentation.reason))
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
        .accessibilityLabel(copy.text(.captureQuality))
        .accessibilityValue(
            "\(copy.captureQualityLabel(presentation.quality)). \(copy.captureQualityReason(presentation.reason))")
    }
}
