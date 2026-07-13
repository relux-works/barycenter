import Foundation
import Testing

@testable import NodeCore

struct CoordinatorOriginTests {
  @Test(arguments: [
    ("https://coord.example.com", "https://coord.example.com"),
    ("https://coord.example.com:443", "https://coord.example.com"),
    ("https://coord.example.com:443/", "https://coord.example.com"),
    ("https://coord.example.com:8443", "https://coord.example.com:8443"),
    ("http://coord.example.com", "http://coord.example.com"),
    ("http://coord.example.com:80", "http://coord.example.com"),
    ("http://coord.example.com:8080", "http://coord.example.com:8080"),
    ("https://COORD.Example.COM", "https://coord.example.com"),
    ("https://coord.example.com.", "https://coord.example.com"),
    ("https://127.0.0.1", "https://127.0.0.1"),
    ("https://[::1]:8443", "https://[::1]:8443"),
    ("https://[0:0:0:0:0:0:0:1]:8443", "https://[::1]:8443"),
    ("https://münchen.example.com", "https://xn--mnchen-3ya.example.com"),
    ("https://faß.de", "https://xn--fa-hia.de"),
    ("https://example。com", "https://example.com"),
    ("https://coord.example.com/path?q=1#frag", "https://coord.example.com"),
  ])
  func frozenVectors(input: String, expected: String) throws {
    #expect(try CoordinatorOrigin(input).rawValue == expected)
  }

  @Test(arguments: [
    "https://coord.example.com.",
    "https://coord.example.com。",
    "https://coord.example.com．",
    "https://coord.example.com｡",
  ])
  func uts46MappingPrecedesSingleRootDotRemoval(input: String) throws {
    #expect(try CoordinatorOrigin(input).rawValue == "https://coord.example.com")
  }

  @Test(arguments: [
    "https://user:pass@coord.example.com",
    "ftp://coord.example.com",
    "https://[::1%25eth0]:8443",
    "https://%63oord.example.com",
    "https://a_b.example.com",
    "https://a\u{200D}b.example.com",
    "https://0177.0.0.1",
    "https://0x7f.0.0.1",
    "https://2130706433",
    "https://127.1",
    "https://256.0.0.1",
    "https://coord.example.com:0",
    "https://coord.example.com:65536",
    "https://coord.example.com:abc",
    "https:///path",
    "https:opaque",
    " https://coord.example.com",
    "https://coord..example.com",
    "https://coord.example.com..",
    "https://coord.example.com。。",
    "https://coord.example.com．｡",
    "https://xn--a.com",
    "https://xn--0.com",
    "https://xn--abc.com",
    "https://aא.example.com",
    "https://\u{200C}a.example.com",
  ])
  func rejectsMalformedAndUnsafeOrigins(input: String) {
    #expect(throws: Error.self) { try CoordinatorOrigin(input) }
  }

  @Test func rejectsOverlengthMappedDomainAndLabel() {
    let fourLabels = Array(repeating: String(repeating: "a", count: 63), count: 4)
      .joined(separator: ".")
    #expect(fourLabels.utf8.count == 255)
    #expect(throws: CoordinatorOriginError.invalidHost) {
      try CoordinatorOrigin("https://\(fourLabels)")
    }
    #expect(throws: CoordinatorOriginError.invalidHost) {
      try CoordinatorOrigin("https://\(String(repeating: "a", count: 64)).example")
    }
  }

  @Test func derivesExactWebSocketAndLoopbackRules() throws {
    #expect(
      try CoordinatorOrigin("https://coord.example.com/base?q=x").webSocketURL?.absoluteString
        == "wss://coord.example.com/ws")
    #expect(
      try CoordinatorOrigin("http://127.0.0.1:8080/path").webSocketURL?.absoluteString
        == "ws://127.0.0.1:8080/ws")
    #expect(try CoordinatorOrigin("http://127.0.0.1").isSecureForCredentials)
    #expect(try CoordinatorOrigin("http://[::1]").isSecureForCredentials)
    #expect(!(try CoordinatorOrigin("http://localhost").isSecureForCredentials))
    #expect(
      URLRedactor.originOnly(URL(string: "wss://user:pass@coord.example.com/ws?token=secret")!)
        == "<invalid-url>")
    #expect(
      URLRedactor.originOnly(URL(string: "wss://coord.example.com/ws?token=secret")!)
        == "wss://coord.example.com")
    #expect(
      URLRedactor.originOnly(
        URL(
          string:
            "https://coord.example.com/join?invite_code=ABCDEFGHJKMNPQRSTVWXYZ23456"
        )!) == "https://coord.example.com")
    #expect(
      URLRedactor.originOnly(
        URL(
          string:
            "https://t.me/barycenter_bot?start=ABCDEFGHJKMNPQRSTVWXYZ23456"
        )!) == "https://t.me")
  }

  @Test func rawValueAndCodableCannotBypassValidation() throws {
    #expect(CoordinatorOrigin(rawValue: "https://user:secret@coord.example.com/path") == nil)
    #expect(CoordinatorOrigin(rawValue: "ftp://coord.example.com") == nil)
    #expect(
      CoordinatorOrigin(rawValue: "HTTPS://COORD.EXAMPLE.COM:443/path")?.rawValue
        == "https://coord.example.com")

    let encoded = try JSONEncoder().encode(try CoordinatorOrigin("https://coord.example.com"))
    #expect(
      try JSONDecoder().decode(String.self, from: encoded)
        == "https://coord.example.com")
    #expect(
      try JSONDecoder().decode(
        CoordinatorOrigin.self,
        from: Data("\"HTTPS://COORD.EXAMPLE.COM:443/path\"".utf8)
      ).rawValue == "https://coord.example.com")
    #expect(throws: DecodingError.self) {
      try JSONDecoder().decode(
        CoordinatorOrigin.self,
        from: Data("\"https://user:secret@coord.example.com/path\"".utf8)
      )
    }
  }
}
