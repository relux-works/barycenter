package store

import (
	"os"
	"testing"
)

func TestModerationPreviousHeadAuthority(t *testing.T) {
	path := os.Getenv("BARYCENTER_MODERATION_PREVIOUS_DB")
	if path == "" {
		t.Skip("moderation previous-head driver")
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting("moderation_previous_head_probe", "written"); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}
