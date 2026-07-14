package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestPublicPolicyLinksUseStableRussianHTTPSRoutes(t *testing.T) {
	if len(uiPublicPolicyURLs) != 5 {
		t.Fatalf("policy URL count=%d, want 5", len(uiPublicPolicyURLs))
	}
	seen := make(map[string]struct{}, len(uiPublicPolicyURLs))
	for _, raw := range uiPublicPolicyURLs {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Scheme != "https" || parsed.Host != "barycenter.live" || parsed.User != nil ||
			parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasPrefix(parsed.Path, "/legal/") ||
			!strings.HasSuffix(parsed.Path, "/ru") {
			t.Fatalf("unsafe or unstable policy URL %q", raw)
		}
		if _, duplicate := seen[raw]; duplicate {
			t.Fatalf("duplicate policy URL %q", raw)
		}
		seen[raw] = struct{}{}
	}
}
