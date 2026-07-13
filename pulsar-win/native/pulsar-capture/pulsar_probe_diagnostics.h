#ifndef PULSAR_PROBE_DIAGNOSTICS_H_
#define PULSAR_PROBE_DIAGNOSTICS_H_

#include "pulsar_capture.h"

// Private packaged-probe evidence extension. This is not part of the frozen
// Rev16 core ABI. Its symbols, version, and wire struct are independently
// named and negotiated; CapGetVersion continues to negotiate core ABI v1 only.
enum PulsarProbeDiagnosticsExtensionVersion {
    PULSAR_PROBE_DIAGNOSTICS_EXTENSION_V1 = 1,
};

typedef struct PulsarProbeCaptureDiagnosticsV1 {
    uint32_t structSize;
    uint32_t version;
    uint32_t timestampErrorCount;
    HRESULT cleanupReleaseBufferHResult;
    HRESULT cleanupStopHResult;
} PulsarProbeCaptureDiagnosticsV1;

#if defined(__cplusplus)
static_assert(sizeof(PulsarProbeCaptureDiagnosticsV1) == 5u * sizeof(uint32_t),
              "private probe diagnostics v1 wire size changed");
#endif

extern "C" {

PULSAR_CAPTURE_API HRESULT __stdcall PulsarProbeDiagnosticsGetVersion(
    uint32_t* version,
    uint32_t* structSize);

PULSAR_CAPTURE_API HRESULT __stdcall PulsarProbeCaptureGetDiagnosticsV1(
    uint32_t opId,
    PulsarProbeCaptureDiagnosticsV1* diagnostics);

}

#endif
