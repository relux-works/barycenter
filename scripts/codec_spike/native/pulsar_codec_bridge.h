#ifndef PULSAR_CODEC_BRIDGE_H
#define PULSAR_CODEC_BRIDGE_H

#include <stdint.h>

#if defined(_WIN32) && defined(PULSAR_CODEC_BUILD)
#define PULSAR_CODEC_EXPORT __declspec(dllexport)
#elif defined(_WIN32)
#define PULSAR_CODEC_EXPORT __declspec(dllimport)
#else
#define PULSAR_CODEC_EXPORT __attribute__((visibility("default")))
#endif

typedef struct PulsarCodecProbeResult {
    char codec[16];
    int sample_rate;
    int channels;
    int64_t frames;
    int64_t samples;
    int64_t first_pts_ms;
    int64_t last_pts_ms;
    uint64_t pcm_checksum;
    int drained;
    int cancelled;
    int error_code;
} PulsarCodecProbeResult;

PULSAR_CODEC_EXPORT unsigned pulsar_codec_bridge_abi(void);
PULSAR_CODEC_EXPORT int pulsar_codec_probe_file(
    const char *path,
    int64_t seek_ms,
    int64_t cancel_after_frames,
    PulsarCodecProbeResult *result
);

#endif
