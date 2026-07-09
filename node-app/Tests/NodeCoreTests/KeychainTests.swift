import Foundation
import Security
import Testing
@testable import NodeCore

@Suite struct KeychainTests {
    @Test func saveNeverRequestsTheEntitlementGatedDataProtectionKeychain() throws {
        let keychain = RecordingKeychain(updateStatus: errSecMissingEntitlement)

        try CredentialsStore.save(credentials, using: keychain)

        #expect(keychain.updateQueries.count == 1)
        #expect(keychain.deleteQueries.count == 1)
        #expect(keychain.addItems.count == 1)
        #expect(keychain.updateQueries[0][kSecUseDataProtectionKeychain as String] == nil)
        #expect(keychain.addItems[0][kSecUseDataProtectionKeychain as String] == nil)
        #expect(keychain.addItems[0][kSecAttrAccessible as String] == nil)
    }

    @Test func successfulRepairUpdatesInPlace() throws {
        let keychain = RecordingKeychain(updateStatus: errSecSuccess)

        try CredentialsStore.save(credentials, using: keychain)

        #expect(keychain.updateQueries.count == 1)
        #expect(keychain.deleteQueries.isEmpty)
        #expect(keychain.addItems.isEmpty)
    }

    @Test func loadUsesTheSameFileBasedKeychainStore() throws {
        let keychain = RecordingKeychain(data: try JSONEncoder().encode(credentials))

        let loaded = CredentialsStore.loadFromKeychain(using: keychain)

        #expect(loaded == credentials)
        #expect(keychain.copyQueries.count == 1)
        #expect(keychain.copyQueries[0][kSecUseDataProtectionKeychain as String] == nil)
    }

    private var credentials: NodeCredentials {
        NodeCredentials(
            orbitId: 7,
            slot: "a",
            token: String(repeating: "f", count: 64),
            wsUrl: "wss://barycenter.relux.works/ws"
        )
    }
}

private final class RecordingKeychain: KeychainAccess {
    let updateStatus: OSStatus
    let addStatus: OSStatus
    let data: Data?

    private(set) var updateQueries: [[String: Any]] = []
    private(set) var deleteQueries: [[String: Any]] = []
    private(set) var addItems: [[String: Any]] = []
    private(set) var copyQueries: [[String: Any]] = []

    init(
        updateStatus: OSStatus = errSecItemNotFound,
        addStatus: OSStatus = errSecSuccess,
        data: Data? = nil
    ) {
        self.updateStatus = updateStatus
        self.addStatus = addStatus
        self.data = data
    }

    func update(query: [String: Any], attributes: [String: Any]) -> OSStatus {
        updateQueries.append(query)
        return updateStatus
    }

    func add(attributes: [String: Any]) -> OSStatus {
        addItems.append(attributes)
        return addStatus
    }

    func delete(query: [String: Any]) -> OSStatus {
        deleteQueries.append(query)
        return errSecSuccess
    }

    func copyData(query: [String: Any]) -> Data? {
        copyQueries.append(query)
        return data
    }
}
