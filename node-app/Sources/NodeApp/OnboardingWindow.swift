// Legacy optional Telegram pairing. The primary shell owns Create and Join.
// Local Network access is explained and requested only from an explicit button
// for users who choose the optional Spotify integration.

import AppKit
import SwiftUI
import NodeCore

struct OnboardingView: View {
    let coordinatorBase: String
    let showNetworkPriming: Bool
    let onPaired: (NodeCredentials) -> Void

    enum Step { case network, pairing }
    @State private var step: Step

    init(coordinatorBase: String, showNetworkPriming: Bool,
         onPaired: @escaping (NodeCredentials) -> Void) {
        self.coordinatorBase = coordinatorBase
        self.showNetworkPriming = showNetworkPriming
        self.onPaired = onPaired
        _step = State(initialValue: showNetworkPriming ? .network : .pairing)
    }

    var body: some View {
        switch step {
        case .network:
            NetworkPrimingView(onContinue: { step = .pairing })
        case .pairing:
            PairingView(coordinatorBase: coordinatorBase, onPaired: onPaired)
        }
    }
}

// NetworkPrimingView: explain WHY we need the local network before the system
// asks, then trigger the prompt from an explicit button (product decision
// 2026-07-08 — a bare "Pulsar wants to find devices on your network" prompt out
// of context scares people off). Never a dead end: "Позже" and "Продолжить"
// always move forward, since the daemon will ask again later anyway.
private struct NetworkPrimingView: View {
    let onContinue: () -> Void

    @State private var probe = LocalNetworkProbe()
    @State private var verdict: LocalNetworkProbe.Verdict?

    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "wifi")
                .font(.system(size: 34, weight: .light))
                .foregroundStyle(.tint)
            Text("Доступ к локальной сети").font(.title2).bold()
            Text("Пульсар использует локальную сеть только для необязательной интеграции Spotify. Локальная проверка и основной звук Пульсара работают без неё.")
                .font(.callout)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .frame(width: 320)

            switch verdict {
            case .granted:
                Label("Доступ разрешён", systemImage: "checkmark.circle.fill")
                    .foregroundStyle(.green)
                Button("Дальше", action: onContinue)
                    .buttonStyle(.borderedProminent)
            case .blocked:
                Label("Похоже, доступ не дан", systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(.orange)
                Text("Открой Настройки → Конфиденциальность и безопасность → Локальная сеть и включи Pulsar.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .frame(width: 300)
                HStack(spacing: 10) {
                    Button("Открыть Настройки", action: openLocalNetworkSettings)
                    Button("Продолжить", action: onContinue)
                        .buttonStyle(.borderedProminent)
                }
            case .asking:
                ProgressView().controlSize(.small)
                Text("Подтверди доступ в системном окне…")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            case nil:
                Button("Разрешить доступ к локальной сети", action: request)
                    .buttonStyle(.borderedProminent)
                Button("Позже", action: onContinue)
                    .buttonStyle(.link)
            }
        }
        .padding(28)
        .frame(width: 380)
        .onDisappear { probe.stop() }
    }

    private func request() {
        probe.onVerdict = { verdict = $0 }
        probe.start()
    }

    private func openLocalNetworkSettings() {
        if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_LocalNetwork") {
            NSWorkspace.shared.open(url)
        }
    }
}

private struct PairingView: View {
    let coordinatorBase: String
    let onPaired: (NodeCredentials) -> Void

    @State private var code = ""
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        VStack(spacing: 14) {
            Image(systemName: "dot.radiowaves.left.and.right")
                .font(.system(size: 34, weight: .light))
                .foregroundStyle(.tint)
            Text("Пульсар").font(.title2).bold()
            VStack(alignment: .leading, spacing: 6) {
                Text("В главном окне Пульсара можно создать Барицентр или подключить это устройство. Общий эфир создаётся позже.")
                Text("Telegram — необязательный дополнительный пульт. Если ты решил его использовать, получи командой /pair код для этого мака и введи его ниже.")
                Text("Локальная проверка, маршрутизация и история доступны без Telegram.")
            }
            .font(.callout)
            .foregroundStyle(.secondary)
            .frame(width: 320, alignment: .leading)

            HStack(spacing: 10) {
                Link("Открыть @barycenter_bot", destination: URL(string: "https://t.me/barycenter_bot")!)
                Link("barycenter.live", destination: URL(string: "https://barycenter.live")!)
            }
            .font(.callout)

            TextField("КОД ИЗ БОТА", text: $code)
                .textFieldStyle(.roundedBorder)
                .font(.system(.title3, design: .monospaced))
                .multilineTextAlignment(.center)
                .frame(width: 220)
                .onChange(of: code) { _, v in
                    code = String(v.uppercased().filter { $0.isLetter || $0.isNumber }.prefix(8))
                }
                .onSubmit(pair)
                .disabled(busy)

            if let error {
                Text(error)
                    .font(.callout)
                    .foregroundStyle(.red)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 300)
            }

            Button(action: pair) {
                if busy {
                    ProgressView().controlSize(.small).frame(width: 120)
                } else {
                    Text("Подключить").frame(width: 120)
                }
            }
            .buttonStyle(.borderedProminent)
            .disabled(code.count != 8 || busy)

            VStack(spacing: 3) {
                Text("Необязательная интеграция Spotify требует общей Wi-Fi сети; при необходимости проверь файрвол macOS и VPN.")
                Link("Полная инструкция → barycenter.live/guide", destination: URL(string: "https://barycenter.live/guide/")!)
            }
            .font(.caption)
            .foregroundStyle(.secondary)
            .multilineTextAlignment(.center)
            .frame(width: 320)
        }
        .padding(28)
        .frame(width: 380)
    }

    private func pair() {
        guard code.count == 8, !busy else { return }
        busy = true
        error = nil
        let entered = code
        DispatchQueue.global().async {
            let result = pairNode(code: entered, coordinatorBase: coordinatorBase)
            DispatchQueue.main.async {
                busy = false
                switch result {
                case .success(let creds):
                    do {
                        try CredentialsStore.save(creds)
                    } catch {
                        self.error = "не смог сохранить в связку ключей: \(error.localizedDescription)"
                        return
                    }
                    onPaired(creds)
                case .failure(let err):
                    self.error = err.description
                }
            }
        }
    }
}

final class OnboardingWindowController {
    private var window: NSWindow?

    // promptForNetwork: show the Local Network priming step first. True on the
    // real first launch; false on re-pair, where LAN access was settled long ago.
    func show(coordinatorBase: String, promptForNetwork: Bool,
              onPaired: @escaping (NodeCredentials) -> Void) {
        // L6: a second show() (e.g. "Подключить заново…" clicked twice) used to
        // orphan the first window — pairing in the orphan then started a second
        // core on top of the live one (two librespots on one token). One window
        // at a time: close the previous one first.
        if let old = window {
            old.close()
            window = nil
        }
        let view = OnboardingView(coordinatorBase: coordinatorBase,
                                  showNetworkPriming: promptForNetwork) { [weak self] creds in
            self?.window?.close()
            self?.window = nil
            onPaired(creds)
        }
        let hosting = NSHostingController(rootView: view)
        let w = NSWindow(contentViewController: hosting)
        w.title = "Pulsar"
        w.styleMask = [.titled, .closable]
        w.isReleasedWhenClosed = false
        w.center()
        window = w
        NSApp.activate(ignoringOtherApps: true)
        w.makeKeyAndOrderFront(nil)
    }
}
