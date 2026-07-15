//go:build windows

package main

import (
	"log/slog"
	"runtime"
	"sync"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

type windowsAudioOutputController struct {
	engine    *Engine
	player    *Player
	log       *slog.Logger
	mu        sync.RWMutex
	outputs   []WindowsAudioOutput
	selected  int
	stop      chan struct{}
	done      chan struct{}
	closed    bool
	switching bool
}

func newWindowsAudioOutputController(engine *Engine, player *Player, log *slog.Logger) WindowsAudioOutputControl {
	outputs := enumerateWindowsAudioOutputs()
	c := &windowsAudioOutputController{engine: engine, player: player, log: log, outputs: outputs}
	c.startLocked()
	return c
}

func (c *windowsAudioOutputController) Snapshot() ([]WindowsAudioOutput, int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]WindowsAudioOutput(nil), c.outputs...), c.selected
}

func (c *windowsAudioOutputController) SelectNext() {
	c.mu.Lock()
	if c.closed || c.switching || len(c.outputs) < 2 {
		c.mu.Unlock()
		return
	}
	c.switching = true
	oldStop, oldDone := c.stop, c.done
	c.stop, c.done = nil, nil
	c.selected = (c.selected + 1) % len(c.outputs)
	c.mu.Unlock()
	if oldStop != nil {
		close(oldStop)
		<-oldDone
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.switching = false
	if !c.closed {
		c.startLocked()
	}
}

func (c *windowsAudioOutputController) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	stop, done := c.stop, c.done
	c.stop, c.done = nil, nil
	c.mu.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
}

func (c *windowsAudioOutputController) startLocked() {
	stop, done := make(chan struct{}), make(chan struct{})
	c.stop, c.done = stop, done
	deviceID := ""
	if c.selected >= 0 && c.selected < len(c.outputs) {
		deviceID = c.outputs[c.selected].ID
	}
	go func() {
		defer close(done)
		if err := renderLoop(c.engine, c.player, c.log, stop, deviceID); err != nil {
			c.log.Error("WASAPI render loop failed", "err", err)
		}
	}()
}

func enumerateWindowsAudioOutputs() []WindowsAudioOutput {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return nil
	}
	defer ole.CoUninitialize()
	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		return nil
	}
	defer enumerator.Release()
	var defaultDevice *wca.IMMDevice
	defaultID := ""
	if enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &defaultDevice) == nil {
		_ = defaultDevice.GetId(&defaultID)
		defaultDevice.Release()
	}
	var collection *wca.IMMDeviceCollection
	if err := enumerator.EnumAudioEndpoints(wca.ERender, 1, &collection); err != nil {
		return nil
	}
	defer collection.Release()
	var count uint32
	if collection.GetCount(&count) != nil {
		return nil
	}
	outputs := make([]WindowsAudioOutput, 0, count)
	for index := uint32(0); index < count; index++ {
		var device *wca.IMMDevice
		if collection.Item(index, &device) != nil {
			continue
		}
		var id string
		_ = device.GetId(&id)
		name := deviceFriendlyName(device)
		device.Release()
		if id == "" {
			continue
		}
		output := WindowsAudioOutput{ID: id, Name: name}
		if output.Name == "" {
			output.Name = "Windows audio output"
		}
		if id == defaultID {
			outputs = append([]WindowsAudioOutput{output}, outputs...)
		} else {
			outputs = append(outputs, output)
		}
	}
	return outputs
}
