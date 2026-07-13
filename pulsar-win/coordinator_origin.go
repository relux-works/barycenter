package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

var coordinatorIDNAProfile = idna.New(
	idna.MapForLookup(),
	idna.Transitional(false),
	idna.StrictDomainName(true),
	idna.BidiRule(),
	idna.CheckJoiners(true),
	idna.VerifyDNSLength(true),
)

// CoordinatorOrigin is the canonical, path-free authority used to bind
// credentials and recovery state to one coordinator.
type CoordinatorOrigin struct{ value string }

func (o CoordinatorOrigin) String() string   { return o.value }
func (o CoordinatorOrigin) GoString() string { return fmt.Sprintf("CoordinatorOrigin(%q)", o.value) }

func CanonicalCoordinatorOrigin(raw string) (CoordinatorOrigin, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || !utf8.ValidString(raw) || strings.Contains(raw, "\\") {
		return CoordinatorOrigin{}, errInvalidCoordinatorOrigin
	}
	separator := strings.Index(raw, "://")
	if separator <= 0 {
		return CoordinatorOrigin{}, errInvalidCoordinatorOrigin
	}
	authorityEnd := len(raw)
	if i := strings.IndexAny(raw[separator+3:], "/?#"); i >= 0 {
		authorityEnd = separator + 3 + i
	}
	authority := raw[separator+3 : authorityEnd]
	if authority == "" || strings.Contains(authority, "%") {
		return CoordinatorOrigin{}, errInvalidCoordinatorOrigin
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Host == "" || u.User != nil {
		return CoordinatorOrigin{}, errInvalidCoordinatorOrigin
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return CoordinatorOrigin{}, errInvalidCoordinatorOrigin
	}
	host, port, err := canonicalHostPort(u)
	if err != nil {
		return CoordinatorOrigin{}, errInvalidCoordinatorOrigin
	}
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	value := scheme + "://" + host
	if port != "" {
		value += ":" + port
	}
	return CoordinatorOrigin{value: value}, nil
}

func canonicalHostPort(u *url.URL) (string, string, error) {
	hostname := u.Hostname()
	if hostname == "" || strings.Contains(hostname, "%") {
		return "", "", errInvalidCoordinatorOrigin
	}
	port := u.Port()
	rawHost := u.Host
	if strings.HasSuffix(rawHost, ":") || strings.ContainsAny(port, "+-") {
		return "", "", errInvalidCoordinatorOrigin
	}
	if port != "" {
		for _, r := range port {
			if r < '0' || r > '9' {
				return "", "", errInvalidCoordinatorOrigin
			}
		}
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return "", "", errInvalidCoordinatorOrigin
		}
		port = strconv.FormatUint(value, 10)
	}
	if strings.Contains(hostname, ":") {
		ip := net.ParseIP(hostname)
		if ip == nil || ip.To4() != nil {
			return "", "", errInvalidCoordinatorOrigin
		}
		return "[" + ip.String() + "]", port, nil
	}
	ascii, err := coordinatorIDNAProfile.ToASCII(hostname)
	if err != nil || ascii == "" {
		return "", "", errInvalidCoordinatorOrigin
	}
	ascii = strings.ToLower(ascii)
	ascii = strings.TrimSuffix(ascii, ".")
	if ascii == "" || strings.HasSuffix(ascii, ".") {
		return "", "", errInvalidCoordinatorOrigin
	}
	if ip := net.ParseIP(ascii); ip != nil {
		v4 := ip.To4()
		if v4 == nil {
			return "", "", errInvalidCoordinatorOrigin
		}
		return v4.String(), port, nil
	}
	if looksLikeAmbiguousIPv4(ascii) {
		return "", "", errInvalidCoordinatorOrigin
	}
	if strings.ContainsAny(ascii, "[]:") {
		return "", "", errInvalidCoordinatorOrigin
	}
	return ascii, port, nil
}

func looksLikeAmbiguousIPv4(host string) bool {
	lower := strings.ToLower(host)
	if strings.HasPrefix(lower, "0x") {
		if _, err := strconv.ParseUint(lower[2:], 16, 64); err == nil {
			return true
		}
	}
	allDigits := true
	for _, r := range lower {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return true
	}
	parts := strings.Split(lower, ".")
	if len(parts) <= 1 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return true
		}
		base := 10
		value := part
		if strings.HasPrefix(value, "0x") {
			base, value = 16, value[2:]
		}
		if value == "" {
			return false
		}
		if _, err := strconv.ParseUint(value, base, 64); err != nil {
			return false
		}
	}
	return true
}

func (o CoordinatorOrigin) URL(path string) (*url.URL, error) {
	if o.value == "" || path == "" || path[0] != '/' || strings.ContainsAny(path, "?#") {
		return nil, errInvalidCoordinatorOrigin
	}
	u, err := url.Parse(o.value)
	if err != nil {
		return nil, errInvalidCoordinatorOrigin
	}
	u.Path = path
	return u, nil
}

func (o CoordinatorOrigin) permitsSecrets() bool {
	u, err := url.Parse(o.value)
	if err != nil {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "::1"
}

func (o CoordinatorOrigin) WebSocketURL() (string, error) {
	u, err := url.Parse(o.value)
	if err != nil {
		return "", errInvalidCoordinatorOrigin
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else if u.Scheme == "http" && o.permitsSecrets() {
		u.Scheme = "ws"
	} else {
		return "", errInvalidCoordinatorOrigin
	}
	u.Path = "/ws"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

var errInvalidCoordinatorOrigin = errors.New("coordinator origin is invalid")
