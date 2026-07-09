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

    private static func baseQuery() -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecUseDataProtectionKeychain as String: true,
        ]
    }

    public static func save(_ creds: NodeCredentials) throws {
        let data = try JSONEncoder().encode(creds)
        let query = baseQuery()
        let attrs: [String: Any] = [kSecValueData as String: data]
        var status = SecItemUpdate(query as CFDictionary, attrs as CFDictionary)
        if status != errSecSuccess {
            // Re-pair must heal whatever state the OLD item is in, not fail
            // the whole pairing over an unupdatable relic: an item written by
            // an earlier build with a different accessible attribute, an
            // owner/partition mismatch after a signing change, or plain
            // not-found. Drop it (a miss is harmless) and write fresh.
            SecItemDelete(query as CFDictionary)
            var add = query
            add[kSecValueData as String] = data
            add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
            status = SecItemAdd(add as CFDictionary, nil)
        }
        guard status == errSecSuccess else {
            // Name the cause: a bare numeric status sent us hunting blind
            // (re-pair failure report, 2026-07-10).
            let msg = SecCopyErrorMessageString(status, nil) as String? ?? "OSStatus \(status)"
            throw NSError(domain: NSOSStatusErrorDomain, code: Int(status),
                          userInfo: [NSLocalizedDescriptionKey: "keychain save failed: \(msg) (\(status))"])
        }
    }

    /// Reads the Data Protection item (nil if absent).
    public static func loadFromKeychain() -> NodeCredentials? {
        var query = baseQuery()
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

        // Migrate the pre-F2 login-keychain item into the DP keychain.
        if let legacy = loadLegacyKeychain() {
            if (try? save(legacy)) != nil {
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
        SecItemDelete(baseQuery() as CFDictionary)
    }
}
