// R2 (goal v2 D7): first-launch pairing window — the only thing a new user
// ever sees. Code from the bot -> POST /pair -> keychain -> core starts.

import AppKit
import SwiftUI
import NodeCore

struct OnboardingView: View {
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
                Text("Пульсар — общий музыкальный эфир для ваших домов. Чтобы подключить этот мак, нужен код из телеграм-бота:")
                Text("1. Открой бота и напиши /create — если создаёшь свой барицентр.")
                Text("2. Или открой инвайт-ссылку от партнёра и напиши /pair.")
                Text("3. Введи код сюда — и музыка дома.")
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

    func show(coordinatorBase: String, onPaired: @escaping (NodeCredentials) -> Void) {
        let view = OnboardingView(coordinatorBase: coordinatorBase) { [weak self] creds in
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
