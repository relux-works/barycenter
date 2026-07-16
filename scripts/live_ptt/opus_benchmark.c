#include <errno.h>
#include <math.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include <opus.h>

#define SAMPLE_RATE 48000
#define CHANNELS 1
#define BITRATE 24000
#define COMPLEXITY 5
#define EXPECTED_LOSS_PERCENT 2
#define MAX_PACKET_BYTES 400

static double monotonic_seconds(void) {
    struct timespec value;
    if (clock_gettime(CLOCK_MONOTONIC, &value) != 0) {
        perror("clock_gettime");
        exit(2);
    }
    return (double)value.tv_sec + (double)value.tv_nsec / 1000000000.0;
}

static int compare_int(const void *left, const void *right) {
    const int a = *(const int *)left;
    const int b = *(const int *)right;
    return (a > b) - (a < b);
}

static int percentile(const int *values, int count, double probability) {
    int *copy = malloc((size_t)count * sizeof(*copy));
    if (copy == NULL) {
        fprintf(stderr, "allocation failed\n");
        exit(2);
    }
    memcpy(copy, values, (size_t)count * sizeof(*copy));
    qsort(copy, (size_t)count, sizeof(*copy), compare_int);
    int index = (int)ceil(probability * (double)count) - 1;
    if (index < 0) index = 0;
    if (index >= count) index = count - 1;
    int result = copy[index];
    free(copy);
    return result;
}

static void require_opus(int code, const char *operation) {
    if (code != OPUS_OK) {
        fprintf(stderr, "%s: %s\n", operation, opus_strerror(code));
        exit(2);
    }
}

static void fill_signal(opus_int16 *pcm, int samples, uint64_t frame_index) {
    uint32_t noise = (uint32_t)(frame_index + 1U) * 2654435761U;
    for (int index = 0; index < samples; index++) {
        const double t = ((double)(frame_index * (uint64_t)samples) + index) / SAMPLE_RATE;
        noise = noise * 1664525U + 1013904223U;
        const double shaped_noise = ((double)((noise >> 16) & 0xffffU) / 32768.0 - 1.0) * 900.0;
        const double envelope = 0.55 + 0.35 * sin(2.0 * M_PI * 3.0 * t);
        const double voice = envelope * (6500.0 * sin(2.0 * M_PI * 180.0 * t) +
                                         2400.0 * sin(2.0 * M_PI * 360.0 * t));
        double sample = voice + shaped_noise;
        if (sample > 32767.0) sample = 32767.0;
        if (sample < -32768.0) sample = -32768.0;
        pcm[index] = (opus_int16)sample;
    }
}

int main(int argc, char **argv) {
    int frame_ms = 20;
    int frames = 12000;
    for (int index = 1; index < argc; index++) {
        if (strcmp(argv[index], "--frame-ms") == 0 && index + 1 < argc) {
            frame_ms = atoi(argv[++index]);
        } else if (strcmp(argv[index], "--frames") == 0 && index + 1 < argc) {
            frames = atoi(argv[++index]);
        } else {
            fprintf(stderr, "usage: %s [--frame-ms 10|20] [--frames N]\n", argv[0]);
            return 2;
        }
    }
    if ((frame_ms != 10 && frame_ms != 20) || frames < 1000) {
        fprintf(stderr, "frame-ms must be 10 or 20 and frames must be at least 1000\n");
        return 2;
    }
    if (strcmp(opus_get_version_string(), "libopus 1.6.1") != 0) {
        fprintf(stderr, "expected libopus 1.6.1, found %s\n", opus_get_version_string());
        return 2;
    }

    int error = OPUS_OK;
    OpusEncoder *encoder = opus_encoder_create(SAMPLE_RATE, CHANNELS, OPUS_APPLICATION_VOIP, &error);
    require_opus(error, "opus_encoder_create");
    OpusDecoder *decoder = opus_decoder_create(SAMPLE_RATE, CHANNELS, &error);
    require_opus(error, "opus_decoder_create");

    require_opus(opus_encoder_ctl(encoder, OPUS_SET_BITRATE(BITRATE)), "OPUS_SET_BITRATE");
    require_opus(opus_encoder_ctl(encoder, OPUS_SET_VBR(1)), "OPUS_SET_VBR");
    require_opus(opus_encoder_ctl(encoder, OPUS_SET_VBR_CONSTRAINT(1)), "OPUS_SET_VBR_CONSTRAINT");
    require_opus(opus_encoder_ctl(encoder, OPUS_SET_COMPLEXITY(COMPLEXITY)), "OPUS_SET_COMPLEXITY");
    require_opus(opus_encoder_ctl(encoder, OPUS_SET_DTX(0)), "OPUS_SET_DTX");
    require_opus(opus_encoder_ctl(encoder, OPUS_SET_INBAND_FEC(1)), "OPUS_SET_INBAND_FEC");
    require_opus(opus_encoder_ctl(encoder, OPUS_SET_PACKET_LOSS_PERC(EXPECTED_LOSS_PERCENT)),
                 "OPUS_SET_PACKET_LOSS_PERC");
    require_opus(opus_encoder_ctl(encoder, OPUS_SET_LSB_DEPTH(16)), "OPUS_SET_LSB_DEPTH");

    const int samples = SAMPLE_RATE * frame_ms / 1000;
    opus_int16 *input = calloc((size_t)samples, sizeof(*input));
    opus_int16 *output = calloc((size_t)samples, sizeof(*output));
    unsigned char packet[MAX_PACKET_BYTES];
    int *packet_sizes = calloc((size_t)frames, sizeof(*packet_sizes));
    if (input == NULL || output == NULL || packet_sizes == NULL) {
        fprintf(stderr, "allocation failed\n");
        return 2;
    }

    double encode_seconds = 0.0;
    double decode_seconds = 0.0;
    uint64_t encoded_bytes = 0;
    for (int frame = 0; frame < frames; frame++) {
        fill_signal(input, samples, (uint64_t)frame);
        double started = monotonic_seconds();
        int packet_size = opus_encode(encoder, input, samples, packet, MAX_PACKET_BYTES);
        encode_seconds += monotonic_seconds() - started;
        if (packet_size < 0) {
            fprintf(stderr, "opus_encode: %s\n", opus_strerror(packet_size));
            return 2;
        }
        packet_sizes[frame] = packet_size;
        encoded_bytes += (uint64_t)packet_size;

        started = monotonic_seconds();
        int decoded = opus_decode(decoder, packet, packet_size, output, samples, 0);
        decode_seconds += monotonic_seconds() - started;
        if (decoded != samples) {
            fprintf(stderr, "opus_decode returned %d samples, expected %d\n", decoded, samples);
            return 2;
        }
    }

    const double duration_seconds = (double)frames * frame_ms / 1000.0;
    const double average_bitrate = (double)encoded_bytes * 8.0 / duration_seconds;
    printf("{");
    printf("\"schemaVersion\":1,");
    printf("\"libraryVersion\":\"%s\",", opus_get_version_string());
    printf("\"frameMs\":%d,\"frames\":%d,\"audioSeconds\":%.3f,", frame_ms, frames, duration_seconds);
    printf("\"sampleRateHz\":%d,\"channels\":%d,\"targetBitrateBps\":%d,", SAMPLE_RATE, CHANNELS, BITRATE);
    printf("\"constrainedVbr\":true,\"complexity\":%d,\"dtx\":false,", COMPLEXITY);
    printf("\"inbandFec\":true,\"expectedLossPercent\":%d,", EXPECTED_LOSS_PERCENT);
    printf("\"maxPacketBytes\":%d,\"averageBitrateBps\":%.3f,", MAX_PACKET_BYTES, average_bitrate);
    printf("\"packetBytes\":{\"p50\":%d,\"p95\":%d,\"max\":%d},",
           percentile(packet_sizes, frames, 0.50), percentile(packet_sizes, frames, 0.95),
           percentile(packet_sizes, frames, 1.00));
    printf("\"cpu\":{\"encodeMeanUs\":%.3f,\"decodeMeanUs\":%.3f,\"realtimeFactor\":%.6f},",
           encode_seconds * 1000000.0 / frames, decode_seconds * 1000000.0 / frames,
           (encode_seconds + decode_seconds) / duration_seconds);
    printf("\"stateBytes\":{\"encoder\":%d,\"decoder\":%d,\"workingBuffers\":%zu}",
           opus_encoder_get_size(CHANNELS), opus_decoder_get_size(CHANNELS),
           (size_t)(samples * sizeof(*input) * 2 + MAX_PACKET_BYTES + frames * sizeof(*packet_sizes)));
    printf("}\n");

    free(packet_sizes);
    free(output);
    free(input);
    opus_decoder_destroy(decoder);
    opus_encoder_destroy(encoder);
    return 0;
}
