package main

import (
	"errors"
	"net/http"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

type contentPolicyResponse struct {
	Contract             string `json:"contract"`
	Version              string `json:"version"`
	PolicyHash           string `json:"policy_hash"`
	Locale               string `json:"locale"`
	LocaleHash           string `json:"locale_hash"`
	EffectiveAt          string `json:"effective_at"`
	TermsURL             string `json:"terms_url"`
	ContentGuidelinesURL string `json:"content_guidelines_url"`
	Title                string `json:"title"`
	RightsText           string `json:"rights_text"`
	ConsentText          string `json:"consent_text"`
	ControllingLanguage  string `json:"controlling_language"`
}

type contentPolicyGrantResponse struct {
	Contract      string `json:"contract"`
	Version       string `json:"version"`
	PolicyHash    string `json:"policy_hash"`
	Locale        string `json:"locale"`
	AcceptedAt    string `json:"accepted_at"`
	RevokedAt     string `json:"revoked_at,omitempty"`
	Revision      int64  `json:"revision"`
	Current       bool   `json:"current"`
	TermsAccepted bool   `json:"terms_accepted"`
}

func contentPolicyManifestResponse(value store.ContentPolicyManifest) contentPolicyResponse {
	return contentPolicyResponse{
		Contract: "p2-content-policy-consent.v1", Version: value.Version,
		PolicyHash: value.Hash, Locale: string(value.Locale), LocaleHash: value.LocaleHash,
		EffectiveAt: time.UnixMilli(value.EffectiveAt).UTC().Format(time.RFC3339),
		TermsURL:    value.TermsURL, ContentGuidelinesURL: value.ContentGuidelinesURL,
		Title: value.Title, RightsText: value.RightsText, ConsentText: value.ConsentText,
		ControllingLanguage: value.ControllingLanguage,
	}
}

func contentPolicyGrantJSON(value store.ContentPolicyGrant) contentPolicyGrantResponse {
	response := contentPolicyGrantResponse{
		Contract: "p2-content-policy-consent.v1", Version: value.Version,
		PolicyHash: value.PolicyHash, Locale: string(value.Locale),
		AcceptedAt: time.UnixMilli(value.AcceptedAt).UTC().Format(time.RFC3339),
		Revision:   value.Revision, Current: value.Current,
		TermsAccepted: value.TermsAccepted,
	}
	if value.RevokedAt > 0 {
		response.RevokedAt = time.UnixMilli(value.RevokedAt).UTC().Format(time.RFC3339)
	}
	return response
}

func contentPolicyLocale(r *http.Request) (store.ContentPolicyLocale, bool) {
	values, exists := r.URL.Query()["locale"]
	if !exists || len(values) != 1 || (values[0] != "en" && values[0] != "ru") {
		return "", false
	}
	return store.ContentPolicyLocale(values[0]), true
}

func (api *onboardingAPI) contentPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/v1/content-policy" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	locale, ok := contentPolicyLocale(r)
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	manifest, err := store.CurrentContentPolicy(locale)
	if err != nil {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	writeJSON(w, http.StatusOK, contentPolicyManifestResponse(manifest))
}

func (api *onboardingAPI) contentPolicyAcceptance(w http.ResponseWriter, r *http.Request) {
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	identity := store.Identity{Kind: store.IdentityBearer, Token: actor.Bearer}
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path != "/v1/content-policy/acceptance" || r.URL.RawQuery != "" {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		grant, err := api.store.RequireCurrentContentPolicy(
			actor.Context.ActorID, identity,
		)
		if api.contentPolicyError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, contentPolicyGrantJSON(grant))
	case http.MethodPut:
		if r.URL.Path != "/v1/content-policy/acceptance" || r.URL.RawQuery != "" {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		var request struct {
			Version       string `json:"version"`
			PolicyHash    string `json:"policy_hash"`
			Locale        string `json:"locale"`
			TermsAccepted bool   `json:"terms_accepted"`
		}
		if !decodeStrictJSON(w, r, 1024, &request) || !request.TermsAccepted {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		grant, err := api.store.AcceptContentPolicy(store.AcceptContentPolicyParams{
			ExpectedActorID: actor.Context.ActorID, Identity: identity,
			Version: request.Version, PolicyHash: request.PolicyHash,
			Locale:     store.ContentPolicyLocale(request.Locale),
			AcceptedAt: api.contentPolicyNow().UnixMilli(),
		})
		if api.contentPolicyError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, contentPolicyGrantJSON(grant))
	case http.MethodDelete:
		if r.URL.Path != "/v1/content-policy/acceptance" {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		locale, ok := contentPolicyLocale(r)
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		grant, err := api.store.RevokeContentPolicy(
			actor.Context.ActorID, identity, locale, api.contentPolicyNow().UnixMilli(),
		)
		if api.contentPolicyError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, contentPolicyGrantJSON(grant))
	default:
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	}
}

func (api *onboardingAPI) contentPolicyError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var rate *store.ContentPolicyRateLimitError
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
	case errors.Is(err, store.ErrInsufficientCapability), errors.Is(err, store.ErrOrbitDisabled):
		apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
	case errors.Is(err, store.ErrContentPolicyAcceptanceRequired):
		apiError(w, http.StatusPreconditionRequired, errorContentPolicyAcceptance, 0)
	case errors.As(err, &rate):
		apiError(w, http.StatusTooManyRequests, errorTooManyAttempts, rate.RetryAfter)
	case errors.Is(err, store.ErrContentPolicyInvalid):
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	default:
		api.log.Error("content policy mutation", "err", err)
		apiError(w, http.StatusInternalServerError, errorInternal, 0)
	}
	return true
}
