#include "pulsar_codec_bridge.h"

#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#if defined(_WIN32)
#include <windows.h>
#include <psapi.h>
#else
#include <sys/resource.h>
#endif

static uint64_t peak_rss_bytes(void) {
#if defined(_WIN32)
    PROCESS_MEMORY_COUNTERS counters;
    memset(&counters, 0, sizeof(counters));
    counters.cb = sizeof(counters);
    if (GetProcessMemoryInfo(GetCurrentProcess(), &counters, sizeof(counters)))
        return (uint64_t)counters.PeakWorkingSetSize;
    return 0;
#else
    struct rusage usage;
    if (getrusage(RUSAGE_SELF, &usage) != 0)
        return 0;
#if defined(__APPLE__)
    return (uint64_t)usage.ru_maxrss;
#else
    return (uint64_t)usage.ru_maxrss * UINT64_C(1024);
#endif
#endif
}

static void usage(const char *program) {
    fprintf(stderr, "usage: %s <media> [--seek-ms N] [--cancel-after-frames N]\n", program);
}

int main(int argc, char **argv) {
    PulsarCodecProbeResult result;
    int64_t seek_ms = -1;
    int64_t cancel_after_frames = 0;
    int index;
    int status;
    clock_t started;
    uint64_t cpu_ms;
    uint64_t rss_bytes;
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
    started = clock();
    status = pulsar_codec_probe_file(argv[1], seek_ms, cancel_after_frames, &result);
    cpu_ms = (uint64_t)(((double)(clock() - started) * 1000.0) / CLOCKS_PER_SEC);
    rss_bytes = peak_rss_bytes();
    printf("{\"abi\":%u,\"codec\":\"%s\",\"sampleRate\":%d,\"channels\":%d,"
           "\"frames\":%" PRId64 ",\"samples\":%" PRId64 ",\"firstPtsMS\":%" PRId64 ","
           "\"lastPtsMS\":%" PRId64 ",\"pcmChecksum\":\"%016" PRIx64 "\","
           "\"drained\":%s,\"cancelled\":%s,\"cpuMS\":%" PRIu64 ","
           "\"peakRSSBytes\":%" PRIu64 ",\"errorCode\":%d}\n",
           pulsar_codec_bridge_abi(), result.codec, result.sample_rate, result.channels,
           result.frames, result.samples, result.first_pts_ms, result.last_pts_ms,
           result.pcm_checksum, result.drained ? "true" : "false",
           result.cancelled ? "true" : "false", cpu_ms, rss_bytes, result.error_code);
    return status < 0 ? 1 : 0;
}
