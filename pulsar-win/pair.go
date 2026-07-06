// Pairing flow (design §4, mirror of Credentials.swift/pairNode):
// POST {coordinator}/pair {"code": "..."} exchanges a bot-issued one-time
// code for node credentials. The caller persists them via Credentials.Save.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PairError mirrors the macOS PairingError messages so both shells speak
// the same language to the same users.
type PairError struct {
	Status int
	Body   string
}

func (e *PairError) Error() string {
	switch e.Status {
	case http.StatusForbidden:
		return "код не подошёл или истёк — попроси новый: /pair у бота"
	case http.StatusConflict:
		return "в орбите нет свободных мест"
	default:
		return fmt.Sprintf("сервер ответил %d: %s", e.Status, e.Body)
	}
}

// Pair exchanges a one-time code for credentials.
// coordinatorBase is the https base URL, e.g. https://barycenter.relux.works.
func Pair(coordinatorBase, code string) (Credentials, error) {
	base := strings.TrimSuffix(coordinatorBase, "/")
	body, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return Credentials{}, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(base+"/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		return Credentials{}, fmt.Errorf("не достучался до координатора: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return Credentials{}, fmt.Errorf("не достучался до координатора: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Credentials{}, &PairError{Status: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	var creds Credentials
	if err := json.Unmarshal(raw, &creds); err != nil {
		return Credentials{}, fmt.Errorf("непонятный ответ координатора: %w", err)
	}
	if err := ValidateCredentials(creds); err != nil {
		return Credentials{}, fmt.Errorf("непонятный ответ координатора: %w", err)
	}
	return creds, nil
}
