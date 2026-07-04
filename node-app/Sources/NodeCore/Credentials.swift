import Foundation

/// Node credentials obtained through the pairing flow (design §4):
/// `NodeApp --pair CODE --coordinator https://…` exchanges a bot-issued
/// one-time code for these and stores them next to the config
/// (node-credentials.json, 0600). On normal startup they override the yml's
/// coordinator url/token — the user never edits configs. Keychain storage
/// replaces the file in V2-G3.
public struct NodeCredentials: Codable, Equatable {
    public var orbitId: Int64
    public var slot: String
    public var token: String
    public var wsUrl: String

    enum CodingKeys: String, CodingKey {
        case orbitId = "orbit_id", slot, token = "token", wsUrl = "ws_url"
    }

    public static func fileURL(besideConfig configPath: String) -> URL {
        URL(fileURLWithPath: configPath).deletingLastPathComponent()
            .appendingPathComponent("node-credentials.json")
    }

    public static func load(besideConfig configPath: String) -> NodeCredentials? {
        let url = fileURL(besideConfig: configPath)
        guard let data = try? Data(contentsOf: url) else { return nil }
        return try? JSONDecoder().decode(NodeCredentials.self, from: data)
    }

    public func save(besideConfig configPath: String) throws {
        let url = Self.fileURL(besideConfig: configPath)
        let enc = JSONEncoder()
        enc.outputFormatting = [.prettyPrinted, .sortedKeys]
        let data = try enc.encode(self)
        try data.write(to: url, options: .atomic)
        try FileManager.default.setAttributes(
            [.posixPermissions: 0o600], ofItemAtPath: url.path)
    }
}

public enum PairingError: Error, CustomStringConvertible {
    case http(Int, String)
    case transport(String)
    case badResponse

    public var description: String {
        switch self {
        case .http(let code, let body):
            if code == 403 { return "код не подошёл или истёк — попроси новый: /pair у бота" }
            if code == 409 { return "в орбите нет свободных мест" }
            return "сервер ответил \(code): \(body)"
        case .transport(let msg): return "не достучался до координатора: \(msg)"
        case .badResponse: return "непонятный ответ координатора"
        }
    }
}

/// Exchanges a pairing code for credentials: POST {coordinator}/pair.
/// Synchronous by design — it runs from the CLI before anything starts.
public func pairNode(code: String, coordinatorBase: String) -> Result<NodeCredentials, PairingError> {
    let base = coordinatorBase.hasSuffix("/") ? String(coordinatorBase.dropLast()) : coordinatorBase
    guard let url = URL(string: base + "/pair") else {
        return .failure(.transport("кривой адрес: \(coordinatorBase)"))
    }
    var req = URLRequest(url: url, timeoutInterval: 15)
    req.httpMethod = "POST"
    req.setValue("application/json", forHTTPHeaderField: "Content-Type")
    req.httpBody = try? JSONSerialization.data(withJSONObject: ["code": code])

    let sema = DispatchSemaphore(value: 0)
    var result: Result<NodeCredentials, PairingError> = .failure(.badResponse)
    URLSession.shared.dataTask(with: req) { data, resp, err in
        defer { sema.signal() }
        if let err {
            result = .failure(.transport(err.localizedDescription))
            return
        }
        guard let http = resp as? HTTPURLResponse, let data else {
            result = .failure(.badResponse)
            return
        }
        guard http.statusCode == 200 else {
            let body = String(data: data, encoding: .utf8) ?? ""
            result = .failure(.http(http.statusCode, body.trimmingCharacters(in: .whitespacesAndNewlines)))
            return
        }
        guard let creds = try? JSONDecoder().decode(NodeCredentials.self, from: data) else {
            result = .failure(.badResponse)
            return
        }
        result = .success(creds)
    }.resume()
    sema.wait()
    return result
}
