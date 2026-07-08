// Local Network priming (product 2026-07-08). macOS 15 (Sequoia) gates LAN
// access behind a per-app privacy prompt that fires the first time the app
// touches the local network. For a menu-bar app that first touch is go-librespot
// advertising "Pulsar" over Bonjour — headless, with no window in sight — so the
// prompt can appear out of context or be missed, and a miss silently kills the
// mDNS multicast the phone needs to find the speaker (the most likely cause of
// Timur's "speaker invisible from iPhone", 2026-07-07).
//
// There is no public API to *request* Local Network access. Starting an
// NWBrowser for the same Bonjour type the phone looks for (_spotify-connect._tcp)
// is what makes the system show the prompt — so we fire it from a button the
// user just pressed, in context. The verdict is best-effort: a granted browser
// reaches .ready (a denied one sits in .waiting forever — including while the
// prompt is still on screen, which is why denial is detected by timeout, not by
// error code, and why a late Allow must upgrade the verdict). Note the probe
// runs BEFORE pairing, so our own daemon is not advertising yet; any results
// that do arrive are other Spotify Connect devices and also prove the grant.

import Foundation
import Network

final class LocalNetworkProbe {
    enum Verdict: Equatable {
        case asking   // browser started; system prompt is (or was) on screen
        case granted  // discovery is running — permission was allowed
        case blocked  // still waiting past the answer window — likely denied
    }

    /// Fired on the main queue on each meaningful transition. A confirmed
    /// `.granted` is sticky: a late allow upgrades a prior `.blocked`, never the
    /// other way round.
    var onVerdict: ((Verdict) -> Void)?

    private var browser: NWBrowser?
    private var last: Verdict?

    /// Begins browsing (this is what triggers the prompt on the first-ever call)
    /// and reports verdicts until `stop()`. `promptTimeout` is how long we wait
    /// for the user to answer before treating a lingering `.waiting` as blocked —
    /// generous, because a slow "Allow" still upgrades us to `.granted` after.
    func start(promptTimeout: TimeInterval = 10) {
        deliver(.asking)
        let params = NWParameters()
        params.includePeerToPeer = true
        let browser = NWBrowser(
            for: .bonjour(type: "_spotify-connect._tcp", domain: nil), using: params)
        self.browser = browser
        browser.stateUpdateHandler = { [weak self] state in
            switch state {
            case .ready:
                // A denied browser never leaves .waiting on Sequoia, so reaching
                // .ready means discovery is actually running.
                self?.deliver(.granted)
            case .failed(let err):
                NSLog("local network probe failed: %@", "\(err)")
                self?.deliver(.blocked)
            case .waiting(let err):
                // Denial (and "no network") surface as a persistent .waiting;
                // the timeout below decides.
                NSLog("local network probe waiting: %@", "\(err)")
            default:
                break
            }
        }
        browser.browseResultsChangedHandler = { [weak self] results, _ in
            if !results.isEmpty { self?.deliver(.granted) }
        }
        browser.start(queue: .main)
        DispatchQueue.main.asyncAfter(deadline: .now() + promptTimeout) { [weak self] in
            guard let self else { return }
            if case .waiting = self.browser?.state { self.deliver(.blocked) }
        }
    }

    func stop() {
        browser?.cancel()
        browser = nil
    }

    private func deliver(_ verdict: Verdict) {
        // Never downgrade away from a confirmed grant.
        if last == .granted, verdict != .granted { return }
        guard last != verdict else { return }
        last = verdict
        onVerdict?(verdict)
    }
}
