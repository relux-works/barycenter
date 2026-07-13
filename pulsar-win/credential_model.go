package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

const credentialBundleVersion = 1

var (
	lowerHexTokenPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	recoveryIDPattern    = regexp.MustCompile(`^rec_[0-9a-f]{32}$`)
	humanSecretPattern   = regexp.MustCompile(`^[ABCDEFGHJKMNPQRSTVWXYZ2-9]{27}$`)
)

// NodeCredential is the node/playback capability stored in the protected
// bundle. It is intentionally independent from control authority.
type NodeCredential struct {
	OrbitID   int64  `json:"orbit_id"`
	Slot      string `json:"slot"`
	NodeToken string `json:"node_token"`
	WSURL     string `json:"ws_url"`
}

func (c NodeCredential) String() string {
	return fmt.Sprintf("NodeCredential{orbit:%d credentials:<redacted>}", c.OrbitID)
}

func (c NodeCredential) GoString() string { return c.String() }

// ControlContextState distinguishes a fully resolved control capability from
// a valid token whose membership/orbit context is temporarily unavailable.
type ControlContextState string

const (
	ControlContextActive  ControlContextState = "active"
	ControlContextLimited ControlContextState = "limited"
)

// ControlCredential is an independently replaceable control capability.
// OrbitID and Role are absent only for the frozen probe-403 limited state.
type ControlCredential struct {
	ActorID          int64               `json:"actor_id"`
	OrbitID          int64               `json:"orbit_id,omitempty"`
	Role             string              `json:"role,omitempty"`
	LastKnownOrbitID int64               `json:"last_known_orbit_id,omitempty"`
	LastKnownRole    string              `json:"last_known_role,omitempty"`
	ControlToken     string              `json:"control_token"`
	Context          ControlContextState `json:"context"`
}

func (c ControlCredential) String() string {
	return fmt.Sprintf("ControlCredential{actor:%d orbit:%d credentials:<redacted>}", c.ActorID, c.OrbitID)
}

func (c ControlCredential) GoString() string { return c.String() }

// CredentialBundle is the single versioned protected record. Node and control
// capabilities are independently optional; recovery metadata is non-secret.
type CredentialBundle struct {
	Version                    int                `json:"version"`
	Node                       *NodeCredential    `json:"node,omitempty"`
	Control                    *ControlCredential `json:"control,omitempty"`
	RecoveryID                 string             `json:"recovery_id,omitempty"`
	CoordinatorOrigin          string             `json:"coordinator_origin,omitempty"`
	RecoveryConsumed           bool               `json:"recovery_consumed,omitempty"`
	RecoveryBackupAcknowledged bool               `json:"recovery_backup_acknowledged,omitempty"`
}

func (b CredentialBundle) String() string {
	return fmt.Sprintf("CredentialBundle{version:%d node:%t control:%t recovery:<redacted> origin:<redacted>}", b.Version, b.Node != nil, b.Control != nil)
}

func (b CredentialBundle) GoString() string { return b.String() }

func (b CredentialBundle) validate() error {
	if b.Version != credentialBundleVersion || (b.Node == nil && b.Control == nil) || b.CoordinatorOrigin == "" {
		return errCredentialCorrupt
	}
	if b.Node != nil {
		if err := validateNodeCredential(*b.Node); err != nil {
			return errCredentialCorrupt
		}
	}
	if b.Control != nil {
		if err := validateControlCredential(*b.Control); err != nil {
			return errCredentialCorrupt
		}
	}
	if b.RecoveryID != "" && !recoveryIDPattern.MatchString(b.RecoveryID) {
		return errCredentialCorrupt
	}
	if b.RecoveryID != "" && b.Control == nil {
		return errCredentialCorrupt
	}
	if b.RecoveryBackupAcknowledged && b.RecoveryID == "" {
		return errCredentialCorrupt
	}
	if b.RecoveryConsumed && (b.RecoveryID == "" || b.Control == nil) {
		return errCredentialCorrupt
	}
	if b.CoordinatorOrigin != "" {
		origin, err := CanonicalCoordinatorOrigin(b.CoordinatorOrigin)
		if err != nil || origin.String() != b.CoordinatorOrigin || !origin.permitsSecrets() {
			return errCredentialCorrupt
		}
		if b.Node != nil {
			nodeOrigin, err := canonicalCoordinatorOriginFromWSURL(b.Node.WSURL)
			if err != nil || nodeOrigin != origin {
				return errCredentialCorrupt
			}
		}
	}
	if b.Node != nil && b.Control != nil {
		switch b.Control.Context {
		case ControlContextActive:
			if b.Node.OrbitID != b.Control.OrbitID {
				return errCredentialCorrupt
			}
		case ControlContextLimited:
			if b.Control.LastKnownOrbitID != 0 && b.Node.OrbitID != b.Control.LastKnownOrbitID {
				return errCredentialCorrupt
			}
		}
	}
	return nil
}

func validateNodeCredential(c NodeCredential) error {
	if c.OrbitID <= 0 || len(c.Slot) != 1 || c.Slot[0] < 'a' || c.Slot[0] > 'z' || !lowerHexTokenPattern.MatchString(c.NodeToken) {
		return errCredentialCorrupt
	}
	if err := validateLegacyWSURL(c.WSURL); err != nil {
		return errCredentialCorrupt
	}
	return nil
}

func validateControlCredential(c ControlCredential) error {
	if c.ActorID <= 0 || !lowerHexTokenPattern.MatchString(c.ControlToken) {
		return errCredentialCorrupt
	}
	switch c.Context {
	case ControlContextActive:
		if c.OrbitID <= 0 || !validRole(c.Role) || c.LastKnownOrbitID != 0 || c.LastKnownRole != "" {
			return errCredentialCorrupt
		}
	case ControlContextLimited:
		if c.OrbitID != 0 || c.Role != "" {
			return errCredentialCorrupt
		}
		if (c.LastKnownOrbitID == 0) != (c.LastKnownRole == "") || (c.LastKnownOrbitID != 0 && !validRole(c.LastKnownRole)) {
			return errCredentialCorrupt
		}
	default:
		return errCredentialCorrupt
	}
	return nil
}

func validRole(role string) bool {
	return role == "primary" || role == "companion" || role == "satellite"
}

func validateLegacyWSURL(raw string) error {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.Contains(raw, "\\") {
		return errCredentialCorrupt
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Host == "" || u.User != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.RawQuery != "" || u.Fragment != "" {
		return errCredentialCorrupt
	}
	if u.Path != "/ws" || u.RawPath != "" {
		return errCredentialCorrupt
	}
	host, _, err := canonicalHostPort(u)
	if err != nil {
		return errCredentialCorrupt
	}
	if u.Scheme == "ws" {
		if host != "127.0.0.1" && host != "[::1]" {
			return errCredentialCorrupt
		}
	}
	return nil
}

func canonicalCoordinatorOriginFromWSURL(raw string) (CoordinatorOrigin, error) {
	if err := validateLegacyWSURL(raw); err != nil {
		return CoordinatorOrigin{}, errInvalidCoordinatorOrigin
	}
	u, err := url.Parse(raw)
	if err != nil {
		return CoordinatorOrigin{}, errInvalidCoordinatorOrigin
	}
	switch u.Scheme {
	case "wss":
		u.Scheme = "https"
	case "ws":
		u.Scheme = "http"
	default:
		return CoordinatorOrigin{}, errInvalidCoordinatorOrigin
	}
	u.Path, u.RawPath, u.RawQuery, u.Fragment = "", "", "", ""
	return CanonicalCoordinatorOrigin(u.String())
}

func nodeFromCredentials(c Credentials) NodeCredential {
	return NodeCredential{OrbitID: c.OrbitID, Slot: c.Slot, NodeToken: c.Token, WSURL: c.WSURL}
}

func credentialsFromNode(c NodeCredential) Credentials {
	return Credentials{OrbitID: c.OrbitID, Slot: c.Slot, Token: c.NodeToken, WSURL: c.WSURL}
}

// NodeCapability and ControlCapability are origin-bound bearer views. Their
// bearer accessors are deliberately package-private so callers cannot pass an
// arbitrary raw token to authenticated client operations.
type NodeCapability struct {
	origin CoordinatorOrigin
	value  NodeCredential
}

func (c NodeCapability) String() string                           { return "NodeCapability{<redacted>}" }
func (c NodeCapability) GoString() string                         { return c.String() }
func (c NodeCapability) actorBearer() (CoordinatorOrigin, string) { return c.origin, c.value.NodeToken }

type ControlCapability struct {
	origin CoordinatorOrigin
	value  ControlCredential
}

func (c ControlCapability) String() string   { return "ControlCapability{<redacted>}" }
func (c ControlCapability) GoString() string { return c.String() }
func (c ControlCapability) actorBearer() (CoordinatorOrigin, string) {
	return c.origin, c.value.ControlToken
}

type ActorCapability interface {
	actorBearer() (CoordinatorOrigin, string)
}

func (b CredentialBundle) NodeCapability() (NodeCapability, bool) {
	if b.Node == nil || b.validate() != nil {
		return NodeCapability{}, false
	}
	origin, err := CanonicalCoordinatorOrigin(b.CoordinatorOrigin)
	if err != nil {
		return NodeCapability{}, false
	}
	return NodeCapability{origin: origin, value: *b.Node}, true
}

func (b CredentialBundle) ControlCapability() (ControlCapability, bool) {
	if b.Control == nil || b.validate() != nil {
		return ControlCapability{}, false
	}
	origin, err := CanonicalCoordinatorOrigin(b.CoordinatorOrigin)
	if err != nil {
		return ControlCapability{}, false
	}
	return ControlCapability{origin: origin, value: *b.Control}, true
}

// RecoveryMaterial holds the one-time secret only in memory. It deliberately
// implements no JSON or text marshaling interface.
type RecoveryMaterial struct {
	mu                 sync.Mutex
	actorID            int64
	recoveryID         string
	secret             []byte
	discarded          bool
	revealedForDisplay bool
}

func newRecoveryMaterial(actorID int64, recoveryID, secret string) (*RecoveryMaterial, error) {
	if actorID <= 0 || !recoveryIDPattern.MatchString(recoveryID) || !humanSecretPattern.MatchString(secret) {
		return nil, errInvalidResponse
	}
	return &RecoveryMaterial{actorID: actorID, recoveryID: recoveryID, secret: []byte(secret)}, nil
}

func (m *RecoveryMaterial) String() string   { return "RecoveryMaterial{<redacted>}" }
func (m *RecoveryMaterial) GoString() string { return m.String() }

// RevealForDisplay is an explicit UI action. Display and copy do not imply a
// successful backup acknowledgement.
func (m *RecoveryMaterial) RevealForDisplay() (actorID int64, recoveryID, recoverySecret string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.discarded || len(m.secret) == 0 {
		return 0, "", "", false
	}
	m.revealedForDisplay = true
	return m.actorID, m.recoveryID, string(m.secret), true
}

func (m *RecoveryMaterial) metadata() (actorID int64, recoveryID string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.discarded || len(m.secret) == 0 {
		return 0, "", false
	}
	return m.actorID, m.recoveryID, true
}

func (m *RecoveryMaterial) discard() {
	m.mu.Lock()
	defer m.mu.Unlock()
	zeroBytes(m.secret)
	m.secret = nil
	m.discarded = true
}

func (m *RecoveryMaterial) exportJSON() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.discarded || len(m.secret) == 0 {
		return nil, errOneTimeMaterialGone
	}
	type recoveryExport struct {
		ActorID        int64  `json:"actor_id"`
		RecoveryID     string `json:"recovery_id"`
		RecoverySecret string `json:"recovery_secret"`
	}
	return json.Marshal(recoveryExport{ActorID: m.actorID, RecoveryID: m.recoveryID, RecoverySecret: string(m.secret)})
}

// RecoveryInput keeps a user-entered recovery secret memory-only and redacted.
type RecoveryInput struct {
	state *recoveryInputState
}

type recoveryInputState struct {
	mu         sync.Mutex
	actorID    int64
	recoveryID string
	secret     []byte
	consumed   bool
}

func NewRecoveryInput(actorID int64, recoveryID, recoverySecret string) (RecoveryInput, error) {
	canonical, ok := normalizeHumanSecret(recoverySecret)
	if !ok || actorID <= 0 || !recoveryIDPattern.MatchString(recoveryID) || !humanSecretPattern.MatchString(canonical) {
		return RecoveryInput{}, errInvalidRequest
	}
	return RecoveryInput{state: &recoveryInputState{actorID: actorID, recoveryID: recoveryID, secret: []byte(canonical)}}, nil
}

func (i RecoveryInput) String() string   { return "RecoveryInput{<redacted>}" }
func (i RecoveryInput) GoString() string { return i.String() }

func (i RecoveryInput) take() (actorID int64, recoveryID string, secret []byte, err error) {
	if i.state == nil {
		return 0, "", nil, errInvalidRequest
	}
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.consumed || len(i.state.secret) == 0 {
		return 0, "", nil, errInvalidRequest
	}
	i.state.consumed = true
	secret = i.state.secret
	i.state.secret = nil
	return i.state.actorID, i.state.recoveryID, secret, nil
}

func normalizeHumanSecret(value string) (string, bool) {
	if len(value) > 40 {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(value))
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= utf8.RuneSelf {
			return "", false
		}
		switch character {
		case '-', ' ', '\t', '\r', '\n', '\v', '\f':
		default:
			if character >= 'a' && character <= 'z' {
				character -= 'a' - 'A'
			}
			b.WriteByte(character)
		}
	}
	return b.String(), true
}

func zeroBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

var (
	errCredentialCorrupt   = errors.New("credential storage contains invalid data")
	errInvalidRequest      = errors.New("request is invalid")
	errInvalidResponse     = errors.New("coordinator response is invalid")
	errOneTimeMaterialGone = errors.New("one-time recovery material is no longer available")
)
