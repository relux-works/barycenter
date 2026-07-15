#include "pulsar_codec_bridge.h"

#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static void usage(const char *program) {
    fprintf(stderr, "usage: %s <media> [--seek-ms N] [--cancel-after-frames N]\n", program);
}

int main(int argc, char **argv) {
    PulsarCodecProbeResult result;
    int64_t seek_ms = -1;
    int64_t cancel_after_frames = 0;
    int index;
    int status;
    if (argc < 2) {
        usage(argv[0]);
        return 2;
    }
    for (index = 2; index < argc; ++index) {
        if (strcmp(argv[index], "--seek-ms") == 0 && index + 1 < argc)
            seek_ms = strtoll(argv[++index], NULL, 10);
        else if (strcmp(argv[index], "--cancel-after-frames") == 0 && index + 1 < argc)
            cancel_after_frames = strtoll(argv[++index], NULL, 10);
        else {
            usage(argv[0]);
            return 2;
        }
    }
    status = pulsar_codec_probe_file(argv[1], seek_ms, cancel_after_frames, &result);
    printf("{\"abi\":%u,\"codec\":\"%s\",\"sampleRate\":%d,\"channels\":%d,"
           "\"frames\":%" PRId64 ",\"samples\":%" PRId64 ",\"firstPtsMS\":%" PRId64 ","
           "\"lastPtsMS\":%" PRId64 ",\"pcmChecksum\":\"%016" PRIx64 "\","
           "\"drained\":%s,\"cancelled\":%s,\"errorCode\":%d}\n",
           pulsar_codec_bridge_abi(), result.codec, result.sample_rate, result.channels,
           result.frames, result.samples, result.first_pts_ms, result.last_pts_ms,
           result.pcm_checksum, result.drained ? "true" : "false",
           result.cancelled ? "true" : "false", result.error_code);
    return status < 0 ? 1 : 0;
}
