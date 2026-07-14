package bot

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func newCallbackRegistryForTest(t *testing.T) (*CallbackRegistry, time.Time) {
	t.Helper()
	registry, err := NewCallbackRegistry(bytes.Repeat([]byte{0x5a}, 32))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return now }
	return registry, now
}

func callbackBindingForTest() CallbackBinding {
	return CallbackBinding{
		Initiator:     CallbackActor{ActorID: 701, OrbitID: 801, Role: "companion"},
		Authorization: CallbackInitiatorOnly,
		ChatID:        -100500, MessageID: 44, OriginalUpdateID: 12,
		MediaID: "med_01HZZZZZZZZZZZZZZZZZZZZZZZ", MediaGeneration: 3,
		Action: CallbackChooseAfterCurrent, Delivery: "after_current", Audience: "current_air",
	}
}

func callbackRequestForTest(token, query string, now time.Time) CallbackRequest {
	return CallbackRequest{
		QueryID: query, Data: token,
		Actor:  CallbackActor{ActorID: 701, OrbitID: 801, Role: "companion"},
		ChatID: -100500, MessageID: 44, Now: now,
	}
}

func TestCallbackTokenIs36ByteOpaqueReferenceStoredOnlyByKeyedHash(t *testing.T) {
	registry, _ := newCallbackRegistryForTest(t)
	token, err := registry.Mint(callbackBindingForTest())
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 36 || !strings.HasPrefix(token, "tg1_") || !validCallbackToken(token) {
		t.Fatalf("token has invalid wire shape: %q (%d bytes)", token, len(token))
	}
	if len(registry.tokens) != 1 {
		t.Fatalf("stored tokens=%d", len(registry.tokens))
	}
	for digest := range registry.tokens {
		if bytes.Contains(digest[:], []byte(token)) {
			t.Fatal("registry key contains raw callback token")
		}
	}
	if strings.Contains(token, "med_") || strings.Contains(token, "after_current") ||
		strings.Contains(token, "current_air") {
		t.Fatalf("callback leaks server-side fields: %q", token)
	}
}

func TestCallbackRegistryRejectsForgedExpiredCrossActorAndCrossOrbit(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CallbackRequest, time.Time)
		want   CallbackAnswerCode
	}{
		{name: "forged", mutate: func(req *CallbackRequest, _ time.Time) {
			req.Data = "tg1_0123456789abcdefghijklmnopqrstuv"
		}, want: CallbackExpired},
		{name: "expired", mutate: func(req *CallbackRequest, now time.Time) {
			req.Now = now.Add(callbackTokenTTL)
		}, want: CallbackExpired},
		{name: "cross_actor", mutate: func(req *CallbackRequest, _ time.Time) {
			req.Actor.ActorID++
		}, want: CallbackForbidden},
		{name: "cross_orbit", mutate: func(req *CallbackRequest, _ time.Time) {
			req.Actor.OrbitID++
		}, want: CallbackForbidden},
		{name: "changed_role", mutate: func(req *CallbackRequest, _ time.Time) {
			req.Actor.Role = "primary"
		}, want: CallbackForbidden},
		{name: "wrong_message", mutate: func(req *CallbackRequest, _ time.Time) {
			req.MessageID++
		}, want: CallbackExpired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry, now := newCallbackRegistryForTest(t)
			token, err := registry.Mint(callbackBindingForTest())
			if err != nil {
				t.Fatal(err)
			}
			request := callbackRequestForTest(token, "query-"+test.name, now)
			test.mutate(&request, now)
			called := 0
			result := registry.Handle(request, func(CallbackBinding) CallbackDecision {
				called++
				return CallbackDecision{Code: CallbackApplied, Consume: true}
			})
			if result.Code != test.want || called != 0 {
				t.Fatalf("result=%+v apply calls=%d", result, called)
			}
		})
	}
}

func TestCallbackQueryReplayIsIdempotentAndConsumedTokenIsActorBound(t *testing.T) {
	registry, now := newCallbackRegistryForTest(t)
	token, err := registry.Mint(callbackBindingForTest())
	if err != nil {
		t.Fatal(err)
	}
	request := callbackRequestForTest(token, "query-one", now)
	applyCalls := 0
	apply := func(binding CallbackBinding) CallbackDecision {
		applyCalls++
		if binding.Action != CallbackChooseAfterCurrent || binding.MediaGeneration != 3 {
			t.Fatalf("binding=%+v", binding)
		}
		return CallbackDecision{Code: CallbackApplied, Consume: true, ClearKeyboard: true}
	}
	first := registry.Handle(request, apply)
	if first.Code != CallbackApplied || first.Replay || !first.ClearKeyboard || applyCalls != 1 {
		t.Fatalf("first=%+v calls=%d", first, applyCalls)
	}
	replay := registry.Handle(request, apply)
	if replay.Code != CallbackApplied || !replay.Replay || applyCalls != 1 {
		t.Fatalf("replay=%+v calls=%d", replay, applyCalls)
	}
	crossActorReplay := request
	crossActorReplay.Actor.ActorID++
	if got := registry.Handle(crossActorReplay, apply); got.Code != CallbackForbidden || applyCalls != 1 {
		t.Fatalf("cross-actor replay=%+v calls=%d", got, applyCalls)
	}

	secondQuery := request
	secondQuery.QueryID = "query-two"
	already := registry.Handle(secondQuery, apply)
	if already.Code != CallbackAlreadyApplied || !already.ClearKeyboard || applyCalls != 1 {
		t.Fatalf("already=%+v calls=%d", already, applyCalls)
	}

	otherActor := request
	otherActor.QueryID = "query-three"
	otherActor.Actor.ActorID++
	forbidden := registry.Handle(otherActor, apply)
	if forbidden.Code != CallbackForbidden || applyCalls != 1 {
		t.Fatalf("other actor=%+v calls=%d", forbidden, applyCalls)
	}
}

func TestSourcePrimaryAuthorizationRequiresCurrentSourceOrbitPrimary(t *testing.T) {
	registry, now := newCallbackRegistryForTest(t)
	binding := callbackBindingForTest()
	binding.Authorization = CallbackSourcePrimary
	token, err := registry.Mint(binding)
	if err != nil {
		t.Fatal(err)
	}
	request := callbackRequestForTest(token, "query-primary", now)
	request.Actor = CallbackActor{ActorID: 999, OrbitID: binding.Initiator.OrbitID, Role: "primary"}
	result := registry.Handle(request, func(CallbackBinding) CallbackDecision {
		return CallbackDecision{Code: CallbackApplied, Consume: true}
	})
	if result.Code != CallbackApplied {
		t.Fatalf("primary result=%+v", result)
	}

	otherRegistry, _ := newCallbackRegistryForTest(t)
	otherToken, err := otherRegistry.Mint(binding)
	if err != nil {
		t.Fatal(err)
	}
	request = callbackRequestForTest(otherToken, "query-companion", now)
	request.Actor = CallbackActor{ActorID: 998, OrbitID: binding.Initiator.OrbitID, Role: "companion"}
	result = otherRegistry.Handle(request, func(CallbackBinding) CallbackDecision {
		return CallbackDecision{Code: CallbackApplied, Consume: true}
	})
	if result.Code != CallbackForbidden {
		t.Fatalf("companion result=%+v", result)
	}
}

func TestFailedCallbackStaysRetryableWithANewQuery(t *testing.T) {
	registry, now := newCallbackRegistryForTest(t)
	token, err := registry.Mint(callbackBindingForTest())
	if err != nil {
		t.Fatal(err)
	}
	first := registry.Handle(callbackRequestForTest(token, "query-failed", now), func(CallbackBinding) CallbackDecision {
		return CallbackDecision{Code: CallbackFailed, Consume: true}
	})
	if first.Code != CallbackFailed {
		t.Fatalf("first=%+v", first)
	}
	retryRequest := callbackRequestForTest(token, "query-retry", now)
	retry := registry.Handle(retryRequest, func(CallbackBinding) CallbackDecision {
		return CallbackDecision{Code: CallbackApplied, Consume: true}
	})
	if retry.Code != CallbackApplied {
		t.Fatalf("retry=%+v", retry)
	}
}

func TestCallbackMintRejectsNonCanonicalServerOptions(t *testing.T) {
	registry, _ := newCallbackRegistryForTest(t)
	binding := callbackBindingForTest()
	binding.Audience = "orbit:801"
	if token, err := registry.Mint(binding); err == nil || token != "" {
		t.Fatalf("token=%q err=%v", token, err)
	}
}

func TestCallbackAnswerVocabularyIsFiniteAndSanitized(t *testing.T) {
	codes := []CallbackAnswerCode{
		CallbackApplied, CallbackAlreadyApplied, CallbackRequiresConfirmation,
		CallbackTooLate, CallbackExpired, CallbackForbidden, CallbackUnsupported,
		CallbackFailed,
	}
	for _, code := range codes {
		text := CallbackAnswerText(code)
		if text == "" || strings.Contains(text, "tg1_") || strings.Contains(text, "701") {
			t.Fatalf("code=%s unsafe answer=%q", code, text)
		}
	}
}
