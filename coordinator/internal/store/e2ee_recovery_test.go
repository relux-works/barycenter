package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestE2EERecoverySharedPolicyVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "e2ee-recovery-v1-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Contract              string   `json:"contract"`
		Status                string   `json:"status"`
		TransferMaxTTLMS      int64    `json:"transfer_max_ttl_ms"`
		HistoryMaxTTLMS       int64    `json:"history_max_ttl_ms"`
		LocalCleanupMaxGrants int      `json:"local_cleanup_max_grants"`
		FailClosed            []string `json:"fail_closed"`
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if vectors.Contract != "e2ee-recovery.v1" || vectors.Status != "production-disabled" ||
		vectors.TransferMaxTTLMS != e2eeTransferPackageMaxTTL.Milliseconds() ||
		vectors.HistoryMaxTTLMS != e2eeHistoryGrantMaxTTL.Milliseconds() ||
		vectors.LocalCleanupMaxGrants != 100 || len(vectors.FailClosed) != 10 {
		t.Fatalf("recovery vectors=%+v", vectors)
	}
}

func e2eeRecoveryDeviceRevision(t *testing.T, f e2eeRoutingFixture, deviceID string) int64 {
	t.Helper()
	device, err := f.store.GetE2EEPublicDevice(deviceID)
	if err != nil {
		t.Fatal(err)
	}
	return device.Revision
}

func e2eeRecoveryGroup(t *testing.T, f e2eeRoutingFixture) E2EEGroup {
	t.Helper()
	group, err := f.store.GetE2EEGroup(f.group.ID)
	if err != nil {
		t.Fatal(err)
	}
	return group
}

func e2eeRecoveryTransferParams(t *testing.T, f e2eeRoutingFixture,
	kind, recipient string, createdAt int64,
) CreateE2EETransferPackageParams {
	t.Helper()
	group := e2eeRecoveryGroup(t, f)
	payload := []byte(fmt.Sprintf("opaque-recovery-package:%s:%s:%d", kind, recipient, createdAt))
	return CreateE2EETransferPackageParams{
		GroupID: group.ID, PackageKind: kind, IssuerDeviceID: f.ownerDevice,
		RecipientDeviceID: recipient, Epoch: group.CurrentEpoch,
		TargetSnapshotDigest:            group.TargetSnapshotDigest,
		ExpectedGroupRevision:           group.Revision,
		ExpectedRecipientDeviceRevision: e2eeRecoveryDeviceRevision(t, f, recipient),
		EncryptedPackage:                payload, PackageDigest: e2eeDigest(payload),
		CreatedAt: createdAt, ExpiresAt: createdAt + 10_000,
	}
}

func e2eeReadyRecoveryObject(t *testing.T, f e2eeRoutingFixture,
	sourceID string,
) E2EEProtectedObject {
	t.Helper()
	chunk := []byte("opaque-recovery-ciphertext:" + sourceID)
	object := stageRoutedCiphertextObject(t, f, sourceID, chunk)
	putRoutedCiphertextChunks(t, f, object, chunk)
	ready, err := f.store.FinalizeE2EEProtectedObject(object.ID, object.Revision, f.now+150)
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func e2eeRecoveryGrantParams(t *testing.T, f e2eeRoutingFixture,
	object E2EEProtectedObject, mode string, maxReads, issuedAt int64,
) CreateE2EEHistoryGrantParams {
	t.Helper()
	group := e2eeRecoveryGroup(t, f)
	payload := []byte(fmt.Sprintf("opaque-history-grant:%s:%s:%d", object.SourceObjectID, mode, issuedAt))
	return CreateE2EEHistoryGrantParams{
		GroupID: group.ID, IssuedByDeviceID: f.ownerDevice,
		RecipientDeviceID: f.peerDevice, SourceObjectID: object.SourceObjectID,
		TargetSnapshotDigest: group.TargetSnapshotDigest,
		FirstEpoch:           object.Epoch, LastEpoch: object.Epoch,
		ExpectedGroupRevision:           group.Revision,
		ExpectedRecipientDeviceRevision: e2eeRecoveryDeviceRevision(t, f, f.peerDevice),
		AccessMode:                      mode, MaxReads: maxReads, ApprovedAt: issuedAt - 1,
		EncryptedGrant: payload, GrantDigest: e2eeDigest(payload),
		IssuedAt: issuedAt, ExpiresAt: issuedAt + 20_000,
	}
}

func TestE2EERecoveryTransferIsBoundCurrentAndOneTime(t *testing.T) {
	f := newE2EERoutingFixture(t)
	params := e2eeRecoveryTransferParams(t, f, "welcome", f.peerDevice, f.now+200)
	created, err := f.store.CreateAuthorizedE2EETransferPackage(params)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.EncryptedPackage) == 0 || created.Status != "pending" ||
		created.TargetSnapshotDigest != f.group.TargetSnapshotDigest {
		t.Fatalf("created transfer=%+v", created)
	}
	if _, err := f.store.ConsumeAuthorizedE2EETransferPackage(created.ID,
		f.ownerDevice, created.Revision, f.now+201); !errors.Is(err, ErrE2EETransferUnavailable) {
		t.Fatalf("foreign consume error=%v", err)
	}
	consumed, err := f.store.ConsumeAuthorizedE2EETransferPackage(created.ID,
		f.peerDevice, created.Revision, f.now+202)
	if err != nil || consumed.Status != "consumed" || consumed.ConsumedAt != f.now+202 {
		t.Fatalf("consumed=%+v err=%v", consumed, err)
	}
	if _, err := f.store.ConsumeAuthorizedE2EETransferPackage(created.ID,
		f.peerDevice, created.Revision, f.now+203); !errors.Is(err, ErrE2EETransferUnavailable) {
		t.Fatalf("replay consume error=%v", err)
	}

	crossOrbit := e2eeRecoveryTransferParams(t, f, "device_transfer", f.peerDevice, f.now+204)
	if _, err := f.store.CreateAuthorizedE2EETransferPackage(crossOrbit); !errors.Is(err, ErrE2EEInvalid) {
		t.Fatalf("cross-orbit device transfer error=%v", err)
	}
	stale := e2eeRecoveryTransferParams(t, f, "welcome", f.peerDevice, f.now+205)
	stale.ExpectedRecipientDeviceRevision++
	if _, err := f.store.CreateAuthorizedE2EETransferPackage(stale); !errors.Is(err, ErrE2EEConflict) {
		t.Fatalf("cloned recipient revision error=%v", err)
	}
	revocable, err := f.store.CreateAuthorizedE2EETransferPackage(
		e2eeRecoveryTransferParams(t, f, "welcome", f.peerDevice, f.now+206))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.RevokeAuthorizedE2EETransferPackage(revocable.ID,
		f.peerDevice, revocable.Revision, f.now+207); !errors.Is(err, ErrE2EETransferUnavailable) {
		t.Fatalf("foreign transfer revoke error=%v", err)
	}
	if revoked, err := f.store.RevokeAuthorizedE2EETransferPackage(revocable.ID,
		f.ownerDevice, revocable.Revision, f.now+208); err != nil || revoked.Status != "revoked" {
		t.Fatalf("transfer revoke=%+v err=%v", revoked, err)
	}
}

func TestE2EESameUserDeviceTransferBootstrapsCurrentEpochWithoutHistory(t *testing.T) {
	f := newE2EERoutingFixture(t)
	newDevice := "routing_owner_device_0002"
	registerE2EERoutingDevice(t, f.store, f.owner, newDevice,
		f.ownerActor, f.now+210)
	requirement, err := f.store.ReconcileE2EERotation(f.group.ID, f.now+211)
	if err != nil || requirement == nil || requirement.State != "required" {
		t.Fatalf("new-device rotation=%+v err=%v", requirement, err)
	}
	snapshot, err := f.store.E2EEAirSnapshot(f.airID)
	if err != nil {
		t.Fatal(err)
	}
	routed, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), e2eeCommitRaw(t, f,
		"commit_for_same_user_transfer_1", snapshot.Digest, strings.Repeat("7", 64)), f.now+212)
	if err != nil || routed.Group.CurrentEpoch != f.group.CurrentEpoch+1 {
		t.Fatalf("new-device commit=%+v err=%v", routed, err)
	}
	params := e2eeRecoveryTransferParams(t, f, "device_transfer", newDevice, f.now+213)
	created, err := f.store.CreateAuthorizedE2EETransferPackage(params)
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := f.store.ConsumeAuthorizedE2EETransferPackage(created.ID,
		newDevice, created.Revision, f.now+214)
	if err != nil || consumed.Epoch != routed.Group.CurrentEpoch || consumed.Status != "consumed" {
		t.Fatalf("current-epoch bootstrap=%+v err=%v", consumed, err)
	}
	var historyGrants int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM e2ee_history_grants
WHERE recipient_device_id = ?`, newDevice).Scan(&historyGrants); err != nil || historyGrants != 0 {
		t.Fatalf("implicit historical grants=%d err=%v", historyGrants, err)
	}
}

func TestE2EEHistoryGrantIsExplicitBoundedAndContentOpaque(t *testing.T) {
	f := newE2EERoutingFixture(t)
	object := e2eeReadyRecoveryObject(t, f, "recovery_history_source_0001")
	params := e2eeRecoveryGrantParams(t, f, object, "one_time", 1, f.now+300)
	grant, err := f.store.CreateAuthorizedE2EEHistoryGrant(params)
	if err != nil {
		t.Fatal(err)
	}
	if grant.Status != "active" || grant.SourceObjectID != object.SourceObjectID {
		t.Fatalf("grant=%+v", grant)
	}
	access, err := f.store.AuthorizeE2EEHistoryGrant(grant.ID, f.peerDevice, f.now+301)
	if err != nil || access.ReadCount != 1 || !strings.HasPrefix(
		string(access.EncryptedGrant), "opaque-history-grant:") {
		t.Fatalf("access=%+v err=%v", access, err)
	}
	if _, err := f.store.AuthorizeE2EEHistoryGrant(grant.ID,
		f.peerDevice, f.now+302); !errors.Is(err, ErrE2EEHistoryUnavailable) {
		t.Fatalf("one-time replay error=%v", err)
	}
	if _, err := f.store.AuthorizeE2EEHistoryGrant(grant.ID,
		f.ownerDevice, f.now+303); !errors.Is(err, ErrE2EEHistoryUnavailable) {
		t.Fatalf("foreign history access error=%v", err)
	}

	var secretColumns int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('e2ee_history_grants')
WHERE lower(name) LIKE '%plaintext%' OR lower(name) LIKE '%media_key%'
OR lower(name) LIKE '%recovery_secret%'`).Scan(&secretColumns); err != nil || secretColumns != 0 {
		t.Fatalf("coordinator recovery secret columns=%d err=%v", secretColumns, err)
	}
	var audits int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM e2ee_audit_events
WHERE subject_id = ? AND operation IN ('history_grant.create','history_grant.read')`,
		grant.ID).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("history grant audits=%d err=%v", audits, err)
	}
	revocable, err := f.store.CreateAuthorizedE2EEHistoryGrant(
		e2eeRecoveryGrantParams(t, f, object, "time_bound", 2, f.now+304))
	if err != nil {
		t.Fatal(err)
	}
	if revoked, err := f.store.RevokeAuthorizedE2EEHistoryGrant(revocable.ID,
		f.peerDevice, revocable.Revision, f.now+305); err != nil || revoked.Status != "revoked" {
		t.Fatalf("history grant revoke=%+v err=%v", revoked, err)
	}
}

func TestE2EEOneTimeHistoryGrantConcurrentReadHasOneWinner(t *testing.T) {
	f := newE2EERoutingFixture(t)
	object := e2eeReadyRecoveryObject(t, f, "recovery_history_race_00001")
	grant, err := f.store.CreateAuthorizedE2EEHistoryGrant(
		e2eeRecoveryGrantParams(t, f, object, "one_time", 1, f.now+350))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(offset int64) {
			defer wg.Done()
			_, err := f.store.AuthorizeE2EEHistoryGrant(grant.ID, f.peerDevice, f.now+351+offset)
			results <- err
		}(int64(i))
	}
	wg.Wait()
	close(results)
	var accepted, rejected int
	for err := range results {
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrE2EEHistoryUnavailable) || errors.Is(err, ErrE2EEConflict) {
			rejected++
		} else {
			t.Fatalf("one-time race error=%v", err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("one-time race accepted=%d rejected=%d", accepted, rejected)
	}
}

func TestE2EERecoveryExpiryCleanupAndLostDeviceRevocation(t *testing.T) {
	f := newE2EERoutingFixture(t)
	object := e2eeReadyRecoveryObject(t, f, "recovery_expiry_source_00001")
	transferParams := e2eeRecoveryTransferParams(t, f, "welcome", f.peerDevice, f.now+400)
	transferParams.ExpiresAt = transferParams.CreatedAt + 5
	transfer, err := f.store.CreateAuthorizedE2EETransferPackage(transferParams)
	if err != nil {
		t.Fatal(err)
	}
	grantParams := e2eeRecoveryGrantParams(t, f, object, "time_bound", 2, f.now+401)
	grantParams.ExpiresAt = grantParams.IssuedAt + 5
	grant, err := f.store.CreateAuthorizedE2EEHistoryGrant(grantParams)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := f.store.ExpireE2EERecoveryArtifacts(f.now+410, 2)
	if err != nil || expired.TransferPackages != 1 || expired.HistoryGrants != 1 {
		t.Fatalf("expiry=%+v err=%v", expired, err)
	}
	if current, err := f.store.GetE2EETransferPackage(transfer.ID); err != nil || current.Status != "expired" {
		t.Fatalf("expired transfer=%+v err=%v", current, err)
	}
	if current, err := f.store.GetE2EEHistoryGrant(grant.ID); err != nil || current.Status != "expired" {
		t.Fatalf("expired grant=%+v err=%v", current, err)
	}

	activeTransfer, err := f.store.CreateAuthorizedE2EETransferPackage(
		e2eeRecoveryTransferParams(t, f, "welcome", f.peerDevice, f.now+420))
	if err != nil {
		t.Fatal(err)
	}
	activeGrant, err := f.store.CreateAuthorizedE2EEHistoryGrant(
		e2eeRecoveryGrantParams(t, f, object, "time_bound", 2, f.now+421))
	if err != nil {
		t.Fatal(err)
	}
	peer, err := f.store.GetE2EEPublicDevice(f.peerDevice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.RevokeE2EEPublicDevice(f.peerDevice, peer.Revision, f.now+422); err != nil {
		t.Fatal(err)
	}
	if current, err := f.store.GetE2EETransferPackage(activeTransfer.ID); err != nil || current.Status != "revoked" {
		t.Fatalf("revoked transfer=%+v err=%v", current, err)
	}
	if current, err := f.store.GetE2EEHistoryGrant(activeGrant.ID); err != nil || current.Status != "revoked" {
		t.Fatalf("revoked grant=%+v err=%v", current, err)
	}
	requirement, err := f.store.GetE2EERotationRequirement(f.group.ID)
	if err != nil || requirement.State != "required" || requirement.ReasonCode != "device_revoke" {
		t.Fatalf("lost-device rotation=%+v err=%v", requirement, err)
	}
	if _, err := f.store.AuthorizeE2EEHistoryGrant(activeGrant.ID,
		f.peerDevice, f.now+423); !errors.Is(err, ErrE2EEHistoryUnavailable) {
		t.Fatalf("revoked history access error=%v", err)
	}
}

func TestE2EERecoveryCheckpointRollbackLeavesNoArtifact(t *testing.T) {
	f := newE2EERoutingFixture(t)
	f.store.testCheckpoint = func(name string) error {
		if name == "e2ee_transfer_package_before_commit" {
			return errors.New("injected recovery crash")
		}
		return nil
	}
	if _, err := f.store.CreateAuthorizedE2EETransferPackage(
		e2eeRecoveryTransferParams(t, f, "welcome", f.peerDevice, f.now+500)); err == nil {
		t.Fatal("expected injected transfer failure")
	}
	f.store.testCheckpoint = nil
	var packages, bindings, audits int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM e2ee_transfer_packages`).Scan(&packages); err != nil {
		t.Fatal(err)
	}
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM e2ee_transfer_package_bindings`).Scan(&bindings); err != nil {
		t.Fatal(err)
	}
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM e2ee_audit_events
WHERE operation = 'transfer_package.create'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if packages != 0 || bindings != 0 || audits != 0 {
		t.Fatalf("rollback packages=%d bindings=%d audits=%d", packages, bindings, audits)
	}
}
