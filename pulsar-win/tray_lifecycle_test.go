package main

import "testing"

func TestShouldPostTrayQuitRejectsFailedCreationDestroy(t *testing.T) {
	if shouldPostTrayQuit(0x1234, 0) {
		t.Fatal("WM_DESTROY during failed tray creation must not quit the app")
	}
}

func TestShouldPostTrayQuitAcceptsActiveTrayDestroy(t *testing.T) {
	if !shouldPostTrayQuit(0x1234, 0x1234) {
		t.Fatal("destroying the active tray window must end its message loop")
	}
	if shouldPostTrayQuit(0x9999, 0x1234) {
		t.Fatal("unrelated WM_DESTROY must not quit the tray message loop")
	}
}
