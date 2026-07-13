package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const (
	maximumHTTPResponseBytes = 64 << 10
	maximumJSONDepth         = 32
)

func parseStrictJSONObject(raw []byte) (map[string]any, error) {
	if !validJSONUnicode(raw) {
		return nil, errInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := parseStrictJSONValue(decoder, 0)
	if err != nil {
		return nil, errInvalidResponse
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errInvalidResponse
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errInvalidResponse
	}
	return object, nil
}

func validJSONUnicode(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	inString := false
	for index := 0; index < len(raw); index++ {
		switch raw[index] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || index+1 >= len(raw) {
				continue
			}
			if raw[index+1] != 'u' {
				index++
				continue
			}
			value, ok := jsonHexQuad(raw, index+2)
			if !ok {
				return false
			}
			switch {
			case value >= 0xd800 && value <= 0xdbff:
				if index+11 >= len(raw) || raw[index+6] != '\\' || raw[index+7] != 'u' {
					return false
				}
				low, ok := jsonHexQuad(raw, index+8)
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				index += 11
			case value >= 0xdc00 && value <= 0xdfff:
				return false
			default:
				index += 5
			}
		}
	}
	return !inString
}

func jsonHexQuad(raw []byte, start int) (uint16, bool) {
	if start < 0 || start+4 > len(raw) {
		return 0, false
	}
	var value uint16
	for _, character := range raw[start : start+4] {
		value <<= 4
		switch {
		case character >= '0' && character <= '9':
			value |= uint16(character - '0')
		case character >= 'a' && character <= 'f':
			value |= uint16(character-'a') + 10
		case character >= 'A' && character <= 'F':
			value |= uint16(character-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func parseStrictJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > maximumJSONDepth {
		return nil, errInvalidResponse
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		switch token.(type) {
		case nil, bool, string, json.Number:
			return token, nil
		default:
			return nil, errInvalidResponse
		}
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errInvalidResponse
			}
			if _, exists := object[key]; exists {
				return nil, errInvalidResponse
			}
			value, err := parseStrictJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return nil, errInvalidResponse
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := parseStrictJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return nil, errInvalidResponse
		}
		return array, nil
	default:
		return nil, errInvalidResponse
	}
}

func exactObjectKeys(object map[string]any, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return false
		}
	}
	return true
}

func jsonString(object map[string]any, key string) (string, bool) {
	value, ok := object[key].(string)
	return value, ok
}

func jsonBool(object map[string]any, key string) (bool, bool) {
	value, ok := object[key].(bool)
	return value, ok
}

func jsonInt64(object map[string]any, key string) (int64, bool) {
	value, ok := object[key].(json.Number)
	if !ok {
		return 0, false
	}
	integer, err := value.Int64()
	return integer, err == nil
}

func readBoundedResponse(reader io.Reader) ([]byte, error) {
	limited := io.LimitReader(reader, maximumHTTPResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		zeroBytes(raw)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errInvalidResponse
	}
	if len(raw) > maximumHTTPResponseBytes {
		zeroBytes(raw)
		return nil, errInvalidResponse
	}
	return raw, nil
}
