import SwiftUI

enum PulsarDesktopMetrics {
    static let minimumWindowWidth: CGFloat = 900
    static let minimumWindowHeight: CGFloat = 640
    static let defaultWindowWidth: CGFloat = 1120
    static let defaultWindowHeight: CGFloat = 760
    static let sidebarMinimumWidth: CGFloat = 210
    static let sidebarIdealWidth: CGFloat = 236
    static let sidebarMaximumWidth: CGFloat = 300
    static let pageMaximumWidth: CGFloat = 960
    static let pagePadding: CGFloat = 24
    static let sectionSpacing: CGFloat = 20
    static let cornerRadius: CGFloat = 12
}

enum PulsarStatusTone: String, CaseIterable {
    case neutral
    case success
    case warning
    case failure
    case progress

    var symbol: String {
        switch self {
        case .neutral: "info.circle"
        case .success: "checkmark.circle.fill"
        case .warning: "exclamationmark.triangle.fill"
        case .failure: "xmark.octagon.fill"
        case .progress: "arrow.triangle.2.circlepath"
        }
    }

    var accent: Color {
        switch self {
        case .neutral: .secondary
        case .success: .green
        case .warning: .orange
        case .failure: .red
        case .progress: .accentColor
        }
    }
}

struct PulsarStatusMessage: View {
    let title: String
    var detail: String? = nil
    let tone: PulsarStatusTone

    @Environment(\.colorSchemeContrast) private var contrast

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            Image(systemName: tone.symbol)
                .foregroundStyle(tone.accent)
                .imageScale(.medium)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(.callout.weight(.semibold))
                if let detail, !detail.isEmpty {
                    Text(detail)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                        .textSelection(.enabled)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .background(.regularMaterial, in: .rect(cornerRadius: PulsarDesktopMetrics.cornerRadius))
        .overlay {
            RoundedRectangle(cornerRadius: PulsarDesktopMetrics.cornerRadius)
                .stroke(tone.accent.opacity(contrast == .increased ? 0.9 : 0.45),
                        lineWidth: contrast == .increased ? 2 : 1)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(title)
        .accessibilityValue(detail ?? "")
    }
}

struct PulsarPage<Content: View>: View {
    let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        ScrollView {
            content
                .frame(maxWidth: PulsarDesktopMetrics.pageMaximumWidth, alignment: .leading)
                .padding(PulsarDesktopMetrics.pagePadding)
                .frame(maxWidth: .infinity, alignment: .center)
        }
        .scrollContentBackground(.hidden)
        .background(.background)
    }
}
