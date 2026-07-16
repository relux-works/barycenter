package store

import (
	"os"
	"testing"
)

func TestAutomationPreviousHeadAuthority(t *testing.T) {
	path := os.Getenv("BARYCENTER_AUTOMATION_PREVIOUS_DB")
	if path == "" {
		t.Skip("automation previous-head driver")
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting("automation_previous_head_probe", "written"); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}
