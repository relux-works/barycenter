// Pairing flow (design §4, mirror of Credentials.swift/pairNode):
// POST {coordinator}/pair {"code": "..."} exchanges a bot-issued one-time
// code for node credentials. The caller persists them via Credentials.Save.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// PairError mirrors the macOS PairingError messages so both shells speak
// the same language to the same users.
type PairError struct {
	Status int
}

func (e *PairError) Error() string {
	switch e.Status {
	case http.StatusForbidden:
		return "код не подошёл или истёк — попроси новый: /pair у бота"
	case http.StatusConflict:
		return "в орбите нет свободных мест"
	default:
		return fmt.Sprintf("сервер ответил %d", e.Status)
	}
}

func (e *PairError) String() string   { return e.Error() }
func (e *PairError) GoString() string { return fmt.Sprintf("PairError{status:%d}", e.Status) }

// Pair exchanges a one-time code for credentials.
// coordinatorBase is the https base URL, e.g. https://barycenter.relux.works.
func Pair(coordinatorBase, code string) (Credentials, error) {
	return pairWithDoer(coordinatorBase, code, nil)
}

func pairWithDoer(coordinatorBase, code string, doer HTTPDoer) (Credentials, error) {
	client, err := NewOnboardingClient(coordinatorBase, doer)
	if err != nil {
		return Credentials{}, fmt.Errorf("неверный адрес координатора")
	}
	creds, err := client.pairLegacy(context.Background(), code)
	if err == nil {
		return creds, nil
	}
	var clientErr *OnboardingClientError
	if errors.As(err, &clientErr) {
		if clientErr.Status != 0 {
			return Credentials{}, &PairError{Status: clientErr.Status}
		}
		if clientErr.Kind == ClientErrorTransport || clientErr.Kind == ClientErrorCancelled {
			return Credentials{}, fmt.Errorf("не достучался до координатора")
		}
	}
	return Credentials{}, fmt.Errorf("непонятный ответ координатора")
}
