package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const (
	maximumHTTPResponseBytes = 64 << 10
	maximumJSONDepth         = 32
)

func parseStrictJSONObject(raw []byte) (map[string]any, error) {
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
	if err != nil || len(raw) > maximumHTTPResponseBytes {
		zeroBytes(raw)
		return nil, errInvalidResponse
	}
	return raw, nil
}
