import Foundation
import Testing

@testable import NodeCore

@Suite(.serialized)
struct PairingClientTests {
  private let code = "ABCDEFGH"

  @Test func validPairCompatibilityUsesBoundedSameOriginRequest() async throws {
    let transport = ScriptedTransport { request, maximum in
      #expect(maximum == PairingClient.maximumResponseBytes)
      return testHTTPResponse(
        request: request, status: 200,
        json:
          #"{"orbit_id":7,"slot":"c","token":"\#(testNodeToken)","ws_url":"wss://coord.example.com/ws"}"#
      )
    }
    let client = try PairingClient(
      coordinatorBase: "https://coord.example.com/path?invite=secret", transport: transport)
    let result = await client.pair(code: code)
    guard case .success(let credentials) = result else {
      Issue.record("Valid pair response was rejected")
      return
    }
    #expect(
      credentials
        == NodeCredentials(
          orbitId: 7, slot: "c", token: testNodeToken,
          wsUrl: "wss://coord.example.com/ws"))
    let request = try #require(transport.requests().first)
    #expect(request.httpMethod == "POST")
    #expect(request.url?.path == "/pair")
    #expect(request.url?.query == nil)
    #expect(request.value(forHTTPHeaderField: "Authorization") == nil)
    let body = try #require(request.httpBody)
    let object = try #require(
      try JSONSerialization.jsonObject(with: body) as? [String: String])
    #expect(object == ["code": code])
  }

  @Test func redirectsCrossOriginOverflowAndPlaintextFailWithoutSecretRetention() async throws {
    for (status, finalURL) in [
      (307, URL(string: "https://coord.example.com/pair")!),
      (308, URL(string: "http://evil.example/pair")!),
      (200, URL(string: "https://evil.example/pair")!),
    ] {
      let transport = ScriptedTransport { request, _ in
        testHTTPResponse(request: request, status: status, json: "{}", url: finalURL)
      }
      let client = try PairingClient(
        coordinatorBase: "https://coord.example.com", transport: transport)
      guard case .failure(.transport) = await client.pair(code: code) else {
        Issue.record("Redirect or cross-origin pair response was accepted")
        continue
      }
      #expect(transport.requests().count == 1)
    }

    let oversized = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 200,
        json: String(repeating: "x", count: PairingClient.maximumResponseBytes + 1))
    }
    let bounded = try PairingClient(
      coordinatorBase: "https://coord.example.com", transport: oversized)
    guard case .failure(.transport) = await bounded.pair(code: code) else {
      Issue.record("Oversized pair response was accepted")
      return
    }
    #expect(throws: PairingError.self) {
      try PairingClient(
        coordinatorBase: "http://coord.example.com",
        transport: ScriptedTransport { _, _ in throw PairTestError.unexpectedSend })
    }
    let canary = "PAIR_CODE_CANARY"
    #expect(!String(reflecting: PairingError.transport).contains(canary))
    #expect(!String(reflecting: PairingError.http(500)).contains(canary))
  }

  @Test func malformedCredentialsAndMediaTypesFailClosed() async throws {
    let valid =
      #"{"orbit_id":7,"slot":"c","token":"\#(testNodeToken)","ws_url":"wss://coord.example.com/ws"}"#
    let responses: [(String, [String: String])] = [
      (valid.replacingOccurrences(of: #""orbit_id":7"#, with: #""orbit_id":0"#), [:]),
      (valid.replacingOccurrences(of: #""slot":"c""#, with: #""slot":"cc""#), [:]),
      (valid.replacingOccurrences(of: testNodeToken, with: "UPPER"), [:]),
      (
        valid.replacingOccurrences(
          of: "wss://coord.example.com/ws", with: "wss://evil.example/ws"), [:]
      ),
      (
        valid.replacingOccurrences(
          of: "wss://coord.example.com/ws", with: "wss://coord.example.com/ws?token=x"), [:]
      ),
      (valid.dropLast() + ",\"unknown\":true}", [:]),
      (valid, ["Content-Type": "application/jsonp"]),
    ]
    for (json, headers) in responses {
      let transport = ScriptedTransport { request, _ in
        testHTTPResponse(
          request: request, status: 200, json: String(json), headers: headers)
      }
      let client = try PairingClient(
        coordinatorBase: "https://coord.example.com", transport: transport)
      guard case .failure(.badResponse) = await client.pair(code: code) else {
        Issue.record("Malformed pair credential was accepted")
        continue
      }
    }
  }
}

private enum PairTestError: Error { case unexpectedSend }
