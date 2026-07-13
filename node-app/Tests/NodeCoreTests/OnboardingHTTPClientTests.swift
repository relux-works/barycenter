import Foundation
import Testing

@testable import NodeCore

@Suite(.serialized)
struct OnboardingHTTPClientTests {
  private let attemptID = "installation-attempt-0001"
  private let humanCode = "ABCDEFGHJKMNPQRSTVWXYZ23456"

  @Test func exactRequestsAndSuccessDecodingForAllEndpoints() async throws {
    let transport = ScriptedTransport { request, _ in
      let body: String
      let status: Int
      switch (request.httpMethod, request.url?.path) {
      case ("POST", "/v1/onboarding/orbits"):
        status = 201
        body = self.createdOrbitJSON
      case ("POST", "/v1/device-invites"):
        status = 201
        body =
          #"{"invite_code":"\#(self.humanCode)","intended_role":"companion","expires_at":"2030-01-02T03:04:05Z"}"#
      case ("POST", "/v1/device-invites/consume"):
        status = 200
        body = self.joinedOrbitJSON
      case ("GET", "/v1/actor/context"):
        status = 200
        body = self.contextJSON
      case ("POST", "/v1/recovery/consume"):
        status = 200
        body = self.contextJSON
      case ("POST", "/v1/recovery/rotate"):
        status = 200
        body =
          #"{"actor_id":2,"recovery_id":"\#(testRecoveryID)","recovery_secret":"\#(testRecoverySecret)","shown_once":true}"#
      case ("POST", "/v1/telegram-links"):
        status = 201
        body =
          #"{"link_code":"\#(self.humanCode)","desired_role":"satellite","expires_at":"2030-01-02T03:04:05Z","bot_username":"barycenter_bot"}"#
      default:
        throw TestTransportError.unexpectedRequest
      }
      return testHTTPResponse(request: request, status: status, json: body)
    }
    let client = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com/base?secret=no", transport: transport)
    let control = ControlCapability(
      actorId: 2, orbitId: 1, role: .primary, controlToken: testControlToken)

    let created = try await client.createOrbit(title: " Orbit ", installationAttemptID: attemptID)
    #expect(created.title == "Orbit")
    #expect(created.bundle.node?.wsUrl == "wss://coord.example.com/ws")
    #expect(created.bundle.node?.nodeToken == testNodeToken)
    #expect(created.bundle.control?.controlToken == testControlToken)
    _ = try await client.issueDeviceInvite(control: control)
    let joined = try await client.consumeDeviceInvite("ABCD-EFGH-JKMN-PQRS-TVWX-YZ23-456")
    #expect(joined.bundle.node?.wsUrl == "wss://coord.example.com/ws")
    #expect(
      try await client.probe(token: testControlToken)
        == .active(
          ActorCredentialContext(orbitId: 1, actorId: 2, role: .primary)
        ))
    let recoveryContext = try await client.consumeRecovery(
      recoveryID: testRecoveryID,
      secret: RecoverySecret(validated: testRecoverySecret),
      replacementControlToken: testPendingToken
    )
    #expect(recoveryContext.actorId == 2)
    _ = try await client.rotateRecovery(control: control)
    let telegram = try await client.issueTelegramLink(control: control, desiredRole: .satellite)
    #expect(telegram.code == humanCode)
    #expect(telegram.botUsername == "barycenter_bot")

    let requests = transport.requests()
    #expect(
      requests.map { $0.url?.path } == [
        "/v1/onboarding/orbits", "/v1/device-invites", "/v1/device-invites/consume",
        "/v1/actor/context", "/v1/recovery/consume", "/v1/recovery/rotate",
        "/v1/telegram-links",
      ])
    #expect(requests.map(\.httpMethod) == ["POST", "POST", "POST", "GET", "POST", "POST", "POST"])
    #expect(requests[0].value(forHTTPHeaderField: "Authorization") == nil)
    #expect(requests[1].value(forHTTPHeaderField: "Authorization") == "Bearer \(testControlToken)")
    #expect(requests[2].value(forHTTPHeaderField: "Authorization") == nil)
    #expect(requests[3].value(forHTTPHeaderField: "Authorization") == "Bearer \(testControlToken)")
    #expect(requests[4].value(forHTTPHeaderField: "Authorization") == nil)
    #expect(requests[5].value(forHTTPHeaderField: "Authorization") == "Bearer \(testControlToken)")
    #expect(requests[6].value(forHTTPHeaderField: "Authorization") == "Bearer \(testControlToken)")
    #expect(
      try jsonBody(requests[0]) == [
        "installation_attempt_id": .string(attemptID), "title": .string("Orbit"),
      ])
    #expect(try jsonBody(requests[1]) == ["intended_role": .string("companion")])
    #expect(try jsonBody(requests[2]) == ["invite_code": .string(humanCode)])
    #expect(requests[3].httpBody == nil)
    #expect(
      try jsonBody(requests[4]) == [
        "recovery_id": .string(testRecoveryID),
        "recovery_secret": .string(testRecoverySecret),
        "replacement_control_token": .string(testPendingToken),
      ])
    #expect(try jsonBody(requests[5]).isEmpty)
    #expect(try jsonBody(requests[6]) == ["desired_role": .string("satellite")])
    for request in requests {
      #expect(request.url?.query == nil)
      #expect(request.url?.fragment == nil)
      #expect(request.url?.user == nil)
      #expect(request.url?.password == nil)
    }
  }

  @Test func strictResponseBoundariesAndNoAutomaticRetry() async throws {
    let cases: [(String, Int, String, URL?, [String: String], OnboardingClientError)] = [
      (
        "duplicate", 201,
        createdOrbitJSON.replacingOccurrences(
          of: "{", with: "{\"title\":\"other\",", options: [],
          range: createdOrbitJSON
            .startIndex..<createdOrbitJSON.index(after: createdOrbitJSON.startIndex)), nil, [:],
        .invalidResponse
      ),
      ("trailing", 201, createdOrbitJSON + " false", nil, [:], .invalidResponse),
      (
        "unknown", 201, createdOrbitJSON.dropLast() + ",\"extra\":true}", nil, [:], .invalidResponse
      ),
      (
        "wrong scalar", 201,
        createdOrbitJSON.replacingOccurrences(of: "\"orbit_id\":1", with: "\"orbit_id\":\"1\""),
        nil, [:], .invalidResponse
      ),
      (
        "cross-origin", 201, createdOrbitJSON,
        URL(string: "https://evil.example/v1/onboarding/orbits"), [:], .redirectRejected
      ),
      ("redirect", 302, "{}", nil, [:], .redirectRejected),
      ("wrong mime", 201, createdOrbitJSON, nil, ["Content-Type": "text/plain"], .invalidResponse),
      (
        "jsonp mime", 201, createdOrbitJSON, nil, ["Content-Type": "application/jsonp"],
        .invalidResponse
      ),
    ]
    for (name, status, json, finalURL, headers, expected) in cases {
      let transport = ScriptedTransport { request, _ in
        testHTTPResponse(
          request: request, status: status, json: String(json), url: finalURL, headers: headers)
      }
      let client = try OnboardingHTTPClient(
        coordinator: "https://coord.example.com", transport: transport)
      do {
        _ = try await client.createOrbit(title: "Orbit", installationAttemptID: attemptID)
        Issue.record("Malformed response was accepted")
      } catch {
        #expect((error as? OnboardingClientError) == expected, "case: \(name), error: \(error)")
      }
      #expect(transport.requests().count == 1)
    }

    let oversized = ScriptedTransport { request, _ in
      testHTTPResponse(request: request, status: 201, json: String(repeating: "x", count: 129))
    }
    let bounded = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: oversized, maximumResponseBytes: 128
    )
    await #expect(throws: OnboardingClientError.responseTooLarge) {
      try await bounded.createOrbit(title: "Orbit", installationAttemptID: attemptID)
    }
  }

  @Test func strictJSONNestingDepthIsConservativelyBounded() async throws {
    let accepted =
      String(repeating: "[", count: StrictJSONParser.maximumNestingDepth)
      + "null"
      + String(repeating: "]", count: StrictJSONParser.maximumNestingDepth)
    var acceptedParser = StrictJSONParser(Data(accepted.utf8))
    _ = try acceptedParser.parse()

    let rejected =
      String(repeating: "[", count: StrictJSONParser.maximumNestingDepth + 1)
      + "null"
      + String(repeating: "]", count: StrictJSONParser.maximumNestingDepth + 1)
    var rejectedParser = StrictJSONParser(Data(rejected.utf8))
    #expect(throws: StrictJSONError.self) { try rejectedParser.parse() }

    let transport = ScriptedTransport { request, _ in
      testHTTPResponse(request: request, status: 201, json: rejected)
    }
    let client = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: transport)
    await #expect(throws: OnboardingClientError.invalidResponse) {
      try await client.createOrbit(title: "Orbit", installationAttemptID: attemptID)
    }
  }

  @Test func uniformErrorsCancellationAndCanariesAreRedacted() async throws {
    let rateLimited = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 429,
        json:
          #"{"error":{"code":"too_many_attempts","message":"Too many attempts. Please wait before retrying.","retry_after_seconds":17}}"#,
        headers: ["Retry-After": "17"]
      )
    }
    let client = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: rateLimited)
    #expect(try await client.probe(token: testControlToken) == .rateLimited(17))

    let cancelled = ScriptedTransport { _, _ in throw CancellationError() }
    let cancelledClient = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: cancelled)
    await #expect(throws: OnboardingClientError.cancelled) {
      try await cancelledClient.probe(token: testControlToken)
    }
    let urlCancelled = ScriptedTransport { _, _ in throw URLError(.cancelled) }
    let urlCancelledClient = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: urlCancelled)
    await #expect(throws: OnboardingClientError.cancelled) {
      try await urlCancelledClient.probe(token: testControlToken)
    }

    let canary = "RECOVERY_SECRET_CANARY"
    let leaking = ScriptedTransport { _, _ in throw TestTransportError.containsSecret(canary) }
    let leakingClient = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: leaking)
    do {
      _ = try await leakingClient.consumeDeviceInvite(humanCode)
      Issue.record("Transport error unexpectedly succeeded")
    } catch {
      #expect(!String(describing: error).contains(canary))
      #expect(!String(reflecting: error).contains(canary))
      #expect((error as? OnboardingClientError) == .transport)
    }
  }

  @Test func plaintextAndRedirectPolicyRejectUnsafeOrigins() throws {
    #expect(throws: OnboardingClientError.insecureTransport) {
      try OnboardingHTTPClient(
        coordinator: "http://coord.example.com",
        transport: ScriptedTransport { _, _ in
          throw TestTransportError.unexpectedRequest
        })
    }
    _ = try OnboardingHTTPClient(
      coordinator: "http://127.0.0.1:8080",
      transport: ScriptedTransport { _, _ in
        throw TestTransportError.unexpectedRequest
      })
    _ = try OnboardingHTTPClient(
      coordinator: "http://[::1]:8080",
      transport: ScriptedTransport { _, _ in
        throw TestTransportError.unexpectedRequest
      })
    #expect(throws: OnboardingClientError.invalidRequest) {
      try OnboardingHTTPClient(
        coordinator: "https://coord.example.com",
        transport: ScriptedTransport { _, _ in throw TestTransportError.unexpectedRequest },
        maximumResponseBytes: 0)
    }
    #expect(throws: OnboardingClientError.invalidRequest) {
      try OnboardingHTTPClient(
        coordinator: "https://coord.example.com",
        transport: ScriptedTransport { _, _ in throw TestTransportError.unexpectedRequest },
        maximumResponseBytes: OnboardingHTTPClient.hardMaximumResponseBytes + 1)
    }
  }

  @Test func endpointSpecificSuccessSemanticsRejectNoncanonicalServerValues() async throws {
    let invalidCreate =
      [
        createdOrbitJSON.replacingOccurrences(
          of: #""role":"primary""#, with: #""role":"companion""#),
        createdOrbitJSON.replacingOccurrences(
          of: #""title":"Orbit""#, with: #""title":"""#),
        createdOrbitJSON.replacingOccurrences(
          of: #""recovery_secret":"\#(testRecoverySecret)""#,
          with: #""recovery_secret":"abcd-efgh-jkmn-pqrs-tvwx-yz23-456""#),
      ]
      + ["", "A", "aa", "é"].map { slot in
        createdOrbitJSON.replacingOccurrences(
          of: #""slot":"a""#, with: #""slot":"\#(slot)""#)
      }
    for json in invalidCreate {
      let transport = ScriptedTransport { request, _ in
        testHTTPResponse(request: request, status: 201, json: json)
      }
      let client = try OnboardingHTTPClient(
        coordinator: "https://coord.example.com", transport: transport)
      await #expect(throws: OnboardingClientError.invalidResponse) {
        try await client.createOrbit(title: "Orbit", installationAttemptID: attemptID)
      }
    }

    let control = ControlCapability(
      actorId: 2, orbitId: 1, role: .primary, controlToken: testControlToken)
    let noncanonicalInvite = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 201,
        json:
          #"{"invite_code":"abcd-efgh-jkmn-pqrs-tvwx-yz23-456","intended_role":"companion","expires_at":"2030-01-02T03:04:05Z"}"#
      )
    }
    let inviteClient = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: noncanonicalInvite)
    await #expect(throws: OnboardingClientError.invalidResponse) {
      try await inviteClient.issueDeviceInvite(control: control)
    }

    let invalidJoinResponses =
      [
        joinedOrbitJSON.replacingOccurrences(
          of: #""role":"companion""#, with: #""role":"primary""#)
      ]
      + ["", "B", "bb", "é"].map { slot in
        joinedOrbitJSON.replacingOccurrences(
          of: #""slot":"b""#, with: #""slot":"\#(slot)""#)
      }
    for invalidJoinJSON in invalidJoinResponses {
      let invalidJoin = ScriptedTransport { request, _ in
        testHTTPResponse(request: request, status: 200, json: invalidJoinJSON)
      }
      let joinClient = try OnboardingHTTPClient(
        coordinator: "https://coord.example.com", transport: invalidJoin)
      await #expect(throws: OnboardingClientError.invalidResponse) {
        try await joinClient.consumeDeviceInvite(humanCode)
      }
    }

    let atUsername = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 201,
        json:
          #"{"link_code":"\#(self.humanCode)","desired_role":"companion","expires_at":"2030-01-02T03:04:05Z","bot_username":"@barycenter_bot"}"#
      )
    }
    let telegramClient = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: atUsername)
    await #expect(throws: OnboardingClientError.invalidResponse) {
      try await telegramClient.issueTelegramLink(control: control)
    }
  }

  @Test func createRequiresByteExactEchoOfTrimmedSubmittedTitle() async throws {
    let cases: [(String, String, String, String)] = [
      ("response whitespace", " Orbit ", " Orbit ", "Orbit"),
      ("response case", "Orbit", "orbit", "Orbit"),
      ("response normalization", "Caf\u{00E9}", "Cafe\u{0301}", "Caf\u{00E9}"),
    ]
    for (name, submittedTitle, responseTitle, expectedSubmittedTitle) in cases {
      let responseJSON = createdOrbitJSON.replacingOccurrences(
        of: #""title":"Orbit""#,
        with: #""title":\#(encodedJSONString(responseTitle))"#)
      let transport = ScriptedTransport { request, _ in
        testHTTPResponse(request: request, status: 201, json: responseJSON)
      }
      let client = try OnboardingHTTPClient(
        coordinator: "https://coord.example.com", transport: transport)
      do {
        _ = try await client.createOrbit(
          title: submittedTitle, installationAttemptID: attemptID)
        Issue.record("Mismatched title echo was accepted: \(name)")
      } catch {
        #expect((error as? OnboardingClientError) == .invalidResponse, "case: \(name)")
        #expect(!String(describing: error).contains(testRecoverySecret))
        #expect(!String(reflecting: error).contains(testRecoverySecret))
      }
      let request = try #require(transport.requests().first)
      #expect(
        try jsonBody(request)["title"] == .string(expectedSubmittedTitle),
        "case: \(name)")
    }
  }

  @Test func nonRateLimitErrorsRequireExactNullRetryRepresentation() async throws {
    let responseCases: [(Int, String, String)] = [
      (400, "invalid_request", "The request is malformed or contains invalid parameters."),
      (401, "unauthorized", "Authentication is required."),
      (
        403, "insufficient_capability",
        "This token does not have the required capability."
      ),
      (500, "internal_error", "An internal error occurred."),
    ]
    let malformedValues = [
      #""17""#, "false", "{}", "[]", "0", "-1", "1.5", "1e1",
      "9223372036854775808",
    ]
    let control = ControlCapability(
      actorId: 2, orbitId: 1, role: .primary, controlToken: testControlToken)

    for (status, code, message) in responseCases {
      for retryLiteral in malformedValues {
        let responseJSON = errorEnvelope(
          code: code, message: message, retryLiteral: retryLiteral)
        let transport = ScriptedTransport { request, _ in
          testHTTPResponse(request: request, status: status, json: responseJSON)
        }
        let client = try OnboardingHTTPClient(
          coordinator: "https://coord.example.com", transport: transport)
        do {
          switch status {
          case 400, 500:
            _ = try await client.createOrbit(title: "Orbit", installationAttemptID: attemptID)
          case 401:
            _ = try await client.probe(token: testControlToken)
          case 403:
            _ = try await client.issueDeviceInvite(control: control)
          default:
            Issue.record("Unexpected test status")
          }
          Issue.record(
            "Malformed retry_after_seconds was accepted: status \(status), value \(retryLiteral)"
          )
        } catch {
          #expect(
            (error as? OnboardingClientError) == .invalidResponse,
            "status: \(status), value: \(retryLiteral), error: \(error)")
        }
      }
    }

    let unexpectedHeader = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 400,
        json: errorEnvelope(
          code: "invalid_request",
          message: "The request is malformed or contains invalid parameters.",
          retryLiteral: "null"),
        headers: ["Retry-After": "17"])
    }
    let headerClient = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: unexpectedHeader)
    await #expect(throws: OnboardingClientError.invalidResponse) {
      try await headerClient.createOrbit(title: "Orbit", installationAttemptID: attemptID)
    }
  }

  @Test func rateLimitRequiresCanonicalMatchingBodyAndHeaderRepresentations() async throws {
    let overflow = "9223372036854775808"
    let cases: [(String, String?, String?)] = [
      ("missing body field", nil, "17"),
      ("body null", "null", "17"),
      ("body string", #""17""#, "17"),
      ("body boolean", "false", "17"),
      ("body object", "{}", "17"),
      ("body array", "[]", "17"),
      ("body zero", "0", "0"),
      ("body negative", "-1", "-1"),
      ("body fractional", "17.0", "17"),
      ("body exponent", "1.7e1", "17"),
      ("body leading zero", "017", "17"),
      ("body signed", "+17", "17"),
      ("body overflow", overflow, overflow),
      ("header absent", "17", nil),
      ("header signed", "17", "+17"),
      ("header padded", "17", "0017"),
      ("header leading whitespace", "17", " 17"),
      ("header trailing whitespace", "17", "17 "),
      ("header fractional", "17", "17.0"),
      ("header exponent", "17", "1.7e1"),
      ("header overflow", "17", overflow),
      ("header mismatch", "17", "18"),
    ]
    for (name, retryLiteral, retryHeader) in cases {
      let responseJSON = errorEnvelope(
        code: "too_many_attempts",
        message: "Too many attempts. Please wait before retrying.",
        retryLiteral: retryLiteral)
      let headers = retryHeader.map { ["Retry-After": $0] } ?? [:]
      let transport = ScriptedTransport { request, _ in
        testHTTPResponse(
          request: request, status: 429, json: responseJSON, headers: headers)
      }
      let client = try OnboardingHTTPClient(
        coordinator: "https://coord.example.com", transport: transport)
      do {
        _ = try await client.probe(token: testControlToken)
        Issue.record("Malformed rate limit representation was accepted: \(name)")
      } catch {
        #expect((error as? OnboardingClientError) == .invalidResponse, "case: \(name)")
      }
    }
  }

  @Test func groupedRecoveryInputIsSentCanonicalAndConcurrentEncodingIsIndependent() async throws {
    let recoveryTransport = ScriptedTransport { request, _ in
      testHTTPResponse(request: request, status: 200, json: self.contextJSON)
    }
    let recoveryClient = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: recoveryTransport)
    _ = try await recoveryClient.consumeRecovery(
      recoveryID: testRecoveryID,
      secret: RecoverySecret(validated: "abcd-efgh-jkmn-pqrs-tvwx-yz23-456"),
      replacementControlToken: testPendingToken)
    let body = String(
      decoding: try #require(recoveryTransport.requests().first?.httpBody), as: UTF8.self)
    #expect(body.contains(testRecoverySecret))
    #expect(!body.contains("abcd-efgh"))

    let barrier = RequestBarrier()
    let createdTemplate = createdOrbitJSON
    let concurrentTransport = ScriptedTransport { request, _ in
      await barrier.arriveAndWait()
      let object = request.httpBody.flatMap { data -> [String: Any]? in
        (try? JSONSerialization.jsonObject(with: data)) as? [String: Any]
      }
      let title = object?["title"] as? String ?? "missing"
      return testHTTPResponse(
        request: request, status: 201,
        json: createdTemplate.replacingOccurrences(
          of: #""title":"Orbit""#, with: #""title":"\#(title)""#))
    }
    let concurrentClient = try OnboardingHTTPClient(
      coordinator: "https://coord.example.com", transport: concurrentTransport)
    let first = Task {
      try await concurrentClient.createOrbit(
        title: "First", installationAttemptID: "installation-attempt-first")
    }
    let second = Task {
      try await concurrentClient.createOrbit(
        title: "Second", installationAttemptID: "installation-attempt-second")
    }
    await barrier.waitForArrivals(2)
    await barrier.releaseAll()
    #expect(try await first.value.title == "First")
    #expect(try await second.value.title == "Second")
  }

  @Test func productionTransportStopsChunkedOverflowAtLimitPlusOne() async throws {
    let limit = 32 * 1_024
    let prefixChunks = Array(repeating: Data(repeating: 0x78, count: 4_096), count: 8)
    StreamingURLProtocol.reset(
      chunks: prefixChunks + [Data([0x78])]
        + Array(repeating: Data(repeating: 0x79, count: 4_096), count: 32),
      headers: [:])
    let configuration = URLSessionConfiguration.ephemeral
    configuration.protocolClasses = [StreamingURLProtocol.self]
    let transport = URLSessionOnboardingTransport(configuration: configuration)
    let request = URLRequest(url: URL(string: "https://coord.example.com/test")!)
    let overflow = Task {
      try await transport.send(request, maximumResponseBytes: limit)
    }
    await StreamingURLProtocol.waitUntilStarted()
    for expectedCount in stride(from: 4_096, through: limit, by: 4_096) {
      StreamingURLProtocol.allowNextChunk()
      await StreamingURLProtocol.waitUntilSent(byteCount: expectedCount)
    }
    StreamingURLProtocol.allowNextChunk()
    await StreamingURLProtocol.waitUntilSent(byteCount: limit + 1)
    await #expect(throws: OnboardingClientError.responseTooLarge) {
      try await overflow.value
    }
    await StreamingURLProtocol.waitUntilStopped()
    #expect(StreamingURLProtocol.sentByteCount() == limit + 1)
    #expect(StreamingURLProtocol.wasStopped())

    StreamingURLProtocol.reset(
      chunks: prefixChunks,
      headers: ["Content-Length": "\(limit + 1)", "Content-Type": "application/json"])
    let declaredOversize = Task {
      try await transport.send(request, maximumResponseBytes: limit)
    }
    await StreamingURLProtocol.waitUntilStarted()
    await #expect(throws: OnboardingClientError.responseTooLarge) {
      try await declaredOversize.value
    }
    await StreamingURLProtocol.waitUntilStopped()
    #expect(StreamingURLProtocol.sentByteCount() == 0)

    StreamingURLProtocol.reset(chunks: prefixChunks, headers: [:])
    let cancelled = Task {
      try await transport.send(request, maximumResponseBytes: limit)
    }
    await StreamingURLProtocol.waitUntilStarted()
    cancelled.cancel()
    await #expect(throws: OnboardingClientError.cancelled) {
      try await cancelled.value
    }
    await StreamingURLProtocol.waitUntilStopped()
    #expect(StreamingURLProtocol.sentByteCount() == 0)
  }

  private var createdOrbitJSON: String {
    #"{"orbit_id":1,"title":"Orbit","actor_id":2,"role":"primary","slot":"a","node_token":"\#(testNodeToken)","control_token":"\#(testControlToken)","recovery_id":"\#(testRecoveryID)","recovery_secret":"\#(testRecoverySecret)","shown_once":true}"#
  }

  private var joinedOrbitJSON: String {
    #"{"orbit_id":1,"title":"Orbit","actor_id":2,"role":"companion","slot":"b","node_token":"\#(testNodeToken)","control_token":"\#(testControlToken)"}"#
  }

  private var contextJSON: String { #"{"orbit_id":1,"actor_id":2,"role":"primary"}"# }
}

private enum TestTransportError: Error {
  case unexpectedRequest
  case containsSecret(String)
}

private func encodedJSONString(_ value: String) -> String {
  String(decoding: try! JSONEncoder().encode(value), as: UTF8.self)
}

private func errorEnvelope(code: String, message: String, retryLiteral: String?) -> String {
  let retryField = retryLiteral.map { ",\"retry_after_seconds\":\($0)" } ?? ""
  return "{\"error\":{\"code\":\"\(code)\",\"message\":\"\(message)\"\(retryField)}}"
}

private final class StreamingURLProtocol: URLProtocol, @unchecked Sendable {
  private static let condition = NSCondition()
  private static var responseChunks: [Data] = []
  private static var responseHeaders: [String: String] = [:]
  private static var sentBytes = 0
  private static var stopped = false
  private static var started = false
  private static var permits = 0
  private static var startWaiters: [CheckedContinuation<Void, Never>] = []
  private static var stopWaiters: [CheckedContinuation<Void, Never>] = []
  private static var sentWaiters: [(Int, CheckedContinuation<Void, Never>)] = []

  static func reset(chunks: [Data], headers: [String: String]) {
    condition.lock()
    responseChunks = chunks
    responseHeaders = headers
    sentBytes = 0
    stopped = false
    started = false
    permits = 0
    condition.unlock()
  }

  static func sentByteCount() -> Int {
    condition.lock()
    defer { condition.unlock() }
    return sentBytes
  }

  static func wasStopped() -> Bool {
    condition.lock()
    defer { condition.unlock() }
    return stopped
  }

  static func waitUntilStarted() async {
    await withCheckedContinuation { continuation in
      condition.lock()
      if started {
        condition.unlock()
        continuation.resume()
      } else {
        startWaiters.append(continuation)
        condition.unlock()
      }
    }
  }

  static func allowNextChunk() {
    condition.lock()
    permits += 1
    condition.broadcast()
    condition.unlock()
  }

  static func waitUntilSent(byteCount: Int) async {
    await withCheckedContinuation { continuation in
      condition.lock()
      if sentBytes >= byteCount || stopped {
        condition.unlock()
        continuation.resume()
      } else {
        sentWaiters.append((byteCount, continuation))
        condition.unlock()
      }
    }
  }

  static func waitUntilStopped() async {
    await withCheckedContinuation { continuation in
      condition.lock()
      if stopped {
        condition.unlock()
        continuation.resume()
      } else {
        stopWaiters.append(continuation)
        condition.unlock()
      }
    }
  }

  override class func canInit(with request: URLRequest) -> Bool { true }
  override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

  override func startLoading() {
    let snapshot = Self.lockedSnapshot()
    let response = HTTPURLResponse(
      url: request.url!, statusCode: 200, httpVersion: "HTTP/1.1",
      headerFields: snapshot.headers)!
    client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
    Self.markStarted()
    DispatchQueue.global(qos: .userInitiated).async { [weak self] in
      self?.deliver(snapshot.chunks)
    }
  }

  override func stopLoading() {
    Self.condition.lock()
    Self.stopped = true
    let stopWaiters = Self.stopWaiters
    let sentWaiters = Self.sentWaiters.map(\.1)
    Self.stopWaiters.removeAll()
    Self.sentWaiters.removeAll()
    Self.condition.broadcast()
    Self.condition.unlock()
    for waiter in stopWaiters { waiter.resume() }
    for waiter in sentWaiters { waiter.resume() }
  }

  private static func lockedSnapshot() -> (chunks: [Data], headers: [String: String]) {
    condition.lock()
    defer { condition.unlock() }
    return (responseChunks, responseHeaders)
  }

  private static func markStarted() {
    condition.lock()
    started = true
    let waiters = startWaiters
    startWaiters.removeAll()
    condition.broadcast()
    condition.unlock()
    for waiter in waiters { waiter.resume() }
  }

  private func deliver(_ chunks: [Data]) {
    for chunk in chunks {
      Self.condition.lock()
      while Self.permits == 0 && !Self.stopped { Self.condition.wait() }
      guard !Self.stopped else {
        Self.condition.unlock()
        return
      }
      Self.permits -= 1
      Self.sentBytes += chunk.count
      let ready = Self.sentWaiters.filter { Self.sentBytes >= $0.0 }
      Self.sentWaiters.removeAll { Self.sentBytes >= $0.0 }
      Self.condition.broadcast()
      Self.condition.unlock()
      for waiter in ready { waiter.1.resume() }
      client?.urlProtocol(self, didLoad: chunk)
    }
    Self.condition.lock()
    let shouldFinish = !Self.stopped
    Self.condition.unlock()
    if shouldFinish { client?.urlProtocolDidFinishLoading(self) }
  }
}

private enum TestJSONScalar: Equatable {
  case string(String)
}

private func jsonBody(_ request: URLRequest) throws -> [String: TestJSONScalar] {
  guard let data = request.httpBody,
    let object = try JSONSerialization.jsonObject(with: data) as? [String: Any]
  else {
    throw TestTransportError.unexpectedRequest
  }
  var result: [String: TestJSONScalar] = [:]
  for (key, value) in object {
    guard let string = value as? String else { throw TestTransportError.unexpectedRequest }
    result[key] = .string(string)
  }
  return result
}
