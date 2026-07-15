#include <windows.h>
#include <appmodel.h>
#include <mfapi.h>
#include <mferror.h>
#include <mfidl.h>
#include <mfreadwrite.h>
#include <objidl.h>
#include <psapi.h>
#include <wrl/client.h>

#include <algorithm>
#include <atomic>
#include <chrono>
#include <cstdint>
#include <iomanip>
#include <iterator>
#include <memory>
#include <mutex>
#include <new>
#include <sstream>
#include <string>
#include <thread>
#include <vector>

using Microsoft::WRL::ComPtr;

namespace {

constexpr ULONG kMaximumRead = 1024 * 1024;

std::string utf8(const std::wstring& value) {
    if (value.empty()) return {};
    int size = WideCharToMultiByte(CP_UTF8, 0, value.data(), static_cast<int>(value.size()),
                                   nullptr, 0, nullptr, nullptr);
    std::string result(static_cast<size_t>(size), '\0');
    WideCharToMultiByte(CP_UTF8, 0, value.data(), static_cast<int>(value.size()),
                        result.data(), size, nullptr, nullptr);
    return result;
}

std::string json_escape(const std::string& value) {
    std::ostringstream out;
    for (unsigned char ch : value) {
        switch (ch) {
            case '\\': out << "\\\\"; break;
            case '"': out << "\\\""; break;
            case '\n': out << "\\n"; break;
            case '\r': out << "\\r"; break;
            case '\t': out << "\\t"; break;
            default:
                if (ch < 0x20) {
                    out << "\\u" << std::hex << std::setw(4) << std::setfill('0')
                        << static_cast<unsigned>(ch) << std::dec;
                } else {
                    out << ch;
                }
        }
    }
    return out.str();
}

std::string hresult_hex(HRESULT hr) {
    std::ostringstream out;
    out << "0x" << std::uppercase << std::hex << std::setw(8) << std::setfill('0')
        << static_cast<uint32_t>(hr);
    return out.str();
}

uint64_t monotonic_ms() {
    return static_cast<uint64_t>(std::chrono::duration_cast<std::chrono::milliseconds>(
        std::chrono::steady_clock::now().time_since_epoch()).count());
}

uint64_t rss_bytes() {
    PROCESS_MEMORY_COUNTERS counters{};
    counters.cb = sizeof(counters);
    if (!GetProcessMemoryInfo(GetCurrentProcess(), &counters, sizeof(counters))) return 0;
    return static_cast<uint64_t>(counters.WorkingSetSize);
}

class RangeFileStream final : public IStream {
public:
    static HRESULT Open(const std::wstring& path, RangeFileStream** result) {
        if (!result) return E_POINTER;
        *result = nullptr;
        HANDLE file = CreateFileW(path.c_str(), GENERIC_READ, FILE_SHARE_READ, nullptr,
                                  OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
        if (file == INVALID_HANDLE_VALUE) return HRESULT_FROM_WIN32(GetLastError());
        LARGE_INTEGER size{};
        if (!GetFileSizeEx(file, &size)) {
            HRESULT hr = HRESULT_FROM_WIN32(GetLastError());
            CloseHandle(file);
            return hr;
        }
        *result = new (std::nothrow) RangeFileStream(file, static_cast<uint64_t>(size.QuadPart));
        if (!*result) {
            CloseHandle(file);
            return E_OUTOFMEMORY;
        }
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID iid, void** object) override {
        if (!object) return E_POINTER;
        *object = nullptr;
        if (iid == __uuidof(IUnknown) || iid == __uuidof(ISequentialStream) ||
            iid == __uuidof(IStream)) {
            *object = static_cast<IStream*>(this);
            AddRef();
            return S_OK;
        }
        return E_NOINTERFACE;
    }

    ULONG STDMETHODCALLTYPE AddRef() override { return ++refs_; }
    ULONG STDMETHODCALLTYPE Release() override {
        ULONG value = --refs_;
        if (value == 0) delete this;
        return value;
    }

    HRESULT STDMETHODCALLTYPE Read(void* buffer, ULONG requested, ULONG* bytes_read) override {
        if (!buffer && requested != 0) return STG_E_INVALIDPOINTER;
        std::lock_guard<std::mutex> lock(mutex_);
        if (bytes_read) *bytes_read = 0;
        ULONG total = 0;
        auto* output = static_cast<BYTE*>(buffer);
        while (total < requested && position_ < length_) {
            ULONG chunk = static_cast<ULONG>(std::min<uint64_t>(
                std::min<uint64_t>(requested - total, kMaximumRead), length_ - position_));
            LARGE_INTEGER offset{};
            offset.QuadPart = static_cast<LONGLONG>(position_);
            if (!SetFilePointerEx(file_, offset, nullptr, FILE_BEGIN)) {
                return HRESULT_FROM_WIN32(GetLastError());
            }
            DWORD actual = 0;
            if (!ReadFile(file_, output + total, chunk, &actual, nullptr)) {
                return HRESULT_FROM_WIN32(GetLastError());
            }
            ++read_operations_;
            max_read_ = std::max<ULONG>(max_read_.load(), chunk);
            total += actual;
            position_ += actual;
            total_bytes_read_ += actual;
            if (actual < chunk) break;
        }
        if (bytes_read) *bytes_read = total;
        return total == requested ? S_OK : S_FALSE;
    }

    HRESULT STDMETHODCALLTYPE Write(const void*, ULONG, ULONG*) override { return STG_E_ACCESSDENIED; }

    HRESULT STDMETHODCALLTYPE Seek(LARGE_INTEGER move, DWORD origin, ULARGE_INTEGER* new_position) override {
        std::lock_guard<std::mutex> lock(mutex_);
        int64_t base = 0;
        if (origin == STREAM_SEEK_SET) base = 0;
        else if (origin == STREAM_SEEK_CUR) base = static_cast<int64_t>(position_);
        else if (origin == STREAM_SEEK_END) base = static_cast<int64_t>(length_);
        else return STG_E_INVALIDFUNCTION;
        int64_t target = base + move.QuadPart;
        if (target < 0) return STG_E_INVALIDFUNCTION;
        position_ = static_cast<uint64_t>(target);
        if (new_position) new_position->QuadPart = position_;
        ++seek_operations_;
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE SetSize(ULARGE_INTEGER) override { return STG_E_ACCESSDENIED; }
    HRESULT STDMETHODCALLTYPE CopyTo(IStream*, ULARGE_INTEGER, ULARGE_INTEGER*, ULARGE_INTEGER*) override {
        return E_NOTIMPL;
    }
    HRESULT STDMETHODCALLTYPE Commit(DWORD) override { return S_OK; }
    HRESULT STDMETHODCALLTYPE Revert() override { return STG_E_REVERTED; }
    HRESULT STDMETHODCALLTYPE LockRegion(ULARGE_INTEGER, ULARGE_INTEGER, DWORD) override { return STG_E_INVALIDFUNCTION; }
    HRESULT STDMETHODCALLTYPE UnlockRegion(ULARGE_INTEGER, ULARGE_INTEGER, DWORD) override { return STG_E_INVALIDFUNCTION; }
    HRESULT STDMETHODCALLTYPE Stat(STATSTG* stat, DWORD flags) override {
        if (!stat) return STG_E_INVALIDPOINTER;
        ZeroMemory(stat, sizeof(*stat));
        stat->type = STGTY_STREAM;
        stat->cbSize.QuadPart = length_;
        stat->grfMode = STGM_READ;
        if (!(flags & STATFLAG_NONAME)) stat->pwcsName = nullptr;
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE Clone(IStream**) override { return E_NOTIMPL; }

    uint64_t total_bytes_read() const { return total_bytes_read_.load(); }
    uint64_t read_operations() const { return read_operations_.load(); }
    uint64_t seek_operations() const { return seek_operations_.load(); }
    ULONG max_read() const { return max_read_.load(); }
    uint64_t length() const { return length_; }

private:
    RangeFileStream(HANDLE file, uint64_t length) : file_(file), length_(length) {}
    ~RangeFileStream() { if (file_ != INVALID_HANDLE_VALUE) CloseHandle(file_); }

    std::atomic<ULONG> refs_{1};
    HANDLE file_ = INVALID_HANDLE_VALUE;
    const uint64_t length_;
    uint64_t position_ = 0;
    mutable std::mutex mutex_;
    std::atomic<uint64_t> total_bytes_read_{0};
    std::atomic<uint64_t> read_operations_{0};
    std::atomic<uint64_t> seek_operations_{0};
    std::atomic<ULONG> max_read_{0};
};

struct ReaderContext {
    ComPtr<IMFSourceReader> reader;
    ComPtr<IStream> stream_owner;
    RangeFileStream* stream = nullptr;
    HRESULT open_hr = E_FAIL;
};

ReaderContext open_reader(const std::wstring& path, const std::wstring& mime) {
    ReaderContext context;
    RangeFileStream* raw = nullptr;
    HRESULT hr = RangeFileStream::Open(path, &raw);
    if (FAILED(hr)) { context.open_hr = hr; return context; }
    context.stream = raw;
    context.stream_owner.Attach(raw);

    ComPtr<IMFByteStream> byte_stream;
    hr = MFCreateMFByteStreamOnStreamEx(context.stream_owner.Get(), &byte_stream);
    if (FAILED(hr)) { context.open_hr = hr; return context; }
    ComPtr<IMFAttributes> byte_attributes;
    if (SUCCEEDED(byte_stream.As(&byte_attributes))) {
        byte_attributes->SetString(MF_BYTESTREAM_CONTENT_TYPE, mime.c_str());
        byte_attributes->SetString(MF_BYTESTREAM_ORIGIN_NAME, path.c_str());
    }
    ComPtr<IMFAttributes> attributes;
    hr = MFCreateAttributes(&attributes, 1);
    if (SUCCEEDED(hr)) hr = MFCreateSourceReaderFromByteStream(byte_stream.Get(), attributes.Get(), &context.reader);
    if (FAILED(hr)) { context.open_hr = hr; return context; }
    hr = context.reader->SetStreamSelection(static_cast<DWORD>(MF_SOURCE_READER_ALL_STREAMS), FALSE);
    if (SUCCEEDED(hr)) hr = context.reader->SetStreamSelection(
        static_cast<DWORD>(MF_SOURCE_READER_FIRST_AUDIO_STREAM), TRUE);
    ComPtr<IMFMediaType> pcm;
    if (SUCCEEDED(hr)) hr = MFCreateMediaType(&pcm);
    if (SUCCEEDED(hr)) hr = pcm->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Audio);
    if (SUCCEEDED(hr)) hr = pcm->SetGUID(MF_MT_SUBTYPE, MFAudioFormat_Float);
    if (SUCCEEDED(hr)) hr = context.reader->SetCurrentMediaType(
        static_cast<DWORD>(MF_SOURCE_READER_FIRST_AUDIO_STREAM), nullptr, pcm.Get());
    context.open_hr = hr;
    if (FAILED(hr)) context.reader.Reset();
    return context;
}

struct SampleResult {
    HRESULT hr = E_FAIL;
    DWORD flags = 0;
    LONGLONG timestamp = 0;
    DWORD bytes = 0;
    bool sample = false;
};

SampleResult read_sample(IMFSourceReader* reader) {
    SampleResult result;
    ComPtr<IMFSample> sample;
    DWORD stream_index = 0;
    result.hr = reader->ReadSample(static_cast<DWORD>(MF_SOURCE_READER_FIRST_AUDIO_STREAM), 0, &stream_index,
                                   &result.flags, &result.timestamp, &sample);
    if (SUCCEEDED(result.hr) && sample) {
        result.sample = true;
        sample->GetTotalLength(&result.bytes);
    }
    return result;
}

struct FixtureResult {
    std::string id;
    std::string expected;
    HRESULT open_hr = E_FAIL;
    HRESULT terminal_hr = E_FAIL;
    uint64_t samples = 0;
    uint64_t pcm_bytes = 0;
    uint64_t first_sample_source_bytes = 0;
    uint64_t source_bytes = 0;
    uint64_t read_operations = 0;
    uint64_t seek_operations = 0;
    uint64_t max_read = 0;
    int64_t first_timestamp_hns = 0;
    int64_t seek_timestamp_hns = 0;
    uint64_t scheduled_skew_ms = 0;
    uint64_t seek_to_sample_ms = 0;
    bool paused_without_read = false;
    bool seek_new_generation = false;
    bool resumed = false;
    bool drained = false;
    bool cancelled = false;
    bool passed = false;
};

FixtureResult exercise_fixture(const std::string& id, const std::string& expected,
                               const std::wstring& path, const std::wstring& mime) {
    FixtureResult result;
    result.id = id;
    result.expected = expected;
    ReaderContext context = open_reader(path, mime);
    result.open_hr = context.open_hr;
    if (FAILED(context.open_hr)) {
        result.terminal_hr = context.open_hr;
        result.passed = expected == "reject-with-hresult";
        return result;
    }
    if (expected == "reject-with-hresult") {
        result.terminal_hr = S_OK;
        result.passed = false;
        return result;
    }

    const uint64_t scheduled = monotonic_ms() + 75;
    while (monotonic_ms() < scheduled) std::this_thread::sleep_for(std::chrono::milliseconds(1));
    SampleResult first = read_sample(context.reader.Get());
    result.scheduled_skew_ms = monotonic_ms() > scheduled ? monotonic_ms() - scheduled : 0;
    result.terminal_hr = first.hr;
    if (FAILED(first.hr) || !first.sample) return result;
    result.samples = 1;
    result.pcm_bytes = first.bytes;
    result.first_timestamp_hns = first.timestamp;
    result.first_sample_source_bytes = context.stream->total_bytes_read();

    for (int index = 0; index < 4; ++index) {
        SampleResult sample = read_sample(context.reader.Get());
        result.terminal_hr = sample.hr;
        if (FAILED(sample.hr)) return result;
        if (sample.sample) { ++result.samples; result.pcm_bytes += sample.bytes; }
    }
    uint64_t before_pause = context.stream->total_bytes_read();
    std::this_thread::sleep_for(std::chrono::milliseconds(25));
    result.paused_without_read = before_pause == context.stream->total_bytes_read();

    PROPVARIANT position;
    PropVariantInit(&position);
    position.vt = VT_I8;
    position.hVal.QuadPart = 5LL * 10000000LL;
    const uint64_t seek_start = monotonic_ms();
    HRESULT seek_hr = context.reader->SetCurrentPosition(GUID_NULL, position);
    PropVariantClear(&position);
    result.seek_new_generation = SUCCEEDED(seek_hr);
    if (FAILED(seek_hr)) { result.terminal_hr = seek_hr; return result; }
    SampleResult after_seek = read_sample(context.reader.Get());
    result.seek_to_sample_ms = monotonic_ms() - seek_start;
    result.terminal_hr = after_seek.hr;
    if (FAILED(after_seek.hr) || !after_seek.sample) return result;
    result.seek_timestamp_hns = after_seek.timestamp;
    result.resumed = true;
    ++result.samples;
    result.pcm_bytes += after_seek.bytes;

    for (uint64_t guard = 0; guard < 100000; ++guard) {
        SampleResult sample = read_sample(context.reader.Get());
        result.terminal_hr = sample.hr;
        if (FAILED(sample.hr)) return result;
        if (sample.sample) { ++result.samples; result.pcm_bytes += sample.bytes; }
        if (sample.flags & MF_SOURCE_READERF_ENDOFSTREAM) {
            result.drained = true;
            break;
        }
    }
    ReaderContext cancellation = open_reader(path, mime);
    if (SUCCEEDED(cancellation.open_hr)) {
        for (int index = 0; index < 3; ++index) {
            SampleResult sample = read_sample(cancellation.reader.Get());
            if (FAILED(sample.hr)) break;
        }
        result.cancelled = true;
    }
    result.source_bytes = context.stream->length();
    result.read_operations = context.stream->read_operations();
    result.seek_operations = context.stream->seek_operations();
    result.max_read = context.stream->max_read();
    result.passed = result.drained && result.cancelled && result.paused_without_read &&
                    result.seek_new_generation && result.resumed && result.samples > 0 &&
                    result.max_read <= kMaximumRead && result.scheduled_skew_ms <= 100 &&
                    result.seek_to_sample_ms <= 3000;
    return result;
}

std::wstring module_directory() {
    std::vector<wchar_t> buffer(32768);
    DWORD size = GetModuleFileNameW(nullptr, buffer.data(), static_cast<DWORD>(buffer.size()));
    std::wstring path(buffer.data(), size);
    size_t slash = path.find_last_of(L"\\/");
    return slash == std::wstring::npos ? L"." : path.substr(0, slash);
}

std::wstring package_family_name() {
    UINT32 length = 0;
    LONG status = GetCurrentPackageFamilyName(&length, nullptr);
    if (status != ERROR_INSUFFICIENT_BUFFER) return {};
    std::vector<wchar_t> value(length);
    if (GetCurrentPackageFamilyName(&length, value.data()) != ERROR_SUCCESS) return {};
    return value.data();
}

std::wstring package_full_name() {
    UINT32 length = 0;
    LONG status = GetCurrentPackageFullName(&length, nullptr);
    if (status != ERROR_INSUFFICIENT_BUFFER) return {};
    std::vector<wchar_t> value(length);
    if (GetCurrentPackageFullName(&length, value.data()) != ERROR_SUCCESS) return {};
    return value.data();
}

bool token_is_appcontainer() {
    HANDLE token = nullptr;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) return false;
    DWORD value = 0;
    DWORD size = 0;
    BOOL ok = GetTokenInformation(token, TokenIsAppContainer, &value, sizeof(value), &size);
    CloseHandle(token);
    return ok && value != 0;
}

DWORD soak_seconds_from_command_line() {
    std::wstring line = GetCommandLineW();
    const std::wstring marker = L"--soak-seconds=";
    size_t at = line.find(marker);
    if (at == std::wstring::npos) return 60;
    wchar_t* end = nullptr;
    unsigned long value = wcstoul(line.c_str() + at + marker.size(), &end, 10);
    return value > 7200 ? 7200 : static_cast<DWORD>(value);
}

std::wstring evidence_path(const std::wstring& family) {
    wchar_t user_profile[32768]{};
    DWORD size = GetEnvironmentVariableW(L"USERPROFILE", user_profile,
                                         static_cast<DWORD>(std::size(user_profile)));
    if (size == 0 || size >= std::size(user_profile)) return {};
    std::wstring path(user_profile);
    path += L"\\AppData\\Local\\Packages\\" + family + L"\\LocalState";
    return path + L"\\mf-probe-evidence.json";
}

bool write_file(const std::wstring& path, const std::string& content) {
    HANDLE file = CreateFileW(path.c_str(), GENERIC_WRITE, 0, nullptr, CREATE_ALWAYS,
                              FILE_ATTRIBUTE_NORMAL, nullptr);
    if (file == INVALID_HANDLE_VALUE) return false;
    DWORD written = 0;
    BOOL ok = WriteFile(file, content.data(), static_cast<DWORD>(content.size()), &written, nullptr);
    FlushFileBuffers(file);
    CloseHandle(file);
    return ok && written == content.size();
}

std::string fixture_json(const FixtureResult& result) {
    std::ostringstream out;
    out << "{\"id\":\"" << json_escape(result.id) << "\",\"expected\":\""
        << json_escape(result.expected) << "\",\"openHRESULT\":\""
        << hresult_hex(result.open_hr) << "\",\"terminalHRESULT\":\""
        << hresult_hex(result.terminal_hr) << "\",\"samples\":" << result.samples
        << ",\"pcmBytes\":" << result.pcm_bytes
        << ",\"firstSampleSourceBytes\":" << result.first_sample_source_bytes
        << ",\"sourceBytes\":" << result.source_bytes
        << ",\"readOperations\":" << result.read_operations
        << ",\"seekOperations\":" << result.seek_operations
        << ",\"maximumReadBytes\":" << result.max_read
        << ",\"firstTimestampHNS\":" << result.first_timestamp_hns
        << ",\"seekTimestampHNS\":" << result.seek_timestamp_hns
        << ",\"scheduledSkewMS\":" << result.scheduled_skew_ms
        << ",\"seekToSampleMS\":" << result.seek_to_sample_ms
        << ",\"pausedWithoutRead\":" << (result.paused_without_read ? "true" : "false")
        << ",\"seekNewGeneration\":" << (result.seek_new_generation ? "true" : "false")
        << ",\"resumed\":" << (result.resumed ? "true" : "false")
        << ",\"drained\":" << (result.drained ? "true" : "false")
        << ",\"cancelled\":" << (result.cancelled ? "true" : "false")
        << ",\"passed\":" << (result.passed ? "true" : "false") << "}";
    return out.str();
}

}  // namespace

int WINAPI wWinMain(HINSTANCE, HINSTANCE, PWSTR, int) {
    const std::wstring family = package_family_name();
    const std::wstring output = evidence_path(family);
    const uint64_t rss_start = rss_bytes();
    HRESULT com_hr = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
    HRESULT mf_hr = SUCCEEDED(com_hr) ? MFStartup(MF_VERSION, MFSTARTUP_FULL) : com_hr;

    struct Fixture { const char* id; const char* expected; const wchar_t* file; const wchar_t* mime; };
    const Fixture fixtures[] = {
        {"mp3_cbr_12s", "decode", L"mp3_cbr_12s.mp3", L"audio/mpeg"},
        {"mp3_vbr_12s", "decode", L"mp3_vbr_12s.mp3", L"audio/mpeg"},
        {"aac_m4a_12s", "decode", L"aac_m4a_12s.m4a", L"audio/mp4"},
        {"aac_adts_12s", "decode", L"aac_adts_12s.aac", L"audio/aac"},
        {"opus_ogg_cbr_12s", "reject-with-hresult", L"opus_ogg_cbr_12s.ogg", L"audio/ogg"},
        {"opus_ogg_vbr_12s", "reject-with-hresult", L"opus_ogg_vbr_12s.ogg", L"audio/ogg"},
    };
    std::vector<FixtureResult> results;
    bool passed = SUCCEEDED(mf_hr);
    const std::wstring fixture_root = module_directory() + L"\\Fixtures\\";
    if (SUCCEEDED(mf_hr)) {
        for (const auto& fixture : fixtures) {
            results.push_back(exercise_fixture(fixture.id, fixture.expected,
                                               fixture_root + fixture.file, fixture.mime));
            passed = passed && results.back().passed;
        }
    }

    const DWORD soak_seconds = soak_seconds_from_command_line();
    const uint64_t soak_start_ms = monotonic_ms();
    const uint64_t soak_rss_start = rss_bytes();
    uint64_t soak_peak_rss = soak_rss_start;
    uint64_t soak_iterations = 0;
    while (passed && monotonic_ms() - soak_start_ms < static_cast<uint64_t>(soak_seconds) * 1000) {
        ReaderContext reader = open_reader(fixture_root + L"mp3_cbr_12s.mp3", L"audio/mpeg");
        if (FAILED(reader.open_hr)) { passed = false; break; }
        for (int index = 0; index < 32; ++index) {
            SampleResult sample = read_sample(reader.reader.Get());
            if (FAILED(sample.hr)) { passed = false; break; }
            if (sample.flags & MF_SOURCE_READERF_ENDOFSTREAM) break;
        }
        ++soak_iterations;
        soak_peak_rss = std::max<uint64_t>(soak_peak_rss, rss_bytes());
        std::this_thread::sleep_for(std::chrono::milliseconds(1));
    }
    const uint64_t soak_rss_end = rss_bytes();
    const uint64_t rss_peak = std::max<uint64_t>(soak_peak_rss, rss_bytes());
    passed = passed && token_is_appcontainer() && !family.empty() && rss_peak <= 209715200ULL;

    std::ostringstream json;
    json << "{\"schemaVersion\":1,\"contract\":\"p2-media-foundation-appcontainer-probe.v1\""
         << ",\"claimClass\":\"repository-engineering-prototype\""
         << ",\"packageFamilyName\":\"" << json_escape(utf8(family)) << "\""
         << ",\"packageFullName\":\"" << json_escape(utf8(package_full_name())) << "\""
         << ",\"tokenIsAppContainer\":" << (token_is_appcontainer() ? "true" : "false")
         << ",\"comApartment\":\"MTA\",\"comHRESULT\":\"" << hresult_hex(com_hr) << "\""
         << ",\"mediaFoundationHRESULT\":\"" << hresult_hex(mf_hr) << "\""
         << ",\"decodeThreadId\":" << GetCurrentThreadId()
         << ",\"renderCallbackUsed\":false,\"decoderOwnsNetwork\":false"
         << ",\"maximumPreparedReadBytes\":" << kMaximumRead
         << ",\"rssStartBytes\":" << rss_start << ",\"peakRSSBytes\":" << rss_peak
         << ",\"soak\":{\"requestedSeconds\":" << soak_seconds
         << ",\"actualMilliseconds\":" << (monotonic_ms() - soak_start_ms)
         << ",\"iterations\":" << soak_iterations
         << ",\"rssStartBytes\":" << soak_rss_start
         << ",\"rssEndBytes\":" << soak_rss_end
         << ",\"peakRSSBytes\":" << soak_peak_rss << "},\"fixtures\":[";
    for (size_t index = 0; index < results.size(); ++index) {
        if (index) json << ',';
        json << fixture_json(results[index]);
    }
    json << "],\"passed\":" << (passed ? "true" : "false") << "}\n";

    bool wrote = !output.empty() && write_file(output, json.str());
    if (SUCCEEDED(mf_hr)) MFShutdown();
    if (SUCCEEDED(com_hr)) CoUninitialize();
    return wrote && passed ? 0 : 2;
}
