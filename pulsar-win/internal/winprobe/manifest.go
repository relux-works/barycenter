package winprobe

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	ProbePackageIdentity = "ReluxWorksLLC.PulsarBarycenter"
	ProbePublisher       = "CN=60105954-A0D9-4E89-B32D-18AF2F423ABE"
	ProbeApplicationID   = "PulsarProbe"
)

type manifestPackage struct {
	Identity struct {
		Name                  string `xml:"Name,attr"`
		Publisher             string `xml:"Publisher,attr"`
		ProcessorArchitecture string `xml:"ProcessorArchitecture,attr"`
	} `xml:"Identity"`
	Applications struct {
		Application []struct {
			ID              string `xml:"Id,attr"`
			RuntimeBehavior string `xml:"RuntimeBehavior,attr"`
			TrustLevel      string `xml:"TrustLevel,attr"`
		} `xml:"Application"`
	} `xml:"Applications"`
	Capabilities struct {
		Capability       []namedCapability `xml:"Capability"`
		DeviceCapability []namedCapability `xml:"DeviceCapability"`
	} `xml:"Capabilities"`
}

type namedCapability struct {
	Name string `xml:"Name,attr"`
}

type ManifestAssertions struct {
	Path                   string
	PackageIdentity        string
	Publisher              string
	ApplicationID          string
	MicrophoneDeclared     bool
	BroadFilesystemAbsent  bool
	RunFullTrustAbsent     bool
	ProcessorArchitecture  string
	TrustLevel             string
	RuntimeBehavior        string
	UnexpectedCapabilities []string
}

func ValidatePackagePayload(files []string) error {
	want := map[string]bool{
		"AppxManifest.xml":           false,
		"pulsar-win-probe-amd64.exe": false,
		HelperDLLName:                false,
	}
	for _, file := range files {
		base := filepath.Base(filepath.Clean(file))
		if _, known := want[base]; known {
			want[base] = true
		}
	}
	for file, present := range want {
		if !present {
			return fmt.Errorf("package payload missing %s", file)
		}
	}
	return nil
}

func InspectManifest(path string) (ManifestAssertions, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ManifestAssertions{}, err
	}
	var pkg manifestPackage
	if err := xml.Unmarshal(raw, &pkg); err != nil {
		return ManifestAssertions{}, err
	}
	out := ManifestAssertions{
		Path:                  path,
		PackageIdentity:       pkg.Identity.Name,
		Publisher:             pkg.Identity.Publisher,
		BroadFilesystemAbsent: true,
		RunFullTrustAbsent:    true,
		ProcessorArchitecture: pkg.Identity.ProcessorArchitecture,
	}
	if len(pkg.Applications.Application) > 0 {
		out.ApplicationID = pkg.Applications.Application[0].ID
		out.TrustLevel = pkg.Applications.Application[0].TrustLevel
		out.RuntimeBehavior = pkg.Applications.Application[0].RuntimeBehavior
	}
	expected := map[string]bool{
		"capability:internetClient":             false,
		"capability:internetClientServer":       false,
		"capability:privateNetworkClientServer": false,
		"deviceCapability:microphone":           false,
	}
	for _, cap := range pkg.Capabilities.Capability {
		key := "capability:" + cap.Name
		if present, ok := expected[key]; ok && !present {
			expected[key] = true
		} else if ok {
			out.UnexpectedCapabilities = append(out.UnexpectedCapabilities, "duplicate:"+key)
		} else {
			out.UnexpectedCapabilities = append(out.UnexpectedCapabilities, key)
		}
		switch cap.Name {
		case "broadFileSystemAccess", "documentsLibrary", "musicLibrary", "picturesLibrary", "videosLibrary", "removableStorage":
			out.BroadFilesystemAbsent = false
			out.UnexpectedCapabilities = append(out.UnexpectedCapabilities, cap.Name)
		case "runFullTrust":
			out.RunFullTrustAbsent = false
			out.UnexpectedCapabilities = append(out.UnexpectedCapabilities, cap.Name)
		}
	}
	for _, cap := range pkg.Capabilities.DeviceCapability {
		key := "deviceCapability:" + cap.Name
		if present, ok := expected[key]; ok && !present {
			expected[key] = true
		} else if ok {
			out.UnexpectedCapabilities = append(out.UnexpectedCapabilities, "duplicate:"+key)
		} else {
			out.UnexpectedCapabilities = append(out.UnexpectedCapabilities, key)
		}
		if cap.Name == "microphone" {
			out.MicrophoneDeclared = true
		}
	}
	for capability, present := range expected {
		if !present {
			out.UnexpectedCapabilities = append(out.UnexpectedCapabilities, "missing:"+capability)
		}
	}
	sort.Strings(out.UnexpectedCapabilities)
	return out, nil
}

func (m ManifestAssertions) Validate() error {
	if m.PackageIdentity != ProbePackageIdentity {
		return fmt.Errorf("manifest package identity = %q, want current Partner Center identity %q", m.PackageIdentity, ProbePackageIdentity)
	}
	if m.Publisher != ProbePublisher {
		return fmt.Errorf("manifest publisher = %q, want current Partner Center publisher %q", m.Publisher, ProbePublisher)
	}
	if m.ApplicationID != ProbeApplicationID {
		return fmt.Errorf("manifest application ID = %q, want %q", m.ApplicationID, ProbeApplicationID)
	}
	if m.ProcessorArchitecture != "x64" {
		return fmt.Errorf("manifest processor architecture = %q, want x64", m.ProcessorArchitecture)
	}
	if m.TrustLevel != "appContainer" {
		return fmt.Errorf("manifest trust level = %q, want appContainer", m.TrustLevel)
	}
	if m.RuntimeBehavior != "packagedClassicApp" {
		return fmt.Errorf("manifest runtime behavior = %q, want packagedClassicApp", m.RuntimeBehavior)
	}
	if !m.MicrophoneDeclared {
		return fmt.Errorf("manifest missing microphone capability")
	}
	if !m.BroadFilesystemAbsent {
		return fmt.Errorf("manifest unexpectedly declares broadFileSystemAccess")
	}
	if !m.RunFullTrustAbsent {
		return fmt.Errorf("manifest unexpectedly declares runFullTrust")
	}
	if len(m.UnexpectedCapabilities) != 0 {
		return fmt.Errorf("manifest capability set differs from reviewed set: %v", m.UnexpectedCapabilities)
	}
	return nil
}
