//go:build windows

// Windows audio legs (research report §B, spike gates U-W4):
//  1. Named-pipe server on \\.\pipe\LOCAL\… (go-winio) — the bundled
//     go-librespot fork connects and streams f32le PCM; pumpF32LE moves it
//     into the ring with backpressure (the daemon blocks on the pipe when
//     the ring is full, mirroring the macOS FIFO design rule).
//  2. WASAPI shared-mode event-driven render loop (go-wca, pure syscalls,
//     CGO_ENABLED=0) pulling from the ring; underruns render as silence.
//
// BLIND-BUILD STATUS: compiles for GOOS=windows; runtime behavior is
// untestable until a Windows machine exists (U-W4: period sizing, underrun
// budget, AUTOCONVERTPCM path, AppContainer pipe access).
package main

import (
	"fmt"
	"log/slog"
	"net"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

// startAudio wires both legs. Non-fatal renderer errors are logged by the
// goroutine; the caller decides overall policy (main: warn and continue).
func startAudio(pipeName string, ring *Ring, player *Player, log *slog.Logger, stop <-chan struct{}) error {
	listener, err := winio.ListenPipe(pipeName, nil)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", pipeName, err)
	}
	go acceptLoop(listener, ring, log, stop)
	go func() {
		<-stop
		listener.Close()
	}()
	go func() {
		if err := renderLoop(ring, player, log, stop); err != nil {
			log.Error("wasapi render loop failed", "err", err)
		}
	}()
	return nil
}

// acceptLoop serves one daemon connection at a time (the supervisor restarts
// the daemon; each incarnation reconnects to the pipe).
func acceptLoop(l net.Listener, ring *Ring, log *slog.Logger, stop <-chan struct{}) {
	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-stop:
				return
			default:
			}
			log.Warn("pipe accept failed", "err", err)
			return
		}
		log.Info("daemon connected to audio pipe")
		err = pumpF32LE(conn, ring, stop)
		conn.Close()
		if err == errPumpStopped {
			return
		}
		if err != nil {
			log.Warn("pipe reader ended", "err", err)
		} else {
			log.Info("daemon closed the audio pipe") // daemon restart: accept again
		}
	}
}

// renderLoop is the WASAPI shared-mode event-driven render pump.
//
// The client is initialized directly at the pipeline format (44100/2/f32)
// with AUTOCONVERTPCM|SRC_DEFAULT_QUALITY: since Win10 the audio engine
// converts to the device mix format, so the loop pulls raw ring floats with
// no local resampler — the macOS AVAudioEngine did the same conversion.
func renderLoop(ring *Ring, player *Player, log *slog.Logger, stop <-chan struct{}) error {
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return fmt.Errorf("CoInitializeEx: %w", err)
	}
	defer ole.CoUninitialize()

	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		return fmt.Errorf("create device enumerator: %w", err)
	}
	defer enumerator.Release()

	var device *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &device); err != nil {
		return fmt.Errorf("default render endpoint: %w", err)
	}
	defer device.Release()

	var client *wca.IAudioClient
	if err := device.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &client); err != nil {
		return fmt.Errorf("activate IAudioClient: %w", err)
	}
	defer client.Release()

	const waveFormatIEEEFloat = 0x3 // WAVE_FORMAT_IEEE_FLOAT
	format := &wca.WAVEFORMATEX{
		WFormatTag:      waveFormatIEEEFloat,
		NChannels:       channels,
		NSamplesPerSec:  sampleRate,
		WBitsPerSample:  32,
		NBlockAlign:     channels * 4,
		NAvgBytesPerSec: sampleRate * channels * 4,
		CbSize:          0,
	}

	// 100 ms engine buffer (REFERENCE_TIME is 100 ns units); the ring in
	// front of it is the real jitter absorber (ring_buffer_ms, default 1 s).
	const bufferDuration = wca.REFERENCE_TIME(100 * 10000)
	flags := uint32(wca.AUDCLNT_STREAMFLAGS_EVENTCALLBACK |
		wca.AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM |
		wca.AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY)
	if err := client.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, flags,
		bufferDuration, 0, format, nil); err != nil {
		return fmt.Errorf("IAudioClient.Initialize (44100/2/f32 autoconvert): %w", err)
	}

	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return fmt.Errorf("CreateEvent: %w", err)
	}
	defer windows.CloseHandle(event)
	if err := client.SetEventHandle(uintptr(event)); err != nil {
		return fmt.Errorf("SetEventHandle: %w", err)
	}

	var bufferFrames uint32
	if err := client.GetBufferSize(&bufferFrames); err != nil {
		return fmt.Errorf("GetBufferSize: %w", err)
	}

	var renderer *wca.IAudioRenderClient
	if err := client.GetService(wca.IID_IAudioRenderClient, &renderer); err != nil {
		return fmt.Errorf("get IAudioRenderClient: %w", err)
	}
	defer renderer.Release()

	if err := client.Start(); err != nil {
		return fmt.Errorf("IAudioClient.Start: %w", err)
	}
	defer client.Stop()
	log.Info("wasapi render started", "buffer_frames", bufferFrames)

	for {
		select {
		case <-stop:
			return nil
		default:
		}
		ret, err := windows.WaitForSingleObject(event, 2000)
		if err != nil {
			return fmt.Errorf("WaitForSingleObject: %w", err)
		}
		if ret != windows.WAIT_OBJECT_0 {
			continue // timeout: loop back to the stop check
		}

		var padding uint32
		if err := client.GetCurrentPadding(&padding); err != nil {
			return fmt.Errorf("GetCurrentPadding: %w", err)
		}
		frames := bufferFrames - padding
		if frames == 0 {
			continue
		}

		var data *byte
		if err := renderer.GetBuffer(frames, &data); err != nil {
			return fmt.Errorf("GetBuffer: %w", err)
		}
		dst := unsafe.Slice((*float32)(unsafe.Pointer(data)), int(frames)*channels)
		got := ring.Read(dst)
		for i := got; i < len(dst); i++ {
			dst[i] = 0 // underrun = silence, never a stale tail
		}
		if got < len(dst) {
			player.NoteStarved()
		}
		player.NoteRendered(got)
		if err := renderer.ReleaseBuffer(frames, 0); err != nil {
			return fmt.Errorf("ReleaseBuffer: %w", err)
		}
	}
}
