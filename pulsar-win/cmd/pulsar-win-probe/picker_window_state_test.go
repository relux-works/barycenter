package main

import "testing"

func TestPickerWindowStateRestorationTruthTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		wasHidden  bool
		initiated  bool
		asyncDone  bool
		wantOnInit bool
		wantOnDone bool
	}{
		{name: "visible synchronous failure"},
		{name: "hidden owner restore failure", wasHidden: true, wantOnInit: true},
		{name: "hidden picker open failure", wasHidden: true, wantOnInit: true},
		{name: "visible asynchronous terminal", initiated: true, asyncDone: true},
		{name: "hidden picked", wasHidden: true, initiated: true, asyncDone: true, wantOnDone: true},
		{name: "hidden cancelled or failed", wasHidden: true, initiated: true, asyncDone: true, wantOnDone: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var state pickerWindowState
			state.begin(tc.wasHidden)
			if got := state.initiated(tc.initiated); got != tc.wantOnInit {
				t.Fatalf("initiated() rehide = %v, want %v", got, tc.wantOnInit)
			}
			if tc.asyncDone {
				if got := state.complete(); got != tc.wantOnDone {
					t.Fatalf("complete() rehide = %v, want %v", got, tc.wantOnDone)
				}
			}
			if state.complete() {
				t.Fatal("restoration repeated after state was consumed")
			}
		})
	}
}
