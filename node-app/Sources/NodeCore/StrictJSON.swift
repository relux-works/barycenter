import Foundation

enum StrictJSONError: Error { case invalid }

indirect enum StrictJSONValue {
  case object([String: StrictJSONValue])
  case array([StrictJSONValue])
  case string(String)
  case number(String)
  case bool(Bool)
  case null

  var object: [String: StrictJSONValue]? {
    guard case .object(let value) = self else { return nil }
    return value
  }

  var string: String? {
    guard case .string(let value) = self else { return nil }
    return value
  }

  var bool: Bool? {
    guard case .bool(let value) = self else { return nil }
    return value
  }

  var int64: Int64? {
    guard case .number(let value) = self,
      !value.contains("."), !value.contains("e"), !value.contains("E")
    else { return nil }
    return Int64(value)
  }

  var positiveInt: Int? {
    guard let value = int64, value > 0, value <= Int64(Int.max) else { return nil }
    return Int(value)
  }
}

struct StrictJSONParser {
  static let maximumNestingDepth = 32
  private let bytes: [UInt8]
  private var index = 0

  init(_ data: Data) { bytes = Array(data) }

  mutating func parse() throws -> StrictJSONValue {
    skipWhitespace()
    let value = try parseValue(depth: 0)
    skipWhitespace()
    guard index == bytes.count else { throw StrictJSONError.invalid }
    return value
  }

  private mutating func parseValue(depth: Int) throws -> StrictJSONValue {
    guard index < bytes.count, depth <= Self.maximumNestingDepth else {
      throw StrictJSONError.invalid
    }
    switch bytes[index] {
    case 0x7B: return try parseObject(depth: depth)
    case 0x5B: return try parseArray(depth: depth)
    case 0x22: return .string(try parseString())
    case 0x74:
      try consume("true")
      return .bool(true)
    case 0x66:
      try consume("false")
      return .bool(false)
    case 0x6E:
      try consume("null")
      return .null
    case 0x2D, 0x30...0x39: return .number(try parseNumber())
    default: throw StrictJSONError.invalid
    }
  }

  private mutating func parseObject(depth: Int) throws -> StrictJSONValue {
    index += 1
    skipWhitespace()
    var object: [String: StrictJSONValue] = [:]
    if consumeIf(0x7D) { return .object(object) }
    while true {
      guard index < bytes.count, bytes[index] == 0x22 else { throw StrictJSONError.invalid }
      let key = try parseString()
      guard object[key] == nil else { throw StrictJSONError.invalid }
      skipWhitespace()
      guard consumeIf(0x3A) else { throw StrictJSONError.invalid }
      skipWhitespace()
      object[key] = try parseValue(depth: depth + 1)
      skipWhitespace()
      if consumeIf(0x7D) { return .object(object) }
      guard consumeIf(0x2C) else { throw StrictJSONError.invalid }
      skipWhitespace()
    }
  }

  private mutating func parseArray(depth: Int) throws -> StrictJSONValue {
    index += 1
    skipWhitespace()
    var values: [StrictJSONValue] = []
    if consumeIf(0x5D) { return .array(values) }
    while true {
      values.append(try parseValue(depth: depth + 1))
      skipWhitespace()
      if consumeIf(0x5D) { return .array(values) }
      guard consumeIf(0x2C) else { throw StrictJSONError.invalid }
      skipWhitespace()
    }
  }

  private mutating func parseString() throws -> String {
    let start = index
    index += 1
    var escaped = false
    while index < bytes.count {
      let byte = bytes[index]
      if escaped {
        escaped = false
        if byte == 0x75 {
          guard index + 4 < bytes.count,
            bytes[(index + 1)...(index + 4)].allSatisfy({ Self.isHex($0) })
          else {
            throw StrictJSONError.invalid
          }
          index += 5
          continue
        }
        guard [0x22, 0x5C, 0x2F, 0x62, 0x66, 0x6E, 0x72, 0x74].contains(byte) else {
          throw StrictJSONError.invalid
        }
        index += 1
        continue
      }
      if byte == 0x5C {
        escaped = true
        index += 1
        continue
      }
      if byte == 0x22 {
        index += 1
        let token = Data(bytes[start..<index])
        guard let decoded = try? JSONDecoder().decode(String.self, from: token) else {
          throw StrictJSONError.invalid
        }
        return decoded
      }
      guard byte >= 0x20 else { throw StrictJSONError.invalid }
      index += 1
    }
    throw StrictJSONError.invalid
  }

  private mutating func parseNumber() throws -> String {
    let start = index
    if consumeIf(0x2D), index == bytes.count { throw StrictJSONError.invalid }
    guard index < bytes.count else { throw StrictJSONError.invalid }
    if consumeIf(0x30) {
      if index < bytes.count, bytes[index] >= 0x30, bytes[index] <= 0x39 {
        throw StrictJSONError.invalid
      }
    } else {
      guard bytes[index] >= 0x31, bytes[index] <= 0x39 else { throw StrictJSONError.invalid }
      while index < bytes.count, bytes[index] >= 0x30, bytes[index] <= 0x39 { index += 1 }
    }
    if consumeIf(0x2E) {
      guard index < bytes.count, bytes[index] >= 0x30, bytes[index] <= 0x39 else {
        throw StrictJSONError.invalid
      }
      while index < bytes.count, bytes[index] >= 0x30, bytes[index] <= 0x39 { index += 1 }
    }
    if index < bytes.count, bytes[index] == 0x65 || bytes[index] == 0x45 {
      index += 1
      if index < bytes.count, bytes[index] == 0x2B || bytes[index] == 0x2D { index += 1 }
      guard index < bytes.count, bytes[index] >= 0x30, bytes[index] <= 0x39 else {
        throw StrictJSONError.invalid
      }
      while index < bytes.count, bytes[index] >= 0x30, bytes[index] <= 0x39 { index += 1 }
    }
    return String(decoding: bytes[start..<index], as: UTF8.self)
  }

  private mutating func consume(_ text: StaticString) throws {
    let expected = Array("\(text)".utf8)
    guard index + expected.count <= bytes.count,
      Array(bytes[index..<(index + expected.count)]) == expected
    else {
      throw StrictJSONError.invalid
    }
    index += expected.count
  }

  private mutating func consumeIf(_ byte: UInt8) -> Bool {
    guard index < bytes.count, bytes[index] == byte else { return false }
    index += 1
    return true
  }

  private mutating func skipWhitespace() {
    while index < bytes.count, [0x20, 0x09, 0x0A, 0x0D].contains(bytes[index]) { index += 1 }
  }

  private static func isHex(_ byte: UInt8) -> Bool {
    (byte >= 0x30 && byte <= 0x39) || (byte >= 0x41 && byte <= 0x46)
      || (byte >= 0x61 && byte <= 0x66)
  }
}

extension Dictionary where Key == String, Value == StrictJSONValue {
  func exactKeys(_ required: Set<String>, optional: Set<String> = []) throws {
    guard required.isSubset(of: Set(keys)), Set(keys).isSubset(of: required.union(optional)) else {
      throw OnboardingClientError.invalidResponse
    }
  }
}
