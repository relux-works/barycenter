import Foundation
import Security

/// Keychain-backed storage for pairing credentials (goal v2 R1: tokens live
/// in the login keychain, not in files). Migration: an existing
/// node-credentials.json (pre-R1) is imported once and deleted.
public enum CredentialsStore {
    static let service = "works.relux.pulsar"
    static let account = "node-credentials"

    public static func save(_ creds: NodeCredentials) throws {
        let data = try JSONEncoder().encode(creds)
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        let attrs: [String: Any] = [kSecValueData as String: data]
        var status = SecItemUpdate(query as CFDictionary, attrs as CFDictionary)
        if status == errSecItemNotFound {
            var add = query
            add[kSecValueData as String] = data
            status = SecItemAdd(add as CFDictionary, nil)
        }
        guard status == errSecSuccess else {
            throw NSError(domain: NSOSStatusErrorDomain, code: Int(status),
                          userInfo: [NSLocalizedDescriptionKey: "keychain save failed (\(status))"])
        }
    }

    public static func loadFromKeychain() -> NodeCredentials? {
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

    /// load returns credentials from the keychain, importing (and removing)
    /// a legacy JSON file beside the config on first sight.
    public static func load(besideConfig configPath: String) -> NodeCredentials? {
        if let creds = loadFromKeychain() { return creds }
        guard let legacy = NodeCredentials.load(besideConfig: configPath) else { return nil }
        if (try? save(legacy)) != nil {
            try? FileManager.default.removeItem(
                at: NodeCredentials.fileURL(besideConfig: configPath))
        }
        return legacy
    }

    public static func clear() {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        SecItemDelete(query as CFDictionary)
    }
}
