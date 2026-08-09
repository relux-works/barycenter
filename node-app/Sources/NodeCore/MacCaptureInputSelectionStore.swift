import Foundation

/// Persists only stable Core Audio device UIDs. The former v1 value held an
/// ephemeral numeric `AudioDeviceID`; those values are cleared and recording
/// falls back to the current system default.
public final class MacCaptureInputSelectionStore {
    static let stableKey = "captureInputDeviceUID.v2"
    static let legacyKey = "captureInputDevice.v1"

    private let defaults: UserDefaults

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    public func load(availableDevices: [MacCaptureDevice]) -> String? {
        if let persistedValue = defaults.object(forKey: Self.stableKey) {
            defaults.removeObject(forKey: Self.legacyKey)
            guard let persistedUID = persistedValue as? String,
                availableDevices.contains(where: { $0.id == persistedUID })
            else {
                defaults.removeObject(forKey: Self.stableKey)
                return nil
            }
            return persistedUID
        }

        guard let legacyValue = defaults.object(forKey: Self.legacyKey) else {
            return nil
        }
        defaults.removeObject(forKey: Self.legacyKey)

        // Non-numeric prerelease values may already be stable UIDs. Migrate
        // only when the UID is present now; numeric values are always treated
        // as ephemeral AudioDeviceIDs and deliberately discarded.
        guard let legacy = legacyValue as? String,
            UInt32(legacy) == nil,
            availableDevices.contains(where: { $0.id == legacy })
        else {
            return nil
        }
        defaults.set(legacy, forKey: Self.stableKey)
        return legacy
    }

    public func save(_ deviceUID: String?) {
        defaults.removeObject(forKey: Self.legacyKey)
        if let deviceUID {
            defaults.set(deviceUID, forKey: Self.stableKey)
        } else {
            defaults.removeObject(forKey: Self.stableKey)
        }
    }
}
