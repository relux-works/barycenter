package store

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
)

func telegramAirTestAuth(t *testing.T, st *Store, userID int64, key string, now int64) AirMutationAuth {
	t.Helper()
	ctx, err := st.ResolveTelegramActorContext(userID)
	if err != nil {
		t.Fatal(err)
	}
	k, r := sha256.Sum256([]byte(key)), sha256.Sum256([]byte("request:"+key))
	return AirMutationAuth{ExpectedActorID: ctx.ActorID,
		Identity:           Identity{Kind: IdentityTelegram, TelegramUserID: userID},
		IdempotencyKeyHash: hex.EncodeToString(k[:]), RequestHash: hex.EncodeToString(r[:]), Now: now}
}

func TestTelegramAirCallbacksAreOpaqueActorBoundExpiringAndAtomic(t *testing.T) {
	st := openIdentityTemp(t)
	_ = createTelegramAliasOrbit(t, st, "Owner", 101)
	_ = createTelegramAliasOrbit(t, st, "Foreign", 202)
	if _, err := st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateAuthorizedAir(telegramAirTestAuth(t, st, 101, "create", 200), "Friends")
	if err != nil {
		t.Fatal(err)
	}
	if created.AirID == "" || created.MembershipID == "" {
		t.Fatal("invalid Air fixture")
	}
	binding := TelegramAirBinding{Action: TelegramAirActivate, AirID: created.AirID,
		MembershipID: created.MembershipID, AirRevision: created.Revision,
		MembershipRevision: created.MembershipRevision, ExpectedActiveAirID: "none", Policy: created.Policy}
	token, err := st.MintTelegramAirCallback(MintTelegramAirCallbackParams{
		TelegramUserID: 101, ChatID: 101, MessageID: 77, Binding: binding, Now: 300})
	if err != nil {
		t.Fatal(err)
	}
	if !telegramCallbackPattern.MatchString(token) || strings.Contains(token, created.AirID) ||
		strings.Contains(token, created.MembershipID) {
		t.Fatalf("callback leaked binding: %q", token)
	}
	forged, err := st.ClaimTelegramAirCallback(ClaimTelegramAirCallbackParams{
		TelegramUserID: 101, ChatID: 101, MessageID: 77, QueryID: "forged", Token: "tg1_" + strings.Repeat("A", 32), Now: 301})
	if err != nil || forged.Found {
		t.Fatalf("forged=%+v err=%v", forged, err)
	}
	foreign, err := st.ClaimTelegramAirCallback(ClaimTelegramAirCallbackParams{
		TelegramUserID: 202, ChatID: 101, MessageID: 77, QueryID: "foreign", Token: token, Now: 302})
	if err != nil || !foreign.Found || foreign.Outcome != TelegramCallbackForbidden || foreign.Binding != nil {
		t.Fatalf("foreign=%+v err=%v", foreign, err)
	}

	start := make(chan struct{})
	results := make(chan TelegramAirCallbackResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			result, claimErr := st.ClaimTelegramAirCallback(ClaimTelegramAirCallbackParams{
				TelegramUserID: 101, ChatID: 101, MessageID: 77,
				QueryID: string(rune('a' + index)), Token: token, Now: 303 + int64(index)})
			results <- result
			errs <- claimErr
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for claimErr := range errs {
		if claimErr != nil {
			t.Fatal(claimErr)
		}
	}
	claimed, repeated := 0, 0
	for result := range results {
		if result.Binding != nil && result.Outcome == TelegramCallbackApplied {
			claimed++
		}
		if result.Binding == nil && result.Outcome == TelegramCallbackAlreadyApplied {
			repeated++
		}
	}
	if claimed != 1 || repeated != 1 {
		t.Fatalf("claimed=%d repeated=%d", claimed, repeated)
	}

	expiring, err := st.MintTelegramAirCallback(MintTelegramAirCallbackParams{
		TelegramUserID: 101, ChatID: 101, MessageID: 78, Binding: binding, Now: 400})
	if err != nil {
		t.Fatal(err)
	}
	expired, err := st.ClaimTelegramAirCallback(ClaimTelegramAirCallbackParams{
		TelegramUserID: 101, ChatID: 101, MessageID: 78, QueryID: "expired", Token: expiring,
		Now: 400 + telegramCallbackTTL.Milliseconds()})
	if err != nil || expired.Outcome != TelegramCallbackExpired || expired.Binding != nil || !expired.ClearKeyboard {
		t.Fatalf("expired=%+v err=%v", expired, err)
	}

	finalToken, err := st.MintTelegramAirCallback(MintTelegramAirCallbackParams{
		TelegramUserID: 101, ChatID: 101, MessageID: 80, Binding: binding, Now: 450})
	if err != nil {
		t.Fatal(err)
	}
	finalClaim, err := st.ClaimTelegramAirCallback(ClaimTelegramAirCallbackParams{
		TelegramUserID: 101, ChatID: 101, MessageID: 80, QueryID: "finalized", Token: finalToken, Now: 451})
	if err != nil || finalClaim.Binding == nil {
		t.Fatalf("final claim=%+v err=%v", finalClaim, err)
	}
	if err := st.FinalizeTelegramAirCallback(finalToken, "finalized", TelegramCallbackTooLate, 452); err != nil {
		t.Fatal(err)
	}
	finalReplay, err := st.ClaimTelegramAirCallback(ClaimTelegramAirCallbackParams{
		TelegramUserID: 101, ChatID: 101, MessageID: 80, QueryID: "finalized", Token: finalToken, Now: 453})
	if err != nil || !finalReplay.Replay || finalReplay.Outcome != TelegramCallbackTooLate || finalReplay.Binding != nil {
		t.Fatalf("final replay=%+v err=%v", finalReplay, err)
	}

	roleToken, err := st.MintTelegramAirCallback(MintTelegramAirCallbackParams{
		TelegramUserID: 101, ChatID: 101, MessageID: 79, Binding: binding, Now: 500})
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := st.ResolveTelegramActorContext(101)
	if _, err := st.db.Exec(`UPDATE memberships SET role = 'companion' WHERE actor_id = ?`, ctx.ActorID); err != nil {
		t.Fatal(err)
	}
	roleChanged, err := st.ClaimTelegramAirCallback(ClaimTelegramAirCallbackParams{
		TelegramUserID: 101, ChatID: 101, MessageID: 79, QueryID: "role", Token: roleToken, Now: 501})
	if err != nil || roleChanged.Outcome != TelegramCallbackForbidden || roleChanged.Binding != nil {
		t.Fatalf("roleChanged=%+v err=%v", roleChanged, err)
	}
}
