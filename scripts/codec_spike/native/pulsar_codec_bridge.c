#include "pulsar_codec_bridge.h"

#include <limits.h>
#include <stddef.h>
#include <string.h>

#include <libavcodec/avcodec.h>
#include <libavformat/avformat.h>
#include <libavutil/error.h>
#include <libavutil/frame.h>
#include <libavutil/mathematics.h>
#include <libavutil/samplefmt.h>

static uint64_t checksum_bytes(uint64_t value, const uint8_t *data, size_t size) {
    size_t index;
    for (index = 0; index < size; ++index) {
        value ^= data[index];
        value *= UINT64_C(1099511628211);
    }
    return value;
}

static void checksum_frame(PulsarCodecProbeResult *result, const AVFrame *frame) {
    int bytes_per_sample = av_get_bytes_per_sample((enum AVSampleFormat)frame->format);
    int channels = frame->ch_layout.nb_channels;
    int channel;
    if (bytes_per_sample <= 0 || channels <= 0 || frame->nb_samples <= 0)
        return;
    if (av_sample_fmt_is_planar((enum AVSampleFormat)frame->format)) {
        for (channel = 0; channel < channels; ++channel) {
            result->pcm_checksum = checksum_bytes(
                result->pcm_checksum,
                frame->extended_data[channel],
                (size_t)frame->nb_samples * (size_t)bytes_per_sample
            );
        }
    } else {
        result->pcm_checksum = checksum_bytes(
            result->pcm_checksum,
            frame->extended_data[0],
            (size_t)frame->nb_samples * (size_t)channels * (size_t)bytes_per_sample
        );
    }
}

static int consume_frames(
    AVCodecContext *decoder,
    AVFrame *frame,
    AVRational time_base,
    int64_t cancel_after_frames,
    PulsarCodecProbeResult *result
) {
    int status;
    while ((status = avcodec_receive_frame(decoder, frame)) >= 0) {
        int64_t pts = frame->best_effort_timestamp;
        int64_t pts_ms = pts == AV_NOPTS_VALUE ? -1 : av_rescale_q(pts, time_base, (AVRational){1, 1000});
        if (result->frames == 0)
            result->first_pts_ms = pts_ms;
        result->last_pts_ms = pts_ms;
        result->frames += 1;
        result->samples += frame->nb_samples;
        result->sample_rate = frame->sample_rate;
        result->channels = frame->ch_layout.nb_channels;
        checksum_frame(result, frame);
        av_frame_unref(frame);
        if (cancel_after_frames > 0 && result->frames >= cancel_after_frames) {
            result->cancelled = 1;
            return AVERROR_EXIT;
        }
    }
    if (status == AVERROR(EAGAIN) || status == AVERROR_EOF)
        return 0;
    return status;
}

unsigned pulsar_codec_bridge_abi(void) {
    return 1U;
}

int pulsar_codec_probe_file(
    const char *path,
    int64_t seek_ms,
    int64_t cancel_after_frames,
    PulsarCodecProbeResult *result
) {
    AVFormatContext *format = NULL;
    AVCodecContext *decoder = NULL;
    const AVCodec *codec = NULL;
    AVPacket *packet = NULL;
    AVFrame *frame = NULL;
    int stream_index;
    int status = 0;

    if (path == NULL || result == NULL)
        return AVERROR(EINVAL);
    memset(result, 0, sizeof(*result));
    result->first_pts_ms = -1;
    result->last_pts_ms = -1;
    result->pcm_checksum = UINT64_C(1469598103934665603);

    status = avformat_open_input(&format, path, NULL, NULL);
    if (status < 0)
        goto done;
    status = avformat_find_stream_info(format, NULL);
    if (status < 0)
        goto done;
    stream_index = av_find_best_stream(format, AVMEDIA_TYPE_AUDIO, -1, -1, &codec, 0);
    if (stream_index < 0) {
        status = stream_index;
        goto done;
    }
    decoder = avcodec_alloc_context3(codec);
    if (decoder == NULL) {
        status = AVERROR(ENOMEM);
        goto done;
    }
    status = avcodec_parameters_to_context(decoder, format->streams[stream_index]->codecpar);
    if (status < 0)
        goto done;
    decoder->thread_count = 1;
    status = avcodec_open2(decoder, codec, NULL);
    if (status < 0)
        goto done;
    strncpy(result->codec, codec->name, sizeof(result->codec) - 1);

    if (seek_ms >= 0) {
        int64_t target = av_rescale_q(seek_ms, (AVRational){1, 1000}, format->streams[stream_index]->time_base);
        status = avformat_seek_file(format, stream_index, INT64_MIN, target, INT64_MAX, AVSEEK_FLAG_BACKWARD);
        if (status < 0)
            goto done;
        avcodec_flush_buffers(decoder);
    }

    packet = av_packet_alloc();
    frame = av_frame_alloc();
    if (packet == NULL || frame == NULL) {
        status = AVERROR(ENOMEM);
        goto done;
    }
    while ((status = av_read_frame(format, packet)) >= 0) {
        if (packet->stream_index == stream_index) {
            status = avcodec_send_packet(decoder, packet);
            if (status >= 0)
                status = consume_frames(decoder, frame, format->streams[stream_index]->time_base,
                                        cancel_after_frames, result);
        }
        av_packet_unref(packet);
        if (status < 0)
            break;
    }
    if (status == AVERROR_EOF)
        status = 0;
    if (status >= 0 && !result->cancelled) {
        status = avcodec_send_packet(decoder, NULL);
        if (status >= 0)
            status = consume_frames(decoder, frame, format->streams[stream_index]->time_base,
                                    cancel_after_frames, result);
        if (status >= 0) {
            result->drained = 1;
            status = 0;
        }
    }

done:
    if (status == AVERROR_EXIT && result->cancelled)
        status = 0;
    result->error_code = status;
    av_frame_free(&frame);
    av_packet_free(&packet);
    avcodec_free_context(&decoder);
    avformat_close_input(&format);
    return status;
}
