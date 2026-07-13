package winprobe

import "testing"

func TestSelectLoader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		code    uint32
		choice  LoaderChoice
		wantErr bool
	}{
		{name: "packaged", code: 0, choice: LoaderPackaged},
		{name: "unpackaged exact fallback", code: AppModelErrorNoPackage, choice: LoaderExecutableDir},
		{name: "module missing is terminal", code: 126, wantErr: true},
		{name: "access denied is terminal", code: 5, wantErr: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := SelectLoader(tc.code)
			if (err != nil) != tc.wantErr {
				t.Fatalf("SelectLoader(%d) error = %v, wantErr=%v", tc.code, err, tc.wantErr)
			}
			if got != tc.choice {
				t.Fatalf("SelectLoader(%d) = %q, want %q", tc.code, got, tc.choice)
			}
		})
	}
}

func TestProbeDiagnosticsExtensionNegotiatesSeparatelyFromCoreABI(t *testing.T) {
	t.Parallel()
	if HelperABIVersion != 1 {
		t.Fatalf("frozen core ABI = %d, want 1", HelperABIVersion)
	}
	tests := []struct {
		name       string
		symbols    ProbeDiagnosticsSymbols
		version    uint32
		structSize uint32
		wantErr    bool
	}{
		{name: "missing version export", symbols: ProbeDiagnosticsSymbols{Query: true}, version: 1, structSize: ProbeCaptureDiagnosticsV1StructSize, wantErr: true},
		{name: "missing query export", symbols: ProbeDiagnosticsSymbols{Version: true}, version: 1, structSize: ProbeCaptureDiagnosticsV1StructSize, wantErr: true},
		{name: "wrong extension version", symbols: ProbeDiagnosticsSymbols{Version: true, Query: true}, version: 2, structSize: ProbeCaptureDiagnosticsV1StructSize, wantErr: true},
		{name: "wrong extension struct size", symbols: ProbeDiagnosticsSymbols{Version: true, Query: true}, version: 1, structSize: ProbeCaptureDiagnosticsV1StructSize + 4, wantErr: true},
		{name: "matching private extension", symbols: ProbeDiagnosticsSymbols{Version: true, Query: true}, version: 1, structSize: ProbeCaptureDiagnosticsV1StructSize},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateProbeDiagnosticsExtension(tc.symbols, tc.version, tc.structSize)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateProbeDiagnosticsExtension() error = %v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
