import Darwin
import Foundation
import ICU

public enum CoordinatorOriginError: Error, Equatable, LocalizedError {
  case malformed
  case unsupportedScheme
  case userInfo
  case encodedHost
  case invalidHost
  case invalidPort

  public var errorDescription: String? { "The coordinator address is not valid." }
}

/// Canonical coordinator origin used for both HTTP safety checks and protected
/// recovery-state scope keys.
///
/// ICU performs UTS46 processing with non-transitional ASCII/Unicode, STD3,
/// Bidi, and ContextJ checks. The wrapper also rejects ambiguous IP/authority
/// forms before IDNA processing.
public struct CoordinatorOrigin: RawRepresentable, Codable, Hashable, Sendable,
  CustomStringConvertible
{
  public let rawValue: String

  public init?(rawValue input: String) {
    guard let canonical = try? CoordinatorOrigin(input) else { return nil }
    self = canonical
  }

  public init(_ input: String) throws {
    guard input == input.trimmingCharacters(in: .whitespacesAndNewlines),
      !input.isEmpty,
      let schemeSeparator = input.range(of: "://")
    else {
      throw CoordinatorOriginError.malformed
    }

    let rawScheme = String(input[..<schemeSeparator.lowerBound])
    guard !rawScheme.isEmpty,
      rawScheme.unicodeScalars.allSatisfy({
        CharacterSet(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz").contains(
          $0)
      })
    else {
      throw CoordinatorOriginError.malformed
    }
    let scheme = rawScheme.lowercased()
    guard scheme == "https" || scheme == "http" else {
      throw CoordinatorOriginError.unsupportedScheme
    }

    let authorityStart = schemeSeparator.upperBound
    let suffix = input[authorityStart...]
    let authorityEnd =
      suffix.firstIndex(where: { $0 == "/" || $0 == "?" || $0 == "#" }) ?? input.endIndex
    let authority = String(input[authorityStart..<authorityEnd])
    guard !authority.isEmpty else { throw CoordinatorOriginError.malformed }
    guard !authority.contains("@") else { throw CoordinatorOriginError.userInfo }

    let parsed = try Self.parseAuthority(authority)
    guard !parsed.host.contains("%") else { throw CoordinatorOriginError.encodedHost }

    let canonicalHost: String
    if parsed.ipv6 {
      canonicalHost = try Self.canonicalIPv6(parsed.host)
    } else if Self.looksLikeIPv4(parsed.host) {
      canonicalHost = try Self.canonicalIPv4(parsed.host)
    } else {
      canonicalHost = try Self.canonicalDomain(parsed.host)
    }

    let port: Int?
    if let rawPort = parsed.port {
      guard !rawPort.isEmpty,
        rawPort.utf8.allSatisfy({ $0 >= 48 && $0 <= 57 }),
        let value = Int(rawPort), (1...65_535).contains(value)
      else {
        throw CoordinatorOriginError.invalidPort
      }
      port = value
    } else {
      port = nil
    }

    let renderedHost = parsed.ipv6 ? "[\(canonicalHost)]" : canonicalHost
    if let port, !((scheme == "https" && port == 443) || (scheme == "http" && port == 80)) {
      rawValue = "\(scheme)://\(renderedHost):\(port)"
    } else {
      rawValue = "\(scheme)://\(renderedHost)"
    }
  }

  public var description: String { rawValue }
  public var scheme: String { rawValue.hasPrefix("https://") ? "https" : "http" }

  public var isLiteralLoopback: Bool {
    guard let components = URLComponents(string: rawValue), let host = components.host else {
      return false
    }
    let literal = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
    return literal == "127.0.0.1" || literal == "::1"
  }

  public var isSecureForCredentials: Bool { scheme == "https" || isLiteralLoopback }

  public func endpoint(path: String) -> URL? {
    guard path.hasPrefix("/"), !path.contains("?"), !path.contains("#") else { return nil }
    return URL(string: rawValue + path)
  }

  public var webSocketURL: URL? {
    let prefix = scheme == "https" ? "wss" : "ws"
    guard let separator = rawValue.firstIndex(of: ":") else { return nil }
    return URL(string: prefix + rawValue[separator...] + "/ws")
  }

  public init(from decoder: Decoder) throws {
    let container = try decoder.singleValueContainer()
    let encoded = try container.decode(String.self)
    do {
      self = try CoordinatorOrigin(encoded)
    } catch {
      throw DecodingError.dataCorruptedError(
        in: container, debugDescription: "Invalid coordinator origin")
    }
  }

  public func encode(to encoder: Encoder) throws {
    var container = encoder.singleValueContainer()
    try container.encode(rawValue)
  }

  private struct ParsedAuthority {
    let host: String
    let port: String?
    let ipv6: Bool
  }

  private static func parseAuthority(_ authority: String) throws -> ParsedAuthority {
    if authority.hasPrefix("[") {
      guard let close = authority.firstIndex(of: "]") else {
        throw CoordinatorOriginError.invalidHost
      }
      let host = String(authority[authority.index(after: authority.startIndex)..<close])
      let tail = authority[authority.index(after: close)...]
      guard tail.isEmpty || tail.hasPrefix(":") else { throw CoordinatorOriginError.malformed }
      let port = tail.isEmpty ? nil : String(tail.dropFirst())
      return ParsedAuthority(host: host, port: port, ipv6: true)
    }

    guard !authority.contains("[") && !authority.contains("]") else {
      throw CoordinatorOriginError.invalidHost
    }
    let colonCount = authority.reduce(into: 0) { if $1 == ":" { $0 += 1 } }
    guard colonCount <= 1 else { throw CoordinatorOriginError.invalidHost }
    if let colon = authority.lastIndex(of: ":") {
      return ParsedAuthority(
        host: String(authority[..<colon]),
        port: String(authority[authority.index(after: colon)...]),
        ipv6: false
      )
    }
    return ParsedAuthority(host: authority, port: nil, ipv6: false)
  }

  private static func canonicalIPv6(_ host: String) throws -> String {
    guard !host.isEmpty, !host.contains("%") else { throw CoordinatorOriginError.invalidHost }
    var address = in6_addr()
    guard host.withCString({ inet_pton(AF_INET6, $0, &address) }) == 1 else {
      throw CoordinatorOriginError.invalidHost
    }
    var buffer = [CChar](repeating: 0, count: Int(INET6_ADDRSTRLEN))
    guard inet_ntop(AF_INET6, &address, &buffer, socklen_t(buffer.count)) != nil else {
      throw CoordinatorOriginError.invalidHost
    }
    return String(cString: buffer).lowercased()
  }

  private static func looksLikeIPv4(_ host: String) -> Bool {
    let parts = host.split(separator: ".", omittingEmptySubsequences: false)
    guard !parts.isEmpty else { return false }
    return parts.allSatisfy { part in
      guard !part.isEmpty else { return true }
      if part.hasPrefix("0x") || part.hasPrefix("0X") {
        return part.dropFirst(2).allSatisfy { $0.isHexDigit }
      }
      return part.allSatisfy { $0.isNumber }
    }
  }

  private static func canonicalIPv4(_ host: String) throws -> String {
    let parts = host.split(separator: ".", omittingEmptySubsequences: false)
    guard parts.count == 4 else { throw CoordinatorOriginError.invalidHost }
    var octets: [String] = []
    for part in parts {
      guard !part.isEmpty,
        part.allSatisfy({ $0.isASCII && $0.isNumber }),
        part.count == 1 || part.first != "0",
        let value = Int(part), value <= 255
      else {
        throw CoordinatorOriginError.invalidHost
      }
      octets.append(String(value))
    }
    return octets.joined(separator: ".")
  }

  private static func canonicalDomain(_ rawHost: String) throws -> String {
    guard !rawHost.isEmpty, !rawHost.contains("\\"), rawHost.utf8.count <= 1_024
    else {
      throw CoordinatorOriginError.invalidHost
    }

    var status = U_ZERO_ERROR
    let options = UInt32(
      UIDNA_USE_STD3_RULES | UIDNA_CHECK_BIDI | UIDNA_CHECK_CONTEXTJ
        | UIDNA_NONTRANSITIONAL_TO_ASCII | UIDNA_NONTRANSITIONAL_TO_UNICODE)
    guard let idna = uidna_openUTS46(options, &status), status.rawValue <= 0 else {
      throw CoordinatorOriginError.invalidHost
    }
    defer { uidna_close(idna) }
    var info = UIDNAInfo(
      size: Int16(MemoryLayout<UIDNAInfo>.size), isTransitionalDifferent: 0,
      reservedB3: 0, errors: 0, reservedI2: 0, reservedI3: 0)
    var output = [CChar](repeating: 0, count: 4_096)
    let length = rawHost.withCString { input in
      uidna_nameToASCII_UTF8(
        idna, input, Int32(rawHost.utf8.count), &output, Int32(output.count), &info, &status)
    }
    guard status.rawValue <= 0, info.errors == 0, length > 0 else {
      throw CoordinatorOriginError.invalidHost
    }
    var ascii = String(
      decoding: output.prefix(Int(length)).map { UInt8(bitPattern: $0) }, as: UTF8.self
    ).lowercased()
    if ascii.hasSuffix(".") { ascii.removeLast() }
    guard !ascii.isEmpty, !ascii.hasSuffix("."), ascii.utf8.count <= 253 else {
      throw CoordinatorOriginError.invalidHost
    }
    let labels = ascii.split(separator: ".", omittingEmptySubsequences: false)
    guard !labels.isEmpty else { throw CoordinatorOriginError.invalidHost }
    for label in labels {
      guard (1...63).contains(label.utf8.count),
        label.first != "-", label.last != "-",
        label.utf8.allSatisfy({ byte in
          (byte >= 97 && byte <= 122) || (byte >= 48 && byte <= 57) || byte == 45
        })
      else {
        throw CoordinatorOriginError.invalidHost
      }
    }
    return ascii
  }
}

public enum URLRedactor {
  public static func originOnly(_ url: URL) -> String {
    let input = url.absoluteString
    if let origin = try? CoordinatorOrigin(input) { return origin.rawValue }

    // WebSocket diagnostics use the same host/port rules through a
    // temporary HTTP scheme, then restore ws/wss in the rendered origin.
    if let scheme = url.scheme?.lowercased(), scheme == "ws" || scheme == "wss" {
      let mapped = (scheme == "wss" ? "https" : "http") + input.dropFirst(scheme.count)
      if let origin = try? CoordinatorOrigin(mapped) {
        let prefix = scheme == "wss" ? "wss" : "ws"
        return prefix + origin.rawValue.dropFirst(origin.scheme.count)
      }
    }
    return "<invalid-url>"
  }

  static func safeProtocolType(_ value: String) -> String {
    guard !value.isEmpty, value.utf8.count <= 64,
      value.utf8.allSatisfy({
        ($0 >= 97 && $0 <= 122) || ($0 >= 48 && $0 <= 57) || $0 == 95
      })
    else { return "<invalid-type>" }
    return value
  }
}
