package winprobe

import "fmt"

const (
	HelperDLLName          = "pulsar-capture.dll"
	AppModelErrorNoPackage = uint32(15700)

	ProbeDiagnosticsVersionSymbol = "PulsarProbeDiagnosticsGetVersion"
	ProbeDiagnosticsQuerySymbol   = "PulsarProbeCaptureGetDiagnosticsV1"
)

type ProbeDiagnosticsSymbols struct {
	Version bool
	Query   bool
}

// ValidateProbeDiagnosticsExtension negotiates the private probe evidence
// extension independently of CapGetVersion. A core ABI-v1 DLL is valid Rev16,
// but the packaged probe additionally requires this separately named contract.
func ValidateProbeDiagnosticsExtension(symbols ProbeDiagnosticsSymbols, version, structSize uint32) error {
	if !symbols.Version {
		return fmt.Errorf("private probe diagnostics extension is missing %s", ProbeDiagnosticsVersionSymbol)
	}
	if !symbols.Query {
		return fmt.Errorf("private probe diagnostics extension is missing %s", ProbeDiagnosticsQuerySymbol)
	}
	if version != ProbeDiagnosticsExtensionVersion {
		return fmt.Errorf("private probe diagnostics extension version %d, want %d", version, ProbeDiagnosticsExtensionVersion)
	}
	if structSize != ProbeCaptureDiagnosticsV1StructSize {
		return fmt.Errorf("private probe diagnostics v1 struct size %d, want %d", structSize, ProbeCaptureDiagnosticsV1StructSize)
	}
	return nil
}

type LoaderChoice string

const (
	LoaderPackaged      LoaderChoice = "LoadPackagedLibrary"
	LoaderExecutableDir LoaderChoice = "LoadLibraryExW(executable-directory)"
)

// SelectLoader freezes the only permitted fallback: an unpackaged process may
// load an absolute executable-directory path. Every other packaged-loader
// error is terminal and must be logged as fail/blocked evidence.
func SelectLoader(packagedError uint32) (LoaderChoice, error) {
	if packagedError == 0 {
		return LoaderPackaged, nil
	}
	if packagedError == AppModelErrorNoPackage {
		return LoaderExecutableDir, nil
	}
	return "", fmt.Errorf("LoadPackagedLibrary failed with win32=0x%08x", packagedError)
}
