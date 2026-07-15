package main

type WindowsAudioOutput struct{ ID, Name string }

type WindowsAudioOutputControl interface {
	Snapshot() ([]WindowsAudioOutput, int)
	SelectNext()
	Close()
}
