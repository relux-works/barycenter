import CryptoKit
import Foundation

public enum MacProtectedMediaPlaybackFailure: String, Error, Equatable, LocalizedError,
    Sendable
{
    case blocked
    case corruptCiphertext = "corrupt_ciphertext"
    case downgradeForbidden = "downgrade_forbidden"
    case expired
    case forkedEpoch = "forked_epoch"
    case invalidAuthentication = "invalid_authentication"
    case invalidManifest = "invalid_manifest"
    case invalidRequest = "invalid_request"
    case missingGrant = "missing_grant"
    case productionDisabled = "production_disabled"
    case revoked
    case staleEpoch = "stale_epoch"
    case targetChanged = "target_changed"
    case transport

    public var errorDescription: String? {
        switch self {
        case .productionDisabled:
            "Protected media playback is not available in this build."
        case .missingGrant:
            "This device does not have access to the protected media history."
        case .blocked, .revoked:
            "Protected media access is no longer available."
        default:
            "Protected media could not be played safely."
        }
    }
}

public struct MacProtectedMediaPlaybackRequest: Sendable {
    public let objectID: String
    public let recipientDeviceID: String
    public let groupID: String
    public let expectedGroupRevision: UInt64
    public let expectedEpoch: UInt64
    public let expectedGeneration: UInt64
    public let expectedTargetSnapshotDigest: String
    public let historyGrantID: String?
    public let policyAllowed: Bool
    public let dndAllowed: Bool
    public let senderBlocked: Bool

    public init(
        objectID: String, recipientDeviceID: String, groupID: String,
        expectedGroupRevision: UInt64, expectedEpoch: UInt64,
        expectedGeneration: UInt64, expectedTargetSnapshotDigest: String,
        historyGrantID: String? = nil, policyAllowed: Bool, dndAllowed: Bool,
        senderBlocked: Bool
    ) {
        self.objectID = objectID
        self.recipientDeviceID = recipientDeviceID
        self.groupID = groupID
        self.expectedGroupRevision = expectedGroupRevision
        self.expectedEpoch = expectedEpoch
        self.expectedGeneration = expectedGeneration
        self.expectedTargetSnapshotDigest = expectedTargetSnapshotDigest
        self.historyGrantID = historyGrantID
        self.policyAllowed = policyAllowed
        self.dndAllowed = dndAllowed
        self.senderBlocked = senderBlocked
    }
}

public struct MacProtectedMediaPlaybackRoute: Equatable, Sendable {
    public let contract: String
    public let capability: String
    public let suite: String
    public let container: String
    public let objectID: String
    public let sourceObjectID: String
    public let objectKind: MacProtectedMediaKind
    public let authorDeviceID: String
    public let recipientDeviceID: String
    public let groupID: String
    public let epoch: UInt64
    public let generation: UInt64
    public let targetSnapshotDigest: String
    public let expiresAtMS: Int64
    public let manifestDigest: String
    public let encryptedManifest: Data
    public let opaqueKeyEnvelope: Data
    public let authenticatedManifest: Data
    public let signature: Data
    public let streamManifest: MacStreamManifest

    public init(
        contract: String, capability: String, suite: String, container: String,
        objectID: String, sourceObjectID: String, objectKind: MacProtectedMediaKind,
        authorDeviceID: String, recipientDeviceID: String, groupID: String,
        epoch: UInt64, generation: UInt64, targetSnapshotDigest: String,
        expiresAtMS: Int64, manifestDigest: String, encryptedManifest: Data,
        opaqueKeyEnvelope: Data, authenticatedManifest: Data, signature: Data,
        streamManifest: MacStreamManifest
    ) {
        self.contract = contract
        self.capability = capability
        self.suite = suite
        self.container = container
        self.objectID = objectID
        self.sourceObjectID = sourceObjectID
        self.objectKind = objectKind
        self.authorDeviceID = authorDeviceID
        self.recipientDeviceID = recipientDeviceID
        self.groupID = groupID
        self.epoch = epoch
        self.generation = generation
        self.targetSnapshotDigest = targetSnapshotDigest
        self.expiresAtMS = expiresAtMS
        self.manifestDigest = manifestDigest
        self.encryptedManifest = encryptedManifest
        self.opaqueKeyEnvelope = opaqueKeyEnvelope
        self.authenticatedManifest = authenticatedManifest
        self.signature = signature
        self.streamManifest = streamManifest
    }
}

public struct MacProtectedMediaRangeRequest: Equatable, Sendable {
    public let objectID: String
    public let recipientDeviceID: String
    public let groupID: String
    public let epoch: UInt64
    public let generation: UInt64
    public let targetSnapshotDigest: String
    public let manifestDigest: String
    public let etag: String
    public let start: Int64
    public let end: Int64
}

public protocol MacProtectedMediaPlaybackTransport: Sendable {
    func fetchManifest(
        objectID: String, recipientDeviceID: String, requestedAtMS: Int64
    ) async throws -> MacProtectedMediaPlaybackRoute
    func fetchRange(_ request: MacProtectedMediaRangeRequest) async throws
        -> (ciphertext: Data, etag: String)
}

public final class MacProtectedMediaOpenLease: @unchecked Sendable,
    CustomStringConvertible, CustomDebugStringConvertible
{
    private let lock = NSLock()
    private var opaqueState: Data

    public init(opaqueState: Data) {
        self.opaqueState = opaqueState
    }

    public var description: String { "MacProtectedMediaOpenLease(<redacted>)" }
    public var debugDescription: String { description }

    public func withOpaqueState<Result>(
        _ body: (UnsafeRawBufferPointer) throws -> Result
    ) rethrows -> Result {
        lock.lock()
        defer { lock.unlock() }
        return try opaqueState.withUnsafeBytes(body)
    }

    public func destroy() {
        lock.lock()
        _ = opaqueState.withUnsafeMutableBytes {
            $0.initializeMemory(as: UInt8.self, repeating: 0)
        }
        opaqueState.removeAll(keepingCapacity: false)
        lock.unlock()
    }

    deinit { destroy() }
}

/// The selected implementation must authenticate the manifest/envelope in
/// `open` and authenticate every AEAD record before returning bytes from
/// `authenticateAndDecrypt`. NodeCore never implements or selects a suite.
public protocol MacProtectedMediaOpening: Sendable {
    var productionApproved: Bool { get }
    func open(
        route: MacProtectedMediaPlaybackRoute, identity: MacE2EEDeviceIdentityLease,
        groupState: MacE2EEGroupStateLease, historyGrant: MacE2EESecretLease?
    ) async throws -> MacProtectedMediaOpenLease
    func authenticateAndDecrypt(
        ciphertext: Data, chunk: MacStreamChunk, route: MacProtectedMediaPlaybackRoute,
        lease: MacProtectedMediaOpenLease
    ) async throws -> Data
}

private final class MacProtectedMediaAuthorization: @unchecked Sendable {
    private let lock = NSLock()
    private var available = true

    func requireAvailable() throws {
        lock.lock()
        defer { lock.unlock() }
        guard available else { throw MacProtectedMediaPlaybackFailure.revoked }
    }

    func revoke() {
        lock.lock()
        available = false
        lock.unlock()
    }
}

private final class MacProtectedMediaRangeAdapter: MacStreamRangeFetching,
    @unchecked Sendable
{
    private let route: MacProtectedMediaPlaybackRoute
    private let transport: any MacProtectedMediaPlaybackTransport
    private let authorization: MacProtectedMediaAuthorization

    init(
        route: MacProtectedMediaPlaybackRoute,
        transport: any MacProtectedMediaPlaybackTransport,
        authorization: MacProtectedMediaAuthorization
    ) {
        self.route = route
        self.transport = transport
        self.authorization = authorization
    }

    func fetchRange(
        path: String, etag: String, start: Int64, end: Int64
    ) async throws -> (data: Data, etag: String) {
        try authorization.requireAvailable()
        guard path == route.streamManifest.variantUrl,
            etag == route.streamManifest.etag,
            route.streamManifest.chunks.contains(where: { $0.start == start && $0.end == end })
        else {
            throw MacStreamFailure.frozen(stage: "fetch", code: "invalid_range")
        }
        do {
            let response = try await transport.fetchRange(
                MacProtectedMediaRangeRequest(
                    objectID: route.objectID, recipientDeviceID: route.recipientDeviceID,
                    groupID: route.groupID, epoch: route.epoch,
                    generation: route.generation,
                    targetSnapshotDigest: route.targetSnapshotDigest,
                    manifestDigest: route.manifestDigest, etag: etag,
                    start: start, end: end))
            try authorization.requireAvailable()
            return (response.ciphertext, response.etag)
        } catch let failure as MacProtectedMediaPlaybackFailure {
            if failure == .revoked || failure == .expired || failure == .blocked {
                authorization.revoke()
                throw MacStreamFailure.frozen(stage: "fetch", code: "revoked")
            }
            throw MacStreamFailure.frozen(stage: "fetch", code: failure.rawValue)
        } catch let failure as MacStreamFailure {
            throw failure
        } catch is CancellationError {
            throw CancellationError()
        } catch {
            throw MacStreamFailure.frozen(stage: "fetch", code: "network_failed")
        }
    }
}

private final class MacProtectedMediaChunkReader: MacStreamChunkReading,
    @unchecked Sendable
{
    public let manifest: MacStreamManifest
    private let route: MacProtectedMediaPlaybackRoute
    private let cache: MacStreamChunkCache
    private let opener: any MacProtectedMediaOpening
    private let lease: MacProtectedMediaOpenLease
    private let authorization: MacProtectedMediaAuthorization
    private let revalidate: @Sendable () throws -> Void

    init(
        route: MacProtectedMediaPlaybackRoute, cache: MacStreamChunkCache,
        opener: any MacProtectedMediaOpening, lease: MacProtectedMediaOpenLease,
        authorization: MacProtectedMediaAuthorization,
        revalidate: @escaping @Sendable () throws -> Void
    ) {
        self.route = route
        self.manifest = route.streamManifest
        self.cache = cache
        self.opener = opener
        self.lease = lease
        self.authorization = authorization
        self.revalidate = revalidate
    }

    func chunkIndex(forTimeMs positionMs: Int64) -> Int {
        manifest.chunkIndex(forTimeMs: positionMs)
    }

    func readChunk(index: Int) async throws -> Data {
        do {
            try authorization.requireAvailable()
            try revalidate()
            guard manifest.chunks.indices.contains(index) else {
                throw MacProtectedMediaPlaybackFailure.invalidManifest
            }
            let ciphertext = try await cache.chunk(manifest, index: index)
            try authorization.requireAvailable()
            try revalidate()
            let plaintext = try await opener.authenticateAndDecrypt(
                ciphertext: ciphertext, chunk: manifest.chunks[index],
                route: route, lease: lease)
            try authorization.requireAvailable()
            try revalidate()
            guard !plaintext.isEmpty,
                plaintext.count <= Int(MacStreamCacheLimits.maximumChunkBytes)
            else { throw MacProtectedMediaPlaybackFailure.invalidAuthentication }
            var pins = [index]
            if index + 1 < manifest.chunks.count { pins.append(index + 1) }
            try await cache.setPinned(manifest, indexes: pins)
            return plaintext
        } catch let failure as MacProtectedMediaPlaybackFailure {
            if failure == .invalidAuthentication || failure == .corruptCiphertext
                || failure == .targetChanged || failure == .expired
            {
                try? await cache.invalidate(manifest)
            } else if failure == .revoked || failure == .blocked {
                authorization.revoke()
                try? await cache.tombstone(manifest)
            }
            throw MacStreamFailure.frozen(stage: "protected_media", code: failure.rawValue)
        }
    }

    func close() {
        authorization.revoke()
        lease.destroy()
    }
}

public final class MacProtectedMediaPreparedPlayback: @unchecked Sendable {
    public let route: MacProtectedMediaPlaybackRoute
    public let cache: MacStreamChunkCache
    public let chunks: MacStreamChunkReading
    private let reader: MacProtectedMediaChunkReader
    private let authorization: MacProtectedMediaAuthorization

    fileprivate init(
        route: MacProtectedMediaPlaybackRoute, cache: MacStreamChunkCache,
        reader: MacProtectedMediaChunkReader,
        authorization: MacProtectedMediaAuthorization
    ) {
        self.route = route
        self.cache = cache
        self.reader = reader
        self.chunks = reader
        self.authorization = authorization
    }

    public func makeCandidatePlayer(
        decoder: MacStreamCandidateDecoder, clock: MacStreamDeadlineClock,
        send: @escaping @Sendable (Message) -> Void
    ) -> MacStreamCandidatePlayer {
        let player = MacStreamCandidatePlayer(
            cache: cache, decoder: decoder, clock: clock, protectedChunks: chunks,
            send: send)
        player.retainProtectedLifetime(self)
        return player
    }

    public func revoke() async throws {
        authorization.revoke()
        reader.close()
        try await cache.tombstone(route.streamManifest)
    }

    deinit { reader.close() }
}

public actor MacProtectedMediaPlaybackService {
    public static let maximumManifestBytes = 1 << 20
    public static let maximumEnvelopeBytes = 1 << 20
    public static let maximumSignatureBytes = 1 << 16

    private let keyState: MacE2EEKeyStateRepository
    private let opener: any MacProtectedMediaOpening
    private let transport: any MacProtectedMediaPlaybackTransport
    private let cacheRoot: URL
    private let cacheInstallationSecret: Data
    private let currentTimeMS: @Sendable () -> Int64
    private let fixtureMode: Bool

    public init(
        keyState: MacE2EEKeyStateRepository, opener: any MacProtectedMediaOpening,
        transport: any MacProtectedMediaPlaybackTransport, cacheRoot: URL,
        cacheInstallationSecret: Data
    ) throws {
        guard cacheInstallationSecret.count >= 16 else {
            throw MacProtectedMediaPlaybackFailure.invalidRequest
        }
        self.keyState = keyState
        self.opener = opener
        self.transport = transport
        self.cacheRoot = cacheRoot.standardizedFileURL
        self.cacheInstallationSecret = cacheInstallationSecret
        self.currentTimeMS = {
            Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        }
        self.fixtureMode = false
    }

    init(
        auditFixtureKeyState keyState: MacE2EEKeyStateRepository,
        opener: any MacProtectedMediaOpening,
        transport: any MacProtectedMediaPlaybackTransport, cacheRoot: URL,
        cacheInstallationSecret: Data,
        currentTimeMS: @escaping @Sendable () -> Int64 = {
            Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        }
    ) throws {
        guard cacheInstallationSecret.count >= 16 else {
            throw MacProtectedMediaPlaybackFailure.invalidRequest
        }
        self.keyState = keyState
        self.opener = opener
        self.transport = transport
        self.cacheRoot = cacheRoot.standardizedFileURL
        self.cacheInstallationSecret = cacheInstallationSecret
        self.currentTimeMS = currentTimeMS
        self.fixtureMode = true
    }

    public func prepare(
        _ request: MacProtectedMediaPlaybackRequest, nowMS: Int64
    ) async throws -> MacProtectedMediaPreparedPlayback {
        guard opener.productionApproved || fixtureMode else {
            throw MacProtectedMediaPlaybackFailure.productionDisabled
        }
        try validate(request, nowMS: nowMS)
        let identity: MacE2EEDeviceIdentityLease
        let group: MacE2EEGroupStateLease
        do {
            identity = try keyState.loadDeviceIdentity(deviceID: request.recipientDeviceID)
            group = try keyState.loadGroupState(
                installationID: identity.metadata.installationID,
                groupID: request.groupID)
        } catch {
            throw MacProtectedMediaPlaybackFailure.invalidAuthentication
        }
        defer {
            identity.destroy()
            group.destroy()
        }
        guard group.metadata.revision == request.expectedGroupRevision else {
            throw MacProtectedMediaPlaybackFailure.targetChanged
        }

        let route: MacProtectedMediaPlaybackRoute
        do {
            route = try await transport.fetchManifest(
                objectID: request.objectID, recipientDeviceID: request.recipientDeviceID,
                requestedAtMS: nowMS)
        } catch let failure as MacProtectedMediaPlaybackFailure {
            throw failure
        } catch {
            throw MacProtectedMediaPlaybackFailure.transport
        }
        do {
            try validate(route, request: request, group: group.metadata, nowMS: nowMS)
        } catch {
            let permanent = (error as? MacProtectedMediaPlaybackFailure).map {
                $0 == .revoked || $0 == .blocked
            } ?? false
            try? await purge(route, permanent: permanent)
            throw error
        }

        var grantLease: MacE2EESecretLease?
        if route.epoch < group.metadata.epoch {
            guard let grantID = request.historyGrantID else {
                try? await purge(route, permanent: false)
                throw MacProtectedMediaPlaybackFailure.missingGrant
            }
            do {
                let loaded = try keyState.loadGrant(
                    installationID: identity.metadata.installationID,
                    grantID: grantID, nowMS: nowMS)
                guard loaded.0.groupID == route.groupID,
                    loaded.0.firstEpoch <= route.epoch, loaded.0.lastEpoch >= route.epoch
                else { throw MacProtectedMediaPlaybackFailure.missingGrant }
                grantLease = loaded.1
            } catch {
                try? await purge(route, permanent: false)
                throw MacProtectedMediaPlaybackFailure.missingGrant
            }
        }
        defer { grantLease?.destroy() }

        let openLease: MacProtectedMediaOpenLease
        do {
            openLease = try await opener.open(
                route: route, identity: identity, groupState: group,
                historyGrant: grantLease)
        } catch {
            try? await purge(route, permanent: false)
            throw MacProtectedMediaPlaybackFailure.invalidAuthentication
        }
        let authorization = MacProtectedMediaAuthorization()
        let installationID = identity.metadata.installationID
        let clock = currentTimeMS
        let adapter = MacProtectedMediaRangeAdapter(
            route: route, transport: transport, authorization: authorization)
        let cache: MacStreamChunkCache
        do {
            cache = try MacStreamChunkCache(
                root: cacheRoot, installationSecret: cacheInstallationSecret,
                fetcher: adapter)
        } catch {
            openLease.destroy()
            throw MacProtectedMediaPlaybackFailure.invalidRequest
        }
        let reader = MacProtectedMediaChunkReader(
            route: route, cache: cache, opener: opener, lease: openLease,
            authorization: authorization,
            revalidate: { [keyState, clock] in
                let checkedAtMS = clock()
                guard route.expiresAtMS > checkedAtMS else {
                    throw MacProtectedMediaPlaybackFailure.expired
                }
                let current: MacE2EEGroupStateLease
                do {
                    current = try keyState.loadGroupState(
                        installationID: installationID,
                        groupID: route.groupID)
                } catch {
                    throw MacProtectedMediaPlaybackFailure.invalidAuthentication
                }
                defer { current.destroy() }
                guard current.metadata.revision == request.expectedGroupRevision,
                    current.metadata.epoch == group.metadata.epoch,
                    current.metadata.targetSnapshotDigest == group.metadata.targetSnapshotDigest
                else { throw MacProtectedMediaPlaybackFailure.targetChanged }
                if let grantID = request.historyGrantID, route.epoch < group.metadata.epoch {
                    let liveGrant: (MacE2EEGrantMetadata, MacE2EESecretLease)
                    do {
                        liveGrant = try keyState.loadGrant(
                            installationID: installationID, grantID: grantID,
                            nowMS: checkedAtMS)
                    } catch {
                        throw MacProtectedMediaPlaybackFailure.revoked
                    }
                    defer { liveGrant.1.destroy() }
                    guard liveGrant.0.groupID == route.groupID,
                        liveGrant.0.firstEpoch <= route.epoch,
                        liveGrant.0.lastEpoch >= route.epoch
                    else { throw MacProtectedMediaPlaybackFailure.revoked }
                }
            })
        return MacProtectedMediaPreparedPlayback(
            route: route, cache: cache, reader: reader, authorization: authorization)
    }

    private func validate(
        _ request: MacProtectedMediaPlaybackRequest, nowMS: Int64
    ) throws {
        guard Self.validIdentifier(request.objectID),
            Self.validIdentifier(request.recipientDeviceID),
            Self.validIdentifier(request.groupID), request.expectedGroupRevision > 0,
            request.expectedEpoch > 0, request.expectedGeneration > 0,
            Self.validDigest(request.expectedTargetSnapshotDigest), nowMS > 0
        else { throw MacProtectedMediaPlaybackFailure.invalidRequest }
        guard request.policyAllowed, request.dndAllowed, !request.senderBlocked else {
            throw MacProtectedMediaPlaybackFailure.blocked
        }
    }

    private func validate(
        _ route: MacProtectedMediaPlaybackRoute,
        request: MacProtectedMediaPlaybackRequest,
        group: MacE2EEGroupStateMetadata, nowMS: Int64
    ) throws {
        guard route.contract == "e2ee-media-audit.v1",
            route.capability == "e2ee_media_v1"
        else { throw MacProtectedMediaPlaybackFailure.downgradeForbidden }
        guard route.expiresAtMS > nowMS else {
            throw MacProtectedMediaPlaybackFailure.expired
        }
        guard route.targetSnapshotDigest == request.expectedTargetSnapshotDigest else {
            throw MacProtectedMediaPlaybackFailure.targetChanged
        }
        guard !route.suite.isEmpty, route.suite.utf8.count <= 128,
            !route.container.isEmpty, route.container.utf8.count <= 128,
            route.objectID == request.objectID,
            route.recipientDeviceID == request.recipientDeviceID,
            route.groupID == request.groupID,
            route.epoch == request.expectedEpoch,
            route.generation == request.expectedGeneration,
            Self.validIdentifier(route.sourceObjectID),
            Self.validIdentifier(route.authorDeviceID),
            Self.validDigest(route.manifestDigest),
            Self.digest(route.encryptedManifest) == route.manifestDigest,
            !route.encryptedManifest.isEmpty,
            route.encryptedManifest.count <= Self.maximumManifestBytes,
            !route.opaqueKeyEnvelope.isEmpty,
            route.opaqueKeyEnvelope.count <= Self.maximumEnvelopeBytes,
            !route.authenticatedManifest.isEmpty,
            route.authenticatedManifest.count <= Self.maximumManifestBytes,
            !route.signature.isEmpty, route.signature.count <= Self.maximumSignatureBytes
        else { throw MacProtectedMediaPlaybackFailure.invalidManifest }
        do { try route.streamManifest.validate() } catch {
            throw MacProtectedMediaPlaybackFailure.invalidManifest
        }
        guard route.streamManifest.identity.hasPrefix("svm1.protected."),
            route.streamManifest.sha256.count == 64,
            route.streamManifest.sizeBytes <= MacStreamCacheLimits.maximumPerVariantBytes
        else { throw MacProtectedMediaPlaybackFailure.invalidManifest }
        if route.epoch > group.epoch {
            throw MacProtectedMediaPlaybackFailure.forkedEpoch
        }
        if route.epoch == group.epoch,
            route.targetSnapshotDigest != group.targetSnapshotDigest
        { throw MacProtectedMediaPlaybackFailure.targetChanged }
        if route.epoch < group.epoch, request.historyGrantID == nil {
            throw MacProtectedMediaPlaybackFailure.missingGrant
        }
    }

    private func purge(
        _ route: MacProtectedMediaPlaybackRoute, permanent: Bool
    ) async throws {
        guard (try? route.streamManifest.validate()) != nil else { return }
        let authorization = MacProtectedMediaAuthorization()
        authorization.revoke()
        let adapter = MacProtectedMediaRangeAdapter(
            route: route, transport: transport, authorization: authorization)
        let cache = try MacStreamChunkCache(
            root: cacheRoot, installationSecret: cacheInstallationSecret,
            fetcher: adapter)
        if permanent {
            try await cache.tombstone(route.streamManifest)
        } else {
            try await cache.invalidate(route.streamManifest)
        }
    }

    private static func validIdentifier(_ value: String) -> Bool {
        (8...128).contains(value.utf8.count) && !value.contains("/")
            && !value.contains("\\")
    }

    private static func validDigest(_ value: String) -> Bool {
        value.count == 64 && value.utf8.allSatisfy {
            ($0 >= 48 && $0 <= 57) || ($0 >= 97 && $0 <= 102)
        }
    }

    private static func digest(_ data: Data) -> String {
        SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
    }
}
