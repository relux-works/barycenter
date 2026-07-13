package winprobe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestValidationRequiresExactReviewedCapabilitySet(t *testing.T) {
	t.Parallel()
	source := filepath.Join("..", "..", "probe-msix", "AppxManifest.xml.in")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	sourceXML := strings.ReplaceAll(string(raw), "\r\n", "\n")
	tests := []struct {
		name string
		xml  string
	}{
		{name: "unexpected capability", xml: strings.Replace(sourceXML, `<DeviceCapability Name="microphone" />`, `<DeviceCapability Name="microphone" /><Capability Name="enterpriseAuthentication" />`, 1)},
		{name: "missing reviewed network capability", xml: strings.Replace(sourceXML, `    <Capability Name="internetClientServer" />`+"\n", "", 1)},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.xml == sourceXML {
				t.Fatal("manifest test fixture mutation did not change the reviewed source")
			}
			path := filepath.Join(t.TempDir(), "AppxManifest.xml")
			if err := os.WriteFile(path, []byte(tc.xml), 0o600); err != nil {
				t.Fatal(err)
			}
			manifest, err := InspectManifest(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := manifest.Validate(); err == nil {
				t.Fatal("Validate accepted a capability set that differs from the reviewed four-capability set")
			}
		})
	}
}
