package main

import "fmt"

type rule struct {
	reason int
	name   string
	hr     int32
	class  bool
}

func bits(v uint32) int32 { return int32(v) }

var rules = []rule{
	{0, "user_stop", 0, false},
	{1, "permission_revoke", bits(0x80070005), false},
	{2, "device_lost", bits(0x88890004), false},
	{3, "shutdown", 0, false},
	{4, "suspend", 0, false},
	{5, "lock", 0, false},
	{6, "cancel", bits(0x800704C7), false},
	{7, "overflow", bits(0x8007006F), false},
	{8, "wasapi_error", bits(0x80004005), true},
	{9, "format_error", bits(0x80070057), false},
	{10, "discontinuity", bits(0x8007000D), false},
}

func valid(reason int, name string, hr int32) bool {
	if reason < 0 || reason >= len(rules) {
		return false
	}
	r := rules[reason]
	if r.reason != reason || r.name != name {
		return false
	}
	if !r.class {
		return hr == r.hr
	}
	if hr >= 0 {
		return false
	}
	for _, reason := range []int{1, 2, 6, 7, 10} {
		if hr == rules[reason].hr {
			return false
		}
	}
	return true
}

func require(name string, ok bool) {
	if !ok {
		panic("FAIL: " + name)
	}
	fmt.Println("PASS:", name)
}

func main() {
	for _, r := range rules {
		require(fmt.Sprintf("valid row %d/%s", r.reason, r.name), valid(r.reason, r.name, r.hr))
		require(fmt.Sprintf("name mismatch rejected for %d", r.reason), !valid(r.reason, r.name+"_bogus", r.hr))
		if !r.class {
			for _, other := range rules {
				if other.class || other.hr == r.hr {
					continue
				}
				require(fmt.Sprintf("row %d rejects exact code from row %d", r.reason, other.reason),
					!valid(r.reason, r.name, other.hr))
			}
			require(fmt.Sprintf("row %d rejects arbitrary E_FAIL", r.reason),
				!valid(r.reason, r.name, bits(0x80004005)))
		}
	}
	require("cancel zero rejected", !valid(6, "cancel", 0))
	require("cancel ERROR_CANCELLED accepted", valid(6, "cancel", bits(0x800704C7)))
	require("WASAPI S_OK rejected", !valid(8, "wasapi_error", 0))
	require("WASAPI cannot steal permission HRESULT", !valid(8, "wasapi_error", bits(0x80070005)))
	require("WASAPI cannot steal device-lost HRESULT", !valid(8, "wasapi_error", bits(0x88890004)))
	require("WASAPI cannot steal cancel HRESULT", !valid(8, "wasapi_error", bits(0x800704C7)))
	require("WASAPI cannot steal overflow HRESULT", !valid(8, "wasapi_error", bits(0x8007006F)))
	require("WASAPI cannot steal discontinuity HRESULT", !valid(8, "wasapi_error", bits(0x8007000D)))
	require("WASAPI arbitrary failed HRESULT accepted", valid(8, "wasapi_error", bits(0x88890010)))
	require("WASAPI E_INVALIDARG accepted for non-format call context", valid(8, "wasapi_error", bits(0x80070057)))
	require("unknown reason rejected", !valid(11, "unknown", bits(0x80004005)))
}
