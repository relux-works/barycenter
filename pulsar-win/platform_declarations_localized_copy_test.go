package main

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type platformCopyContract map[string]map[string]string

func TestPlatformCopyContractIsConsumedByBothWindowsLocales(t *testing.T) {
	contract := readPlatformCopyContract(t)
	bindings := map[string]struct {
		locale ShellLocale
		keys   map[string]shellText
	}{
		"en": {ShellEnglish, map[string]shellText{
			"create": txtCreate, "join": txtJoin, "try_locally": txtTry,
			"routing": txtRouting, "history": txtHistory, "report": txtReport,
			"integrations": txtIntegrations, "spotify_optional": txtSpotifyOptional,
			"telegram_optional": txtTelegramOptional,
		}},
		"ru": {ShellRussian, map[string]shellText{
			"create": txtCreate, "join": txtJoin, "try_locally": txtTry,
			"routing": txtRouting, "history": txtHistory, "report": txtReport,
			"integrations": txtIntegrations, "spotify_optional": txtSpotifyOptional,
			"telegram_optional": txtTelegramOptional,
		}},
	}
	for language, binding := range bindings {
		copy := NewShellCopy(binding.locale)
		for contractKey, shellKey := range binding.keys {
			if got, want := copy.Text(shellKey), contract[language][contractKey]; got != want {
				t.Errorf("%s %s = %q, want canonical %q", language, contractKey, got, want)
			}
		}
	}
}

func TestProductionMSIXHasLocalizedResourcesAndReviewedCapabilities(t *testing.T) {
	contract := readPlatformCopyContract(t)
	manifest := mustReadPlatformFile(t, filepath.Join("msix", "AppxManifest.xml.in"))
	for _, ref := range []string{"ms-resource:AppDisplayName", "ms-resource:AppDescription"} {
		if !strings.Contains(manifest, ref) {
			t.Errorf("production manifest missing %s", ref)
		}
	}
	if strings.Contains(manifest, "Общий музыкальный эфир") {
		t.Fatal("production manifest still embeds a single-language legacy description")
	}

	type capability struct {
		Name string `xml:"Name,attr"`
	}
	var parsed struct {
		Capabilities struct {
			Capability       []capability `xml:"Capability"`
			DeviceCapability []capability `xml:"DeviceCapability"`
		} `xml:"Capabilities"`
	}
	if err := xml.Unmarshal([]byte(manifest), &parsed); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range parsed.Capabilities.Capability {
		got = append(got, "capability:"+item.Name)
	}
	for _, item := range parsed.Capabilities.DeviceCapability {
		got = append(got, "deviceCapability:"+item.Name)
	}
	sort.Strings(got)
	want := []string{
		"capability:internetClient", "capability:internetClientServer",
		"capability:privateNetworkClientServer", "deviceCapability:microphone",
	}
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("production capability set = %v, want reviewed set %v", got, want)
	}

	for language, directory := range map[string]string{"en": "en-US", "ru": "ru-RU"} {
		resource := mustReadPlatformFile(t, filepath.Join("msix", "Strings", directory, "Resources.resw"))
		if !strings.Contains(resource, ">"+contract[language]["app_description"]+"</value>") {
			t.Errorf("%s MSIX description does not match canonical copy", language)
		}
	}
	workflow := mustReadPlatformFile(t, filepath.Join("..", ".github", "workflows", "release.yml"))
	for _, required := range []string{"cp -R pulsar-win/msix/Strings stage/Strings", "makepri createconfig", "makepri new"} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow missing localized resource step %q", required)
		}
	}
	ci := mustReadPlatformFile(t, filepath.Join("..", ".github", "workflows", "ci.yml"))
	for _, required := range []string{
		"Compile and pack production localized MSIX schema",
		"Pulsar-production-schema.msix", "makeappx schema validation",
	} {
		if !strings.Contains(ci, required) {
			t.Errorf("hosted package validation missing %q", required)
		}
	}
}

func TestMacPrivacyCopyIsLocalizedAndOptional(t *testing.T) {
	contract := readPlatformCopyContract(t)
	for language := range contract {
		resource := mustReadPlatformFile(t, filepath.Join("..", "assets", "macos", language+".lproj", "InfoPlist.strings"))
		for _, key := range []string{"microphone_usage", "local_network_usage", "apple_events_usage"} {
			if !strings.Contains(resource, contract[language][key]) {
				t.Errorf("%s InfoPlist.strings missing canonical %s", language, key)
			}
		}
	}
	build := mustReadPlatformFile(t, filepath.Join("..", "scripts", "build-app.sh"))
	for _, required := range []string{
		"NSMicrophoneUsageDescription", "NSLocalNetworkUsageDescription",
		"NSAppleEventsUsageDescription", "assets/macos/${locale}.lproj/InfoPlist.strings",
	} {
		if !strings.Contains(build, required) {
			t.Errorf("macOS bundle contract missing %q", required)
		}
	}
	for _, forbidden := range []string{"com.apple.security.personal-information.microphone", "com.apple.security.automation.apple-events"} {
		if strings.Contains(build, forbidden) {
			t.Errorf("unreviewed sandbox entitlement slipped into build script: %s", forbidden)
		}
	}
}

func TestLegacyCompanionFlowsStayOptionalAndNeverAutoOpenSpotifyHelp(t *testing.T) {
	windowsCopy := mustReadPlatformFile(t, "ui_common.go")
	macOnboarding := mustReadPlatformFile(t, filepath.Join("..", "node-app", "Sources", "NodeApp", "OnboardingWindow.swift"))
	macMain := mustReadPlatformFile(t, filepath.Join("..", "node-app", "Sources", "NodeApp", "main.swift"))
	for name, source := range map[string]string{"Windows": windowsCopy, "macOS": macOnboarding} {
		if !strings.Contains(source, "необязательн") && !strings.Contains(source, "Необязательн") {
			t.Errorf("%s legacy companion path does not identify optional integrations", name)
		}
		if strings.Contains(source, "Готово! Остался один шаг") {
			t.Errorf("%s retains the mandatory Spotify completion gate", name)
		}
	}
	if strings.Contains(macMain, "SpotifyHelp.presentHowToSound()") {
		t.Fatal("macOS automatically presents optional Spotify help after pairing")
	}
}

func readPlatformCopyContract(t *testing.T) platformCopyContract {
	t.Helper()
	raw := mustReadPlatformFile(t, filepath.Join("..", "assets", "localization", "platform-copy.json"))
	var contract platformCopyContract
	if err := json.Unmarshal([]byte(raw), &contract); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"en", "ru"} {
		if len(contract[language]) == 0 {
			t.Fatalf("canonical copy missing %s", language)
		}
	}
	return contract
}

func mustReadPlatformFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
