# Select and prove the live codec and transport profile

## Description
Measure an exact cross-platform encoder, decoder, binary framing and relay transport before the wire contract freezes assumptions.

## Scope
Evaluate pinned low-latency codec candidates, normally Opus-capable, and current binary WebSocket relay versus any viable documented datagram transport under Store, firewall and deployment constraints. Freeze sample rate and channels, 10 or 20 ms frame duration, bitrate, complexity, DTX, FEC or PLC, max frame and session rates, CPU, memory, license, SBOM and package obligations. Measure encode or decode latency, mouth-to-ear budget, 2 percent loss, jitter and head-of-line behavior, backpressure and slow recipient isolation on Windows and macOS. Prove audio remains intelligible with an agreed objective or blinded human method and publish no-go if no profile can meet C2.

## Acceptance Criteria
One source-cited decision names exact versions, framing and transport with artifacts showing bounded CPU or memory and a credible p50 800 ms and p95 1500 ms end-to-end budget under 2 percent loss. It defines drop or retransmit, FEC or PLC and packet-size rules, works in signed packages with no first-run code download, and either passes licensing and deployment gates or blocks live PTT.
