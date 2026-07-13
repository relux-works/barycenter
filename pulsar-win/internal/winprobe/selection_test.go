package winprobe

import "testing"

func TestResolveInputDeterministicallySeparatesDefaultAndSelected(t *testing.T) {
	t.Parallel()
	devices := []Device{{ID: "mic-a", Name: "Desk"}, {ID: "mic-b", Name: "Headset"}}
	got, err := ResolveInput(InputDefault, "mic-a", devices, 1)
	if err != nil || got.ID != "mic-a" {
		t.Fatalf("default = %#v, %v", got, err)
	}
	got, err = ResolveInput(InputSelected, "mic-a", devices, 1)
	if err != nil || got.ID != "mic-b" {
		t.Fatalf("selected = %#v, %v", got, err)
	}
	if _, err := ResolveInput(InputSelected, "mic-a", devices, -1); err == nil {
		t.Fatal("selected mode accepted no selection")
	}
}
