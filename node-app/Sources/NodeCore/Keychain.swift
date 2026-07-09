import Foundation
import Security

/// Keychain-backed storage for pairing credentials.
///
/// F2 (goal v2.1): items live in the **Data Protection keychain**
/// (kSecUseDataProtectionKeychain), keyed by the app's code signature, not by
/// a per-file ACL. A Sparkle update replaces the app bundle on disk; the old
/// file-ACL login-keychain item then silently denied the new binary and the
/// node fell back to onboarding (beta finding 2026-07-07). DP items survive
/// updates as long as the signing identity is stable (Developer ID is).
///
/// No explicit access group is set — the app's default group (derived from
/// the signature) needs no entitlement, works on dev self-signed builds too,
/// and is stable across updates of the same identity. kSecAttrAccessible is
/// AfterFirstUnlock so the daemon can read on a headless relaunch.
public enum CredentialsStore {
    static let service = "works.relux.pulsar"
    static let account = "node-credentials"

    private static func baseQuery(dataProtection: Bool) -> [String: Any] {
        var q: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        if dataProtection {
            q[kSecUseDataProtectionKeychain as String] = true
        }
        return q
    }

    public static func save(_ creds: NodeCredentials) throws {
        let data = try JSONEncoder().encode(creds)
        var status = saveIn(dataProtection: true, data: data)
        if status == errSecMissingEntitlement {
            // -34018: the DP keychain requires the com.apple.application-
            // identifier entitlement, which the Developer ID bundle does NOT
            // carry — every DP call has failed on real installs since F2 (the
            // app survived on the legacy login-keychain fallback in load()).
            // The file-based keychain is the working store on such builds;
            // with a stable Developer ID identity its ACL survives updates,
            // which was F2's actual goal. (Live finding -34018, 2026-07-10.)
            status = saveIn(dataProtection: false, data: data)
        }
        guard status == errSecSuccess else {
            let msg = SecCopyErrorMessageString(status, nil) as String? ?? "OSStatus \(status)"
            throw NSError(domain: NSOSStatusErrorDomain, code: Int(status),
                          userInfo: [NSLocalizedDescriptionKey: "keychain save failed: \(msg) (\(status))"])
        }
    }

    /// One update-or-recreate dance against either keychain. Any update
    /// failure (relic with a different accessible attribute, owner mismatch
    /// after a signing change, not-found) falls back to delete+add.
    private static func saveIn(dataProtection: Bool, data: Data) -> OSStatus {
        let query = baseQuery(dataProtection: dataProtection)
        let attrs: [String: Any] = [kSecValueData as String: data]
        var status = SecItemUpdate(query as CFDictionary, attrs as CFDictionary)
        if status == errSecSuccess || status == errSecMissingEntitlement {
            return status
        }
        SecItemDelete(query as CFDictionary)
        var add = query
        add[kSecValueData as String] = data
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        return SecItemAdd(add as CFDictionary, nil)
    }

    /// Reads the Data Protection item (nil if absent or DP is unavailable).
    public static func loadFromKeychain() -> NodeCredentials? {
        var query = baseQuery(dataProtection: true)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &out) == errSecSuccess,
              let data = out as? Data else { return nil }
        return try? JSONDecoder().decode(NodeCredentials.self, from: data)
    }

    /// Reads the pre-F2 login-keychain item (no DP flag) for one-time migration.
    private static func loadLegacyKeychain() -> NodeCredentials? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var out: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &out) == errSecSuccess,
              let data = out as? Data else { return nil }
        return try? JSONDecoder().decode(NodeCredentials.self, from: data)
    }

    private static func deleteLegacyKeychain() {
        SecItemDelete([
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ] as CFDictionary)
    }

    /// load returns credentials, migrating forward on first sight from either
    /// a legacy node-credentials.json file or the pre-F2 login-keychain item.
    public static func load(besideConfig configPath: String) -> NodeCredentials? {
        if let creds = loadFromKeychain() { return creds }

        // Migrate the pre-F2 login-keychain item into the DP keychain. Delete
        // the source ONLY when a DP copy verifiably exists: on builds without
        // the DP entitlement save() falls back to the file keychain — i.e. it
        // UPDATES this very item — and an unconditional delete would destroy
        // the only copy of the credentials right after "migrating" them.
        if let legacy = loadLegacyKeychain() {
            if (try? save(legacy)) != nil, loadFromKeychain() != nil {
                deleteLegacyKeychain()
            }
            return legacy
        }

        // Migrate a legacy JSON file (pre-R1).
        guard let file = NodeCredentials.load(besideConfig: configPath) else { return nil }
        if (try? save(file)) != nil {
            try? FileManager.default.removeItem(
                at: NodeCredentials.fileURL(besideConfig: configPath))
        }
        return file
    }

    public static func clear() {
        SecItemDelete(baseQuery(dataProtection: true) as CFDictionary)
        SecItemDelete(baseQuery(dataProtection: false) as CFDictionary)
    }
}
