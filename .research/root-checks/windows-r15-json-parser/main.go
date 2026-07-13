package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// This is the Rev 15 note's rejectDuplicateKeys implementation, copied
// verbatim apart from gofmt. The harness below tests the cases required by the
// root continuation review independently of the agent's checker.
func rejectDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var walk func(seenStack []map[string]bool) error
	walk = func(stack []map[string]bool) error {
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			switch t := tok.(type) {
			case json.Delim:
				if t == '{' {
					seen := map[string]bool{}
					for dec.More() {
						kt, err := dec.Token()
						if err != nil {
							return err
						}
						key := kt.(string)
						if seen[key] {
							return fmt.Errorf("duplicate key %q", key)
						}
						seen[key] = true
						if err := walk(nil); err != nil {
							return err
						}
					}
					if _, err := dec.Token(); err != nil {
						return err
					}
					return nil
				}
				if t == '[' {
					for dec.More() {
						if err := walk(nil); err != nil {
							return err
						}
					}
					if _, err := dec.Token(); err != nil {
						return err
					}
					return nil
				}
			default:
				return nil
			}
		}
	}
	if err := walk(nil); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("trailing content")
	}
	return nil
}

func main() {
	tests := []struct {
		name       string
		json       string
		wantReject bool
	}{
		{name: "valid object", json: `{"reason":0,"nested":{"x":1},"items":[{"y":2}]}`},
		{name: "valid whitespace EOF", json: "  {\n\t\"reason\": 0\n} \r\n"},
		{name: "duplicate top level first order", json: `{"reason":0,"reason":1}`, wantReject: true},
		{name: "duplicate top level reverse order", json: `{"reason":1,"reason":0}`, wantReject: true},
		{name: "duplicate nested object", json: `{"outer":{"x":1,"x":2}}`, wantReject: true},
		{name: "duplicate object in array", json: `{"items":[{"x":1,"x":2}]}`, wantReject: true},
		{name: "two concatenated objects", json: `{"x":1}{"y":2}`, wantReject: true},
		{name: "trailing non whitespace", json: `{"x":1}x`, wantReject: true},
	}

	failed := false
	for _, tc := range tests {
		err := rejectDuplicateKeys([]byte(tc.json))
		gotReject := err != nil
		status := "PASS"
		if gotReject != tc.wantReject {
			status = "FAIL"
			failed = true
		}
		fmt.Printf("%s: %s (err=%v)\n", status, tc.name, err)
	}
	if failed {
		os.Exit(1)
	}
}
