package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

type rateLimitAuditRow struct {
	class   string
	digest  string
	orbitID sql.NullInt64
	actorID sql.NullInt64
}

func apiRequestFrom(handler http.Handler, source, method, path, body, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = source
	req.TLS = &tls.ConnectionState{}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func runtimeHTTPRecoveryID(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return "rec_" + hex.EncodeToString(raw)
}

func expectedRateLimitDigest(class store.RateLimitAuditClass, subject string) string {
	sum := sha256.Sum256([]byte("barycenter/rate-limit-subject/v1:" + string(class) + ":" + subject))
	return hex.EncodeToString(sum[:])
}

func loadRateLimitAuditRows(t *testing.T, path string) []rateLimitAuditRow {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT limiter_class, subject_digest, orbit_id, actor_id
FROM rate_limit_audit_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result []rateLimitAuditRow
	for rows.Next() {
		var row rateLimitAuditRow
		if err := rows.Scan(&row.class, &row.digest, &row.orbitID, &row.actorID); err != nil {
			t.Fatal(err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertSingleRateLimitAudit(t *testing.T, harness onboardingHarness, class store.RateLimitAuditClass, subject string, orbitID, actorID int64) {
	t.Helper()
	rows := loadRateLimitAuditRows(t, harness.path)
	if len(rows) != 1 {
		t.Fatalf("rate-limit audit rows=%d want=1", len(rows))
	}
	row := rows[0]
	if row.class != string(class) || row.digest != expectedRateLimitDigest(class, subject) || row.digest == subject || len(row.digest) != 64 {
		t.Fatalf("rate-limit audit class=%q digest_shape=%d class_matches=%v digest_matches=%v",
			row.class, len(row.digest), row.class == string(class), row.digest == expectedRateLimitDigest(class, subject))
	}
	if orbitID == 0 || actorID == 0 {
		if row.orbitID.Valid || row.actorID.Valid {
			t.Fatalf("pre-identity audit carried scope orbit=%v actor=%v", row.orbitID, row.actorID)
		}
	} else if !row.orbitID.Valid || !row.actorID.Valid || row.orbitID.Int64 != orbitID || row.actorID.Int64 != actorID {
		t.Fatalf("scoped audit orbit=%v actor=%v want=%d/%d", row.orbitID, row.actorID, orbitID, actorID)
	}
	db, err := sql.Open("sqlite", harness.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var raw int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rate_limit_audit_events
WHERE limiter_class = ? OR subject_digest = ?`, subject, subject).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != 0 {
		t.Fatal("raw rate-limit subject entered durable audit row")
	}
}

func TestR2EveryHTTPRateLimitClassPersistsDurableAudit(t *testing.T) {
	t.Run("create_source_ip", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		const source = "198.51.100.41:45001"
		for i := 0; i <= 5; i++ {
			body := fmt.Sprintf(`{"title":"Source %d","installation_attempt_id":"source_attempt_%04d"}`, i, i)
			response := apiRequestFrom(harness.mux, source, http.MethodPost, "/v1/onboarding/orbits", body, "")
			if i < 5 && response.Code != http.StatusCreated {
				t.Fatalf("attempt %d status=%d", i+1, response.Code)
			}
			if i == 5 {
				assertAPIError(t, response, http.StatusTooManyRequests, errorTooManyAttempts,
					func(value *int64) bool { return value != nil && *value > 0 })
			}
		}
		assertSingleRateLimitAudit(t, harness, store.RateLimitCreateSourceIP, "198.51.100.41", 0, 0)
	})

	t.Run("create_installation_attempt", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		attempt := "shared_install_attempt_0042"
		for i := 0; i < 2; i++ {
			body := fmt.Sprintf(`{"title":"Attempt %d","installation_attempt_id":%q}`, i, attempt)
			response := apiRequestFrom(harness.mux, fmt.Sprintf("198.51.100.%d:45002", i+50),
				http.MethodPost, "/v1/onboarding/orbits", body, "")
			if i == 0 && response.Code != http.StatusCreated {
				t.Fatalf("first attempt status=%d", response.Code)
			}
			if i == 1 {
				assertAPIError(t, response, http.StatusTooManyRequests, errorTooManyAttempts,
					func(value *int64) bool { return value != nil && *value > 0 })
			}
		}
		assertSingleRateLimitAudit(t, harness, store.RateLimitCreateInstallationAttempt, attempt, 0, 0)
	})

	t.Run("invite_consume_source_ip", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		owner := createViaAPI(t, harness)
		issued := apiRequest(harness.mux, http.MethodPost, "/v1/device-invites",
			`{"intended_role":"companion"}`, owner["control_token"].(string))
		if issued.Code != http.StatusCreated {
			t.Fatalf("invite issue status=%d", issued.Code)
		}
		inviteCode := decodeObject(t, issued)["invite_code"].(string)
		bodyBytes, err := json.Marshal(map[string]string{"invite_code": inviteCode})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i <= 20; i++ {
			response := apiRequest(harness.mux, http.MethodPost, "/v1/device-invites/consume", string(bodyBytes), "")
			switch {
			case i == 0 && response.Code != http.StatusOK:
				t.Fatalf("invite winner status=%d", response.Code)
			case i > 0 && i < 20:
				assertAPIError(t, response, http.StatusForbidden, errorCredentialInvalid, nil)
			case i == 20:
				assertAPIError(t, response, http.StatusTooManyRequests, errorTooManyAttempts,
					func(value *int64) bool { return value != nil && *value > 0 })
			}
		}
		assertSingleRateLimitAudit(t, harness, store.RateLimitInviteConsumeSourceIP, "127.0.0.1", 0, 0)
	})

	t.Run("recovery_consume_source_ip", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		secret, replacement := runtimeHTTPTestHumanCode(t), runtimeHTTPTestToken(t)
		for i := 0; i <= 30; i++ {
			bodyBytes, err := json.Marshal(map[string]string{
				"recovery_id": runtimeHTTPRecoveryID(t), "recovery_secret": secret,
				"replacement_control_token": replacement,
			})
			if err != nil {
				t.Fatal(err)
			}
			response := apiRequest(harness.mux, http.MethodPost, "/v1/recovery/consume", string(bodyBytes), "")
			if i < 30 {
				assertAPIError(t, response, http.StatusForbidden, errorCredentialInvalid, nil)
			} else {
				assertAPIError(t, response, http.StatusTooManyRequests, errorTooManyAttempts,
					func(value *int64) bool { return value != nil && *value > 0 })
			}
		}
		assertSingleRateLimitAudit(t, harness, store.RateLimitRecoveryConsumeSourceIP, "127.0.0.1", 0, 0)
	})

	t.Run("recovery_consume_recovery_id", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		recoveryID := runtimeHTTPRecoveryID(t)
		bodyBytes, err := json.Marshal(map[string]string{
			"recovery_id": recoveryID, "recovery_secret": runtimeHTTPTestHumanCode(t),
			"replacement_control_token": runtimeHTTPTestToken(t),
		})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i <= 10; i++ {
			response := apiRequest(harness.mux, http.MethodPost, "/v1/recovery/consume", string(bodyBytes), "")
			if i < 10 {
				assertAPIError(t, response, http.StatusForbidden, errorCredentialInvalid, nil)
			} else {
				assertAPIError(t, response, http.StatusTooManyRequests, errorTooManyAttempts,
					func(value *int64) bool { return value != nil && *value > 0 })
			}
		}
		assertSingleRateLimitAudit(t, harness, store.RateLimitRecoveryConsumeRecoveryID, recoveryID, 0, 0)
	})

	t.Run("recovery_rotate_actor", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		owner := createViaAPI(t, harness)
		for i := 0; i <= 10; i++ {
			response := apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{}`, owner["control_token"].(string))
			if i < 10 && response.Code != http.StatusOK {
				t.Fatalf("rotation attempt %d status=%d", i+1, response.Code)
			}
			if i == 10 {
				assertAPIError(t, response, http.StatusTooManyRequests, errorTooManyAttempts,
					func(value *int64) bool { return value != nil && *value > 0 })
			}
		}
		actorID, orbitID := int64(owner["actor_id"].(float64)), int64(owner["orbit_id"].(float64))
		assertSingleRateLimitAudit(t, harness, store.RateLimitRecoveryRotateActor,
			strconv.FormatInt(actorID, 10), orbitID, actorID)
	})

	t.Run("telegram_link_issue_actor", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		owner := createViaAPI(t, harness)
		for i := 0; i <= 10; i++ {
			response := apiRequest(harness.mux, http.MethodPost, "/v1/telegram-links",
				`{"desired_role":"companion"}`, owner["control_token"].(string))
			if i < 10 && response.Code != http.StatusCreated {
				t.Fatalf("link attempt %d status=%d", i+1, response.Code)
			}
			if i == 10 {
				assertAPIError(t, response, http.StatusTooManyRequests, errorTooManyAttempts,
					func(value *int64) bool { return value != nil && *value > 0 })
			}
		}
		actorID, orbitID := int64(owner["actor_id"].(float64)), int64(owner["orbit_id"].(float64))
		assertSingleRateLimitAudit(t, harness, store.RateLimitTelegramLinkIssueActor,
			strconv.FormatInt(actorID, 10), orbitID, actorID)
	})
}

func TestR2RateLimitAuditFailureSuppresses429AndRetryAfter(t *testing.T) {
	for _, test := range []struct {
		name   string
		scoped bool
	}{
		{name: "unscoped", scoped: false},
		{name: "scoped", scoped: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newOnboardingHarness(t)
			var failed *httptest.ResponseRecorder
			var retried *httptest.ResponseRecorder
			var subject string
			var class store.RateLimitAuditClass
			var orbitID, actorID int64
			var bearer string
			if test.scoped {
				seed, err := sql.Open("sqlite", harness.path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := seed.Exec(`INSERT INTO actors
  (id, kind, display_name, external_ref, created_at, revoked_at)
VALUES(987654321, 'telegram_user', '', 'rate-audit-test-seed', ?, NULL)`, time.Now().UnixMilli()); err != nil {
					seed.Close()
					t.Fatal(err)
				}
				if err := seed.Close(); err != nil {
					t.Fatal(err)
				}
				owner := createViaAPI(t, harness)
				actorID = int64(owner["actor_id"].(float64))
				orbitID = int64(owner["orbit_id"].(float64))
				bearer = owner["control_token"].(string)
				subject = strconv.FormatInt(actorID, 10)
				class = store.RateLimitRecoveryRotateActor
				harness.api.rotateActor = newAttemptLimiter(1, time.Hour, 0)
				allowed := apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{}`, bearer)
				if allowed.Code != http.StatusOK {
					t.Fatalf("pre-boundary rotation status=%d", allowed.Code)
				}
			} else {
				const source = "203.0.113.79:47000"
				subject = "203.0.113.79"
				class = store.RateLimitCreateSourceIP
				harness.api.createIP = newAttemptLimiter(1, time.Hour, 10_000)
				allowed := apiRequestFrom(harness.mux, source, http.MethodPost, "/v1/onboarding/orbits",
					`{"title":"Audit allowed","installation_attempt_id":"audit_allowed_attempt"}`, "")
				if allowed.Code != http.StatusCreated {
					t.Fatalf("pre-boundary create status=%d", allowed.Code)
				}
			}

			inspect, err := sql.Open("sqlite", harness.path)
			if err != nil {
				t.Fatal(err)
			}
			defer inspect.Close()
			if _, err := inspect.Exec(`CREATE TRIGGER fail_rate_limit_audit
BEFORE INSERT ON rate_limit_audit_events
BEGIN SELECT RAISE(ABORT, 'rate-limit audit unavailable'); END`); err != nil {
				t.Fatal(err)
			}
			if test.scoped {
				failed = apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{}`, bearer)
			} else {
				failed = apiRequestFrom(harness.mux, "203.0.113.79:47000", http.MethodPost, "/v1/onboarding/orbits",
					`{"title":"Audit failure","installation_attempt_id":"audit_failure_attempt"}`, "")
			}
			assertAPIError(t, failed, http.StatusInternalServerError, errorInternal, nil)
			if failed.Header().Get("Retry-After") != "" {
				t.Fatalf("audit failure emitted Retry-After=%q", failed.Header().Get("Retry-After"))
			}
			var logRecord map[string]any
			lines := strings.Split(strings.TrimSpace(harness.logs.String()), "\n")
			if len(lines) == 0 || json.Unmarshal([]byte(lines[len(lines)-1]), &logRecord) != nil {
				t.Fatal("audit persistence failure did not produce a parseable structural log")
			}
			if strings.Contains(fmt.Sprint(logRecord["msg"]), subject) || strings.Contains(fmt.Sprint(logRecord["err"]), subject) || strings.Contains(failed.Body.String(), subject) {
				t.Fatal("audit persistence failure leaked the limiter subject")
			}
			if rows := loadRateLimitAuditRows(t, harness.path); len(rows) != 0 {
				t.Fatalf("failed audit insert persisted %d rows", len(rows))
			}
			if _, err := inspect.Exec(`DROP TRIGGER fail_rate_limit_audit`); err != nil {
				t.Fatal(err)
			}
			if test.scoped {
				retried = apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{}`, bearer)
			} else {
				retried = apiRequestFrom(harness.mux, "203.0.113.79:47000", http.MethodPost, "/v1/onboarding/orbits",
					`{"title":"Audit retry","installation_attempt_id":"audit_retry_attempt_1"}`, "")
			}
			assertAPIError(t, retried, http.StatusTooManyRequests, errorTooManyAttempts,
				func(value *int64) bool { return value != nil && *value > 0 })
			assertSingleRateLimitAudit(t, harness, class, subject, orbitID, actorID)
		})
	}
}
