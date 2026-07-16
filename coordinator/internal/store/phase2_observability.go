package store

import (
	"database/sql"
	"time"
)

const Phase2ObservabilityWindow = 24 * time.Hour

type Phase2FeatureState struct {
	Enabled  bool   `json:"enabled"`
	State    string `json:"state"`
	Revision int64  `json:"revision"`
}

type Phase2FeatureStates struct {
	StreamedTracks Phase2FeatureState `json:"streamed_tracks"`
	AirRooms       Phase2FeatureState `json:"air_rooms"`
	TargetsInbox   Phase2FeatureState `json:"targets_inbox"`
}

type Phase2Readiness struct {
	Ready                    bool `json:"ready"`
	StreamAccountingRequired bool `json:"stream_accounting_required"`
	StreamAccountingReady    bool `json:"stream_accounting_ready"`
	MediaProcessorRequired   bool `json:"media_processor_required"`
	MediaProcessorReady      bool `json:"media_processor_ready"`
	StreamStorageRequired    bool `json:"stream_storage_required"`
	StreamStorageReady       bool `json:"stream_storage_ready"`
	AirRuntimeRequired       bool `json:"air_runtime_required"`
	AirRuntimeReady          bool `json:"air_runtime_ready"`
}

type Phase2LatencyMetric struct {
	Status      string `json:"status"`
	SampleCount int64  `json:"sample_count"`
	P95MS       int64  `json:"p95_ms"`
	MaxMS       int64  `json:"max_ms"`
	Clock       string `json:"clock"`
}

type Phase2TimingMetrics struct {
	ReleaseToReady Phase2LatencyMetric `json:"release_to_ready"`
	TrackStart     Phase2LatencyMetric `json:"track_start"`
	StartSkew      Phase2LatencyMetric `json:"start_skew"`
	SeekToAudio    Phase2LatencyMetric `json:"seek_to_audio"`
}

type Phase2ProcessingMetrics struct {
	ActiveJobs            int64               `json:"active_jobs"`
	Succeeded24h          int64               `json:"succeeded_24h"`
	Failed24h             int64               `json:"failed_24h"`
	Expired24h            int64               `json:"expired_24h"`
	ProcessorFailures24h  int64               `json:"processor_failures_24h"`
	ValidationFailures24h int64               `json:"validation_failures_24h"`
	Latency               Phase2LatencyMetric `json:"latency"`
}

type Phase2PlaybackMetrics struct {
	Domains            int64  `json:"domains"`
	BufferingDomains   int64  `json:"buffering_domains"`
	PlayingDomains     int64  `json:"playing_domains"`
	PausedDomains      int64  `json:"paused_domains"`
	QueuedItems        int64  `json:"queued_items"`
	ActiveItems        int64  `json:"active_items"`
	SeekGenerations    int64  `json:"seek_generations"`
	BufferSampleStatus string `json:"buffer_sample_status"`
	SeekSampleStatus   string `json:"seek_sample_status"`
}

type Phase2TargetStatusCounts struct {
	Accepted       int64 `json:"accepted"`
	Preparing      int64 `json:"preparing"`
	Ready          int64 `json:"ready"`
	Scheduled      int64 `json:"scheduled"`
	Playing        int64 `json:"playing"`
	Cancelling     int64 `json:"cancelling"`
	Played         int64 `json:"played"`
	MissedOffline  int64 `json:"missed_offline"`
	MissedDND      int64 `json:"missed_dnd"`
	MissedNotReady int64 `json:"missed_not_ready"`
	Blocked        int64 `json:"blocked"`
	Failed         int64 `json:"failed"`
	Cancelled      int64 `json:"cancelled"`
	Expired        int64 `json:"expired"`
}

type Phase2InboxReasonCounts struct {
	OfflineAtAcceptance  int64 `json:"offline_at_acceptance"`
	OfflineBeforePrepare int64 `json:"offline_before_prepare"`
	OfflineBeforeStart   int64 `json:"offline_before_start"`
	LocalDND             int64 `json:"local_dnd"`
	OrbitDND             int64 `json:"orbit_dnd"`
	PrepareDeadline      int64 `json:"prepare_deadline"`
	ConnectionLost       int64 `json:"connection_lost"`
	DeviceUnavailable    int64 `json:"device_unavailable"`
	AudioGraphFailed     int64 `json:"audio_graph_failed"`
}

type Phase2DeliveryMetrics struct {
	TargetStatuses           Phase2TargetStatusCounts `json:"target_statuses"`
	InboxReasons             Phase2InboxReasonCounts  `json:"inbox_reasons"`
	InboxAvailable           int64                    `json:"inbox_available"`
	InboxReplayed            int64                    `json:"inbox_replayed"`
	InboxUnavailable         int64                    `json:"inbox_unavailable"`
	DuplicateTargetAnomalies int64                    `json:"duplicate_target_anomalies"`
}

type Phase2AirMetrics struct {
	AuthorityState        string `json:"authority_state"`
	AuthorityGeneration   int64  `json:"authority_generation"`
	DivergenceCount       int64  `json:"divergence_count"`
	Parked                int64  `json:"parked"`
	Active                int64  `json:"active"`
	Dissolved             int64  `json:"dissolved"`
	JoinedMembers         int64  `json:"joined_members"`
	PendingMembers        int64  `json:"pending_members"`
	ActivePointers        int64  `json:"active_pointers"`
	RuntimeShapeAnomalies int64  `json:"runtime_shape_anomalies"`
}

type Phase2ObservabilitySnapshot struct {
	SchemaVersion int                      `json:"schema_version"`
	Contract      string                   `json:"contract"`
	GeneratedAtMS int64                    `json:"generated_at_ms"`
	WindowSeconds int64                    `json:"window_seconds"`
	Features      Phase2FeatureStates      `json:"features"`
	Readiness     Phase2Readiness          `json:"readiness"`
	Accounting    StreamAccountingSnapshot `json:"accounting"`
	Timing        Phase2TimingMetrics      `json:"timing"`
	Processing    Phase2ProcessingMetrics  `json:"processing"`
	Playback      Phase2PlaybackMetrics    `json:"playback"`
	Delivery      Phase2DeliveryMetrics    `json:"delivery"`
	Air           Phase2AirMetrics         `json:"air"`
}

type Phase2HealthSnapshot struct {
	Features   Phase2FeatureStates      `json:"features"`
	Readiness  Phase2Readiness          `json:"readiness"`
	Accounting StreamAccountingSnapshot `json:"accounting"`
}

type phase2ObservabilityQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

func phase2LatencyMetric(q phase2ObservabilityQuerier, query string, args ...any) (Phase2LatencyMetric, error) {
	metric := Phase2LatencyMetric{Status: "no_samples", Clock: "coordinator_wall_clock_milliseconds"}
	if err := q.QueryRow(
		`SELECT COUNT(*), COALESCE(MAX(value), 0) FROM (`+query+`) AS samples`, args...,
	).Scan(&metric.SampleCount, &metric.MaxMS); err != nil {
		return Phase2LatencyMetric{}, err
	}
	if metric.SampleCount == 0 {
		return metric, nil
	}
	rank := (95*metric.SampleCount + 99) / 100
	p95Args := append(append([]any{}, args...), rank-1)
	if err := q.QueryRow(
		`SELECT value FROM (`+query+`) AS samples ORDER BY value LIMIT 1 OFFSET ?`, p95Args...,
	).Scan(&metric.P95MS); err != nil {
		return Phase2LatencyMetric{}, err
	}
	metric.Status = "observed"
	return metric, nil
}

func phase2ObservabilitySnapshot(q phase2ObservabilityQuerier, now int64) (Phase2ObservabilitySnapshot, error) {
	if now <= 0 {
		return Phase2ObservabilitySnapshot{}, ErrStreamAccountingInvalid
	}
	cutoff := now - Phase2ObservabilityWindow.Milliseconds()
	view := Phase2ObservabilitySnapshot{
		SchemaVersion: 1,
		Contract:      "p2-observability-quota-view.v1",
		GeneratedAtMS: now,
		WindowSeconds: int64(Phase2ObservabilityWindow / time.Second),
	}
	var streamEnabled int64
	if err := q.QueryRow(`SELECT production_selection_enabled, contract_version, revision
FROM stream_variant_policy WHERE singleton = 1`).Scan(
		&streamEnabled, &view.Features.StreamedTracks.State,
		&view.Features.StreamedTracks.Revision,
	); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	view.Features.StreamedTracks.Enabled = streamEnabled == 1
	if err := q.QueryRow(`SELECT mode, generation, divergence_count, updated_at
FROM air_authority WHERE singleton = 1`).Scan(
		&view.Air.AuthorityState, &view.Air.AuthorityGeneration,
		&view.Air.DivergenceCount, &view.Features.AirRooms.Revision,
	); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	view.Features.AirRooms = Phase2FeatureState{
		Enabled:  view.Air.AuthorityState == "airs_authoritative",
		State:    view.Air.AuthorityState,
		Revision: view.Air.AuthorityGeneration,
	}
	view.Features.TargetsInbox = Phase2FeatureState{Enabled: true, State: "available", Revision: 1}
	accounting, err := streamAccountingSnapshot(q, now)
	if err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	view.Accounting = accounting
	view.Readiness = Phase2Readiness{
		StreamAccountingRequired: view.Features.StreamedTracks.Enabled,
		StreamAccountingReady:    accounting.Ready,
		MediaProcessorRequired:   view.Features.StreamedTracks.Enabled,
		MediaProcessorReady:      !view.Features.StreamedTracks.Enabled,
		StreamStorageRequired:    view.Features.StreamedTracks.Enabled,
		StreamStorageReady:       !view.Features.StreamedTracks.Enabled,
		AirRuntimeRequired:       view.Features.AirRooms.Enabled,
		AirRuntimeReady:          !view.Features.AirRooms.Enabled || view.Air.DivergenceCount == 0,
	}
	view.Readiness.Ready = (!view.Readiness.StreamAccountingRequired || view.Readiness.StreamAccountingReady) &&
		view.Readiness.MediaProcessorReady && view.Readiness.StreamStorageReady && view.Readiness.AirRuntimeReady

	view.Timing.ReleaseToReady, err = phase2LatencyMetric(q, `SELECT tt.ready_at - t.accepted_at AS value
FROM transmission_targets tt JOIN transmissions t ON t.id = tt.transmission_id
JOIN media_items m ON m.id = t.media_id
WHERE m.kind = 'audio_track' AND tt.ready_at >= t.accepted_at AND tt.ready_at > ?`, cutoff)
	if err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	view.Timing.TrackStart, err = phase2LatencyMetric(q, `SELECT tt.started_at - t.accepted_at AS value
FROM transmission_targets tt JOIN transmissions t ON t.id = tt.transmission_id
JOIN media_items m ON m.id = t.media_id
WHERE m.kind = 'audio_track' AND tt.started_at >= t.accepted_at AND tt.started_at > ?`, cutoff)
	if err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	view.Timing.StartSkew, err = phase2LatencyMetric(q, `SELECT MAX(tt.started_at) - MIN(tt.started_at) AS value
FROM transmission_targets tt JOIN transmissions t ON t.id = tt.transmission_id
JOIN media_items m ON m.id = t.media_id
WHERE m.kind = 'audio_track' AND tt.started_at > ?
GROUP BY tt.transmission_id HAVING COUNT(*) >= 2`, cutoff)
	if err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	view.Timing.SeekToAudio = Phase2LatencyMetric{
		Status: "client_evidence_required", Clock: "process_monotonic_clock",
	}
	if err := q.QueryRow(`SELECT COALESCE(SUM(state='active'),0),
COALESCE(SUM(state='succeeded' AND completed_at > ?),0),
COALESCE(SUM(state='failed' AND completed_at > ?),0),
COALESCE(SUM(state='expired' AND completed_at > ?),0),
COALESCE(SUM(outcome='processor_failed' AND completed_at > ?),0),
COALESCE(SUM(outcome='validation_failed' AND completed_at > ?),0)
FROM stream_processing_jobs`, cutoff, cutoff, cutoff, cutoff, cutoff).Scan(
		&view.Processing.ActiveJobs, &view.Processing.Succeeded24h,
		&view.Processing.Failed24h, &view.Processing.Expired24h,
		&view.Processing.ProcessorFailures24h, &view.Processing.ValidationFailures24h,
	); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	view.Processing.Latency, err = phase2LatencyMetric(q, `SELECT completed_at - created_at AS value
FROM stream_processing_jobs WHERE completed_at > ? AND completed_at >= created_at`, cutoff)
	if err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}

	if err := q.QueryRow(`SELECT COUNT(*),
COALESCE(SUM(state = 'buffering'), 0), COALESCE(SUM(state = 'playing'), 0),
COALESCE(SUM(state = 'paused'), 0), COALESCE(SUM(seek_generation), 0)
FROM stream_playback_domains`).Scan(&view.Playback.Domains,
		&view.Playback.BufferingDomains, &view.Playback.PlayingDomains,
		&view.Playback.PausedDomains, &view.Playback.SeekGenerations); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	if err := q.QueryRow(`SELECT COALESCE(SUM(state = 'queued'), 0),
COALESCE(SUM(state = 'active'), 0) FROM stream_queue_items`).Scan(
		&view.Playback.QueuedItems, &view.Playback.ActiveItems,
	); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	view.Playback.BufferSampleStatus = "client_evidence_required"
	view.Playback.SeekSampleStatus = "client_evidence_required"

	targets := &view.Delivery.TargetStatuses
	if err := q.QueryRow(`SELECT
COALESCE(SUM(status='accepted'),0), COALESCE(SUM(status='preparing'),0),
COALESCE(SUM(status='ready'),0), COALESCE(SUM(status='scheduled'),0),
COALESCE(SUM(status='playing'),0), COALESCE(SUM(status='cancelling'),0),
COALESCE(SUM(status='played'),0), COALESCE(SUM(status='missed_offline'),0),
COALESCE(SUM(status='missed_dnd'),0), COALESCE(SUM(status='missed_not_ready'),0),
COALESCE(SUM(status='blocked'),0), COALESCE(SUM(status='failed'),0),
COALESCE(SUM(status='cancelled'),0), COALESCE(SUM(status='expired'),0)
FROM transmission_targets WHERE updated_at > ?`, cutoff).Scan(
		&targets.Accepted, &targets.Preparing, &targets.Ready, &targets.Scheduled,
		&targets.Playing, &targets.Cancelling, &targets.Played, &targets.MissedOffline,
		&targets.MissedDND, &targets.MissedNotReady, &targets.Blocked, &targets.Failed,
		&targets.Cancelled, &targets.Expired,
	); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	reasons := &view.Delivery.InboxReasons
	if err := q.QueryRow(`SELECT
COALESCE(SUM(missed_reason='offline_at_acceptance'),0),
COALESCE(SUM(missed_reason='offline_before_prepare'),0),
COALESCE(SUM(missed_reason='offline_before_start'),0),
COALESCE(SUM(missed_reason='local_dnd'),0), COALESCE(SUM(missed_reason='orbit_dnd'),0),
COALESCE(SUM(missed_reason='prepare_deadline'),0),
COALESCE(SUM(missed_reason='connection_lost'),0),
COALESCE(SUM(missed_reason='device_unavailable'),0),
COALESCE(SUM(missed_reason='audio_graph_failed'),0),
COALESCE(SUM(availability='available'),0), COALESCE(SUM(availability='replayed'),0),
COALESCE(SUM(availability='unavailable'),0)
FROM transmission_inbox_items WHERE created_at > ?`, cutoff).Scan(
		&reasons.OfflineAtAcceptance, &reasons.OfflineBeforePrepare,
		&reasons.OfflineBeforeStart, &reasons.LocalDND, &reasons.OrbitDND,
		&reasons.PrepareDeadline, &reasons.ConnectionLost, &reasons.DeviceUnavailable,
		&reasons.AudioGraphFailed, &view.Delivery.InboxAvailable,
		&view.Delivery.InboxReplayed, &view.Delivery.InboxUnavailable,
	); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	if err := q.QueryRow(`SELECT COUNT(*) FROM (
SELECT transmission_id, actor_id, COUNT(*) AS n FROM transmission_targets
GROUP BY transmission_id, actor_id HAVING n > 1)`).Scan(
		&view.Delivery.DuplicateTargetAnomalies,
	); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}

	if err := q.QueryRow(`SELECT COALESCE(SUM(status='parked'),0),
COALESCE(SUM(status='active'),0), COALESCE(SUM(status='dissolved'),0) FROM airs`).Scan(
		&view.Air.Parked, &view.Air.Active, &view.Air.Dissolved,
	); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	if err := q.QueryRow(`SELECT COALESCE(SUM(status='joined'),0),
COALESCE(SUM(status='pending_confirmation'),0) FROM air_members`).Scan(
		&view.Air.JoinedMembers, &view.Air.PendingMembers,
	); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	if err := q.QueryRow(`SELECT COUNT(*) FROM air_active_pointers`).Scan(
		&view.Air.ActivePointers,
	); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	if err := q.QueryRow(`SELECT COUNT(*) FROM air_active_pointers p
LEFT JOIN airs a ON a.public_id = p.air_id AND a.status = 'active'
LEFT JOIN air_members m ON m.air_id = p.air_id AND m.orbit_id = p.orbit_id
  AND m.status = 'joined'
WHERE a.public_id IS NULL OR m.public_id IS NULL`).Scan(&view.Air.RuntimeShapeAnomalies); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	if view.Features.AirRooms.Enabled && view.Air.RuntimeShapeAnomalies > 0 {
		view.Readiness.AirRuntimeReady = false
		view.Readiness.Ready = false
	}
	return view, nil
}

func phase2HealthSnapshot(q phase2ObservabilityQuerier, now int64) (Phase2HealthSnapshot, error) {
	if now <= 0 {
		return Phase2HealthSnapshot{}, ErrStreamAccountingInvalid
	}
	var view Phase2HealthSnapshot
	var streamEnabled int64
	if err := q.QueryRow(`SELECT production_selection_enabled, contract_version, revision
FROM stream_variant_policy WHERE singleton = 1`).Scan(
		&streamEnabled, &view.Features.StreamedTracks.State,
		&view.Features.StreamedTracks.Revision,
	); err != nil {
		return Phase2HealthSnapshot{}, err
	}
	view.Features.StreamedTracks.Enabled = streamEnabled == 1
	var airDivergence, airGeneration int64
	if err := q.QueryRow(`SELECT mode, generation, divergence_count
FROM air_authority WHERE singleton = 1`).Scan(
		&view.Features.AirRooms.State, &airGeneration, &airDivergence,
	); err != nil {
		return Phase2HealthSnapshot{}, err
	}
	view.Features.AirRooms.Enabled = view.Features.AirRooms.State == "airs_authoritative"
	view.Features.AirRooms.Revision = airGeneration
	view.Features.TargetsInbox = Phase2FeatureState{Enabled: true, State: "available", Revision: 1}
	accounting, err := streamAccountingSnapshot(q, now)
	if err != nil {
		return Phase2HealthSnapshot{}, err
	}
	view.Accounting = accounting
	view.Readiness = Phase2Readiness{
		StreamAccountingRequired: view.Features.StreamedTracks.Enabled,
		StreamAccountingReady:    accounting.Ready,
		MediaProcessorRequired:   view.Features.StreamedTracks.Enabled,
		MediaProcessorReady:      !view.Features.StreamedTracks.Enabled,
		StreamStorageRequired:    view.Features.StreamedTracks.Enabled,
		StreamStorageReady:       !view.Features.StreamedTracks.Enabled,
		AirRuntimeRequired:       view.Features.AirRooms.Enabled,
		AirRuntimeReady:          !view.Features.AirRooms.Enabled || airDivergence == 0,
	}
	if view.Features.AirRooms.Enabled && view.Readiness.AirRuntimeReady {
		var anomalies int64
		if err := q.QueryRow(`SELECT COUNT(*) FROM air_active_pointers p
LEFT JOIN airs a ON a.public_id = p.air_id AND a.status = 'active'
LEFT JOIN air_members m ON m.air_id = p.air_id AND m.orbit_id = p.orbit_id
  AND m.status = 'joined'
WHERE a.public_id IS NULL OR m.public_id IS NULL`).Scan(&anomalies); err != nil {
			return Phase2HealthSnapshot{}, err
		}
		view.Readiness.AirRuntimeReady = anomalies == 0
	}
	view.Readiness.Ready = (!view.Readiness.StreamAccountingRequired || view.Readiness.StreamAccountingReady) &&
		view.Readiness.MediaProcessorReady && view.Readiness.StreamStorageReady && view.Readiness.AirRuntimeReady
	return view, nil
}

func (s *Store) Phase2ObservabilitySnapshot(now int64) (Phase2ObservabilitySnapshot, error) {
	return phase2ObservabilitySnapshot(s.db, now)
}

func (s *Store) Phase2HealthSnapshot(now int64) (Phase2HealthSnapshot, error) {
	return phase2HealthSnapshot(s.db, now)
}

func (s *Store) GetAuthorizedPhase2Observability(
	operatorID, bearer string, now int64,
) (Phase2ObservabilitySnapshot, error) {
	if operatorID == "" || now <= 0 {
		return Phase2ObservabilitySnapshot{}, ErrStreamAccountingInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, bearer)
	if err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	if operator.ID != operatorID || !operator.Capabilities.List {
		return Phase2ObservabilitySnapshot{}, ErrModerationForbidden
	}
	view, err := phase2ObservabilitySnapshot(tx, now)
	if err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Phase2ObservabilitySnapshot{}, err
	}
	return view, nil
}
