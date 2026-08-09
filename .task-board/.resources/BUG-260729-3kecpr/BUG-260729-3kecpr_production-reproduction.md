# Production reproduction — 2026-07-29

Installed candidate: /Applications/Pulsar.app 0.3.0 (958.2), NodeApp SHA-256 4efb3069e4ac04ec45c72827a1b8a38d399ba016a2846dc952cd1591ac9e98a2, PID 90081.

Owner pressed Record on a MacBook Pro and saw an initial quality/system-microphone objection, followed by: Recording could not start safely. Check the microphone and output route, then try again.

System evidence at the same time: MacBook Pro Microphone remained Default Input, one channel, 48 kHz; MacBook Pro Speakers remained Default Output, two channels, 44.1 kHz. Persisted captureInputDevice.v1 was numeric AudioDeviceID 92.

CoreAudio receipt around 17:29:31: VPIO/default-duplex processing was enabled for physical input 92 and output 85; transient aggregate device 429 disappeared. CoreAudio logged AudioDeviceSetProperty/AudioObjectGetPropertyData no device with given ID, DDAgg stream-description failures, AVAudioEngine input hw format invalid / config change pending, and engine start OSStatus 560227702 (hex 0x21646576, fourcc !dev). The production recovery policy recognizes only status 35 or layout churn, so this !dev engine failure bypassed retry and consented fallback. No draft was produced.

Architecture direction: ordinary asynchronous clip capture is not full duplex and must not use VPIO/output aggregate. Use microphone-only input capture, stable device identity, one-button Record to Stop to draft, and accurate input-only failures. Keep strict VPIO/AEC policy only for actual simultaneous input/output workflows. Do not solve this by merely adding !dev to the existing narrow retry allowlist or by asking the user to select headphones/limited quality.