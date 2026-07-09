import Foundation
import Security

/// Small Security.framework boundary so credential policy is testable without
/// reading or mutating the developer machine's real Keychain.
protocol KeychainAccess {
    func update(query: [String: Any], attributes: [String: Any]) -> OSStatus
    func add(attributes: [String: Any]) -> OSStatus
    func delete(query: [String: Any]) -> OSStatus
    func copyData(query: [String: Any]) -> Data?
}

private struct SystemKeychainAccess: KeychainAccess {
    func update(query: [String: Any], attributes: [String: Any]) -> OSStatus {
        SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
    }

    func add(attributes: [String: Any]) -> OSStatus {
        SecItemAdd(attributes as CFDictionary, nil)
    }

    func delete(query: [String: Any]) -> OSStatus {
        SecItemDelete(query as CFDictionary)
    }

    func copyData(query: [String: Any]) -> Data? {
        var lookup = query
        lookup[kSecReturnData as String] = true
        lookup[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        guard SecItemCopyMatching(lookup as CFDictionary, &result) == errSecSuccess else {
            return nil
        }
        return result as? Data
    }
}

/// Keychain-backed storage for pairing credentials.
///
/// Pulsar's Developer ID bundle has no provisioning profile authorizing the
/// restricted Data Protection Keychain entitlements. Setting
/// `kSecUseDataProtectionKeychain` therefore fails with
/// `errSecMissingEntitlement` (-34018). Until the release channel provisions
/// those entitlements, credentials live in the ordinary login Keychain. Its
/// ACL follows Pulsar's stable Developer ID designated requirement across
/// updates and requires no entitlement-gated query attributes.
public enum CredentialsStore {
    static let service = "works.relux.pulsar"
    static let account = "node-credentials"

    private static func query() -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
    }

    public static func save(_ credentials: NodeCredentials) throws {
        try save(credentials, using: SystemKeychainAccess())
    }

    static func save(_ credentials: NodeCredentials, using keychain: KeychainAccess) throws {
        let data = try JSONEncoder().encode(credentials)
        let query = query()
        var status = keychain.update(
            query: query,
            attributes: [kSecValueData as String: data]
        )
        if status != errSecSuccess {
            // Re-pair heals a stale or inaccessible item by recreating it.
            // A missing delete is harmless; the add status is authoritative.
            _ = keychain.delete(query: query)
            var item = query
            item[kSecValueData as String] = data
            status = keychain.add(attributes: item)
        }
        guard status == errSecSuccess else {
            let message = SecCopyErrorMessageString(status, nil) as String?
                ?? "OSStatus \(status)"
            throw NSError(
                domain: NSOSStatusErrorDomain,
                code: Int(status),
                userInfo: [
                    NSLocalizedDescriptionKey:
                        "keychain save failed: \(message) (\(status))",
                ]
            )
        }
    }

    public static func loadFromKeychain() -> NodeCredentials? {
        loadFromKeychain(using: SystemKeychainAccess())
    }

    static func loadFromKeychain(using keychain: KeychainAccess) -> NodeCredentials? {
        guard let data = keychain.copyData(query: query()) else { return nil }
        return try? JSONDecoder().decode(NodeCredentials.self, from: data)
    }

    /// Loads credentials from Keychain, migrating the pre-Keychain JSON file
    /// on first sight. The source is removed only after a successful save.
    public static func load(besideConfig configPath: String) -> NodeCredentials? {
        if let credentials = loadFromKeychain() { return credentials }

        guard let file = NodeCredentials.load(besideConfig: configPath) else { return nil }
        if (try? save(file)) != nil {
            try? FileManager.default.removeItem(
                at: NodeCredentials.fileURL(besideConfig: configPath)
            )
        }
        return file
    }

    public static func clear() {
        _ = SystemKeychainAccess().delete(query: query())
    }
}
