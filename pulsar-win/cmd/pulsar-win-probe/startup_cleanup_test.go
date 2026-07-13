package main

import (
	"fmt"
	"reflect"
	"testing"
)

func TestStartupRollbackCoversEveryPartialInitializationStage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		helper          bool
		destroySucceeds bool
		events          int
		windows         int
		want            []string
	}{
		{name: "log only", want: []string{"log"}},
		{name: "helper initialized", helper: true, destroySucceeds: true, want: []string{"helper", "log"}},
		{name: "first event failure", helper: true, destroySucceeds: true, events: 1, want: []string{"helper", "event-0", "log"}},
		{name: "later event failure", helper: true, destroySucceeds: true, events: 4, want: []string{"helper", "event-0", "event-1", "event-2", "event-3", "log"}},
		{name: "window creation failure", helper: true, destroySucceeds: true, events: 2, windows: 1, want: []string{"helper", "window-0", "event-0", "event-1", "log"}},
		{name: "destroy refusal preserves events", helper: true, destroySucceeds: false, events: 2, windows: 1, want: []string{"helper", "window-0", "log"}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			var destroy func() bool
			if tc.helper {
				destroy = func() bool { got = append(got, "helper"); return tc.destroySucceeds }
			}
			windows := make([]func(), tc.windows)
			for i := range windows {
				i := i
				windows[i] = func() { got = append(got, fmt.Sprintf("window-%d", i)) }
			}
			events := make([]func(), tc.events)
			for i := range events {
				i := i
				events[i] = func() { got = append(got, fmt.Sprintf("event-%d", i)) }
			}
			runStartupCleanup(destroy, windows, events, func() { got = append(got, "log") })
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("cleanup order = %v, want %v", got, tc.want)
			}
		})
	}
}
