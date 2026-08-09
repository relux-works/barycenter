package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type airScriptedDoer struct {
	mu       sync.Mutex
	requests []*http.Request
	handle   func(*http.Request, int) (*http.Response, error)
}

func (d *airScriptedDoer) Do(request *http.Request) (*http.Response, error) {
	d.mu.Lock()
	index := len(d.requests)
	d.requests = append(d.requests, request)
	d.mu.Unlock()
	return d.handle(request, index)
}

func airJSONResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

func airFixture(airID, memberID, title string, current bool, membership AirMembershipStatus, role AirRole) string {
	return fmt.Sprintf(`{"air_id":%q,"title":%q,"status":"active","revision":3,"membership_id":%q,"membership_status":%q,"membership_revision":4,"air_role":%q,"member_count":2,"active_member_count":2,"online_pulsar_count":3,"capacity":{"barycenters":8,"online_pulsars":16},"policy":{"revision":5,"invite":"owner_primary","overlay":"air_admin_primary","queue":"primary_companion","replace":"all_member_primaries"},"is_current":%t}`,
		airID, title, memberID, membership, role, current)
}

func TestAirClientReadsCoordinatorAvailability(t *testing.T) {
	doer := &airScriptedDoer{handle: func(request *http.Request, index int) (*http.Response, error) {
		if index != 0 || request.Method != http.MethodGet || request.URL.Path != "/healthz" {
			t.Fatalf("availability request %d=%s %s", index, request.Method, request.URL)
		}
		return airJSONResponse(request, http.StatusOK,
			`{"phase2":{"air_rooms_enabled":false,"air_authority_state":"airs_shadow"}}`), nil
	}}
	client, err := NewAirAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	availability, err := client.Availability(context.Background())
	if err != nil || availability.Enabled || availability.AuthorityState != "airs_shadow" {
		t.Fatalf("availability=%+v err=%v", availability, err)
	}
}

func TestAirClientUsesCommonAuthenticatedLifecycleContract(t *testing.T) {
	airID := "air_" + strings.Repeat("A", 26)
	memberID := "aim_" + strings.Repeat("B", 26)
	inviteID := "ai_" + strings.Repeat("C", 26)
	detail := airFixture(airID, memberID, "Family room", true, AirJoined, AirRoleOwner)
	doer := &airScriptedDoer{}
	doer.handle = func(request *http.Request, index int) (*http.Response, error) {
		if !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") || strings.Contains(request.URL.RawQuery, "token") {
			t.Fatalf("request %d crossed authentication boundary: %s", index, request.URL)
		}
		body, _ := io.ReadAll(request.Body)
		if index > 1 && request.Header.Get("Idempotency-Key") == "" {
			t.Fatalf("request %d omitted idempotency key", index)
		}
		switch index {
		case 0:
			if request.Method != http.MethodGet || request.URL.Path != "/v1/airs" {
				t.Fatalf("list=%s %s", request.Method, request.URL)
			}
			return airJSONResponse(request, http.StatusOK, fmt.Sprintf(`{"current_air_id":%q,"active_pointer_revision":7,"saved":[{"air_id":%q,"title":"Family room","status":"active","membership_status":"joined","air_role":"owner","member_count":2,"active_member_count":2,"online_pulsar_count":3,"capacity":{"barycenters":8,"online_pulsars":16},"policy_revision":5,"is_current":true}]}`, airID, airID)), nil
		case 1:
			if request.URL.Path != "/v1/airs/"+airID {
				t.Fatalf("detail=%s", request.URL)
			}
			return airJSONResponse(request, http.StatusOK, detail), nil
		case 2:
			if request.Method != http.MethodPost || request.URL.Path != "/v1/airs" || !strings.Contains(string(body), `"title":"Family room"`) {
				t.Fatalf("create=%s %s body=%s", request.Method, request.URL, body)
			}
			return airJSONResponse(request, http.StatusCreated, detail), nil
		case 3:
			if request.URL.Path != "/v1/airs/"+airID+"/invites" || !strings.Contains(string(body), `"air_role":"member"`) {
				t.Fatalf("invite=%s body=%s", request.URL, body)
			}
			return airJSONResponse(request, http.StatusCreated, fmt.Sprintf(`{"invite_id":%q,"revision":2,"expires_at":"2026-07-15T15:00:00Z","code":"secret-one-time-code"}`, inviteID)), nil
		case 4:
			if request.URL.Path != "/v1/airs/"+airID+"/invites/"+inviteID+"/withdraw" || !strings.Contains(string(body), `"invite_revision":2`) {
				t.Fatalf("withdraw=%s body=%s", request.URL, body)
			}
			return airJSONResponse(request, http.StatusOK, `{}`), nil
		case 5:
			if request.URL.Path != "/v1/air-invites/consume" || !strings.Contains(string(body), "secret-one-time-code") {
				t.Fatalf("consume=%s body=%s", request.URL, body)
			}
			return airJSONResponse(request, http.StatusAccepted, fmt.Sprintf(`{"air_id":%q,"title":"Family room","owner_display_name":"Ivan","air_role":"member","membership_id":%q,"membership_revision":4,"policy":{"revision":5,"invite":"owner_primary","overlay":"air_admin_primary","queue":"primary_companion","replace":"all_member_primaries"},"member_count":2,"capacity":{"barycenters":8,"online_pulsars":16},"activation_would_switch":true}`, airID, memberID)), nil
		case 6:
			if request.URL.Path != "/v1/airs/"+airID+"/join/confirm" || !strings.Contains(string(body), `"activate":true`) || !strings.Contains(string(body), `"expected_active_air_id":"none"`) {
				t.Fatalf("confirm=%s body=%s", request.URL, body)
			}
			return airJSONResponse(request, http.StatusOK, `{}`), nil
		case 7:
			if request.URL.Path != "/v1/airs/"+airID+"/join/decline" {
				t.Fatalf("decline=%s", request.URL)
			}
			return airJSONResponse(request, http.StatusOK, `{}`), nil
		case 8:
			if request.URL.Path != "/v1/airs/"+airID+"/activate" {
				t.Fatalf("activate=%s", request.URL)
			}
			return airJSONResponse(request, http.StatusOK, `{}`), nil
		case 9:
			if request.URL.Path != "/v1/airs/"+airID+"/deactivate" {
				t.Fatalf("deactivate=%s", request.URL)
			}
			return airJSONResponse(request, http.StatusOK, `{}`), nil
		case 10:
			if request.URL.Path != "/v1/airs/"+airID+"/leave" {
				t.Fatalf("leave=%s", request.URL)
			}
			return airJSONResponse(request, http.StatusOK, `{}`), nil
		case 11:
			if request.Method != http.MethodPut || request.URL.Path != "/v1/airs/"+airID+"/policy" || !strings.Contains(string(body), `"policy_revision":5`) {
				t.Fatalf("policy=%s %s body=%s", request.Method, request.URL, body)
			}
			return airJSONResponse(request, http.StatusOK, `{}`), nil
		case 12:
			if request.URL.Path != "/v1/airs/"+airID+"/dissolve" || !strings.Contains(string(body), `"air_revision":3`) {
				t.Fatalf("dissolve=%s body=%s", request.URL, body)
			}
			return airJSONResponse(request, http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected request %d", index)
			return nil, nil
		}
	}
	client, err := NewAirAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	list, err := client.List(ctx)
	if err != nil || len(list.Saved) != 1 || list.CurrentAirID == nil || *list.CurrentAirID != airID {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if got, err := client.Detail(ctx, airID); err != nil || got.Policy.Queue != AirPlaybackPrimaryCompanion {
		t.Fatalf("detail=%+v err=%v", got, err)
	}
	if _, err := client.Create(ctx, "Family room", "windows-air-create-0000000000000001"); err != nil {
		t.Fatal(err)
	}
	invite, err := client.IssueInvite(ctx, airID, AirRoleMember, "windows-air-invite-0000000000000001")
	if err != nil || invite.Code == "" {
		t.Fatalf("invite=%+v err=%v", invite, err)
	}
	if strings.Contains(fmt.Sprintf("%v %#v", invite, invite), invite.Code) {
		t.Fatal("invite formatting leaked secret")
	}
	if err := client.WithdrawInvite(ctx, airID, inviteID, 2, "windows-air-withdraw-00000000000001"); err != nil {
		t.Fatal(err)
	}
	preview, err := client.ConsumeInvite(ctx, "secret-one-time-code", "windows-air-consume-000000000000001")
	if err != nil || !preview.ActivationWouldSwitch || preview.OwnerDisplayName != "Ivan" {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	if err := client.ConfirmJoin(ctx, airID, 4, true, "", "windows-air-confirm-000000000000001"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeclineJoin(ctx, airID, 4, "windows-air-decline-000000000000001"); err != nil {
		t.Fatal(err)
	}
	if err := client.Activate(ctx, airID, 4, "", "windows-air-activate-00000000000001"); err != nil {
		t.Fatal(err)
	}
	if err := client.Deactivate(ctx, airID, 4, airID, "windows-air-deactivate-0000000000001"); err != nil {
		t.Fatal(err)
	}
	if err := client.Leave(ctx, airID, 4, airID, "windows-air-leave-0000000000000001"); err != nil {
		t.Fatal(err)
	}
	policy := AirPolicy{Revision: 5, Invite: AirInviteOwnerPrimary, Overlay: AirPlaybackAdminPrimary, Queue: AirPlaybackPrimaryCompanion, Replace: AirPlaybackAllMemberPrimarys}
	if err := client.ReplacePolicy(ctx, airID, policy, "windows-air-policy-000000000000001"); err != nil {
		t.Fatal(err)
	}
	if err := client.Dissolve(ctx, airID, 3, "windows-air-dissolve-0000000000001"); err != nil {
		t.Fatal(err)
	}
}

func TestAirClientRejectsRedirectInvalidProjectionAndCanonicalizesErrors(t *testing.T) {
	doer := &airScriptedDoer{handle: func(request *http.Request, index int) (*http.Response, error) {
		switch index {
		case 0:
			redirected := request.Clone(request.Context())
			redirected.URL, _ = request.URL.Parse("https://attacker.example/v1/airs")
			return airJSONResponse(redirected, http.StatusOK, `{}`), nil
		case 1:
			return airJSONResponse(request, http.StatusConflict, `{"error":{"code":"revision_conflict","retry_after_seconds":2}}`), nil
		default:
			return airJSONResponse(request, http.StatusOK, `{"current_air_id":null,"current_air_id":null,"active_pointer_revision":0,"saved":[]}`), nil
		}
	}}
	client, err := NewAirAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.List(context.Background()); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("redirect err=%v", err)
	}
	if _, err := client.Create(context.Background(), "Room", "windows-air-create-conflict-00000001"); err == nil {
		t.Fatal("revision conflict accepted")
	} else if api, ok := err.(*AirClientError); !ok || api.Code != "revision_conflict" || api.RetryAfterSeconds != 2 {
		t.Fatalf("error=%#v", err)
	}
	if _, err := client.List(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid_response") {
		t.Fatalf("duplicate JSON err=%v", err)
	}
	if _, err := client.Create(context.Background(), " ", "windows-air-invalid-00000000000001"); err == nil {
		t.Fatal("blank title reached transport")
	}
	if len(doer.requests) != 3 {
		t.Fatalf("invalid local input reached transport: %d", len(doer.requests))
	}
}

func TestAirClientAcceptsLocalizedEightyCharacterTitle(t *testing.T) {
	title := strings.Repeat("Я", 80)
	if !validAirTitle(title) {
		t.Fatal("80-character localized title was rejected as 160 bytes")
	}
	if validAirTitle(strings.Repeat("Я", 81)) || validAirTitle(" room ") || validAirTitle("bad\nroom") {
		t.Fatal("invalid Air title accepted")
	}
}
