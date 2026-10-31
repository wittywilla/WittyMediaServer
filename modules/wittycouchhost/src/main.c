// wittycouch-host: Moonlight's streaming core as a program of its own.
//
// This is free software under the GNU General Public License version 3: see LICENSE. It links
// moonlight-common-c, which is GPL-3.0, and nothing else from WittyMediaServer. It talks to
// whoever started it over stdin and stdout, in the protocol in PROTOCOL.md, so that anything
// using it stays a separate program.
//
// It takes one stream: a Start message, then frames out and input in, until stdin closes.

#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#include <windows.h>
typedef CRITICAL_SECTION lock_t;
#define lock_init(l) InitializeCriticalSection(l)
#define lock_take(l) EnterCriticalSection(l)
#define lock_give(l) LeaveCriticalSection(l)
#ifdef WITTY_CRYPTO_THREADS
void crypto_threads_init(void); // src/crypto/threads_windows.c
#endif
#else
#include <pthread.h>
typedef pthread_mutex_t lock_t;
#define lock_init(l) pthread_mutex_init(l, NULL)
#define lock_take(l) pthread_mutex_lock(l)
#define lock_give(l) pthread_mutex_unlock(l)
#endif
#ifdef __linux__
#include <linux/landlock.h>
#include <signal.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/syscall.h>
#include <unistd.h>
#endif

#include <Limelight.h>

#define HOST_VERSION "0.1.0"
#define PROTOCOL_VERSION 1
#define MAX_MESSAGE (1 << 20)

// Messages out.
enum { MSG_EVENT = 1, MSG_VIDEO = 2, MSG_AUDIO = 3, MSG_RUMBLE = 4 };
// Messages in.
enum {
    CMD_START = 64,
    CMD_PAD = 65,
    CMD_MOUSE_MOVE = 66,
    CMD_MOUSE_BUTTON = 67,
    CMD_KEY = 68,
    CMD_SCROLL = 69,
    CMD_IDR = 70,
    CMD_QUIT = 71,
    CMD_MOUSE_POSITION = 72,
    CMD_TEXT = 73,
    CMD_HSCROLL = 74,
};

static lock_t out_lock;
static uint8_t *video_buf;
static int video_cap;

static void put_u32(uint8_t *p, uint32_t v)
{
    p[0] = (uint8_t)v;
    p[1] = (uint8_t)(v >> 8);
    p[2] = (uint8_t)(v >> 16);
    p[3] = (uint8_t)(v >> 24);
}

static void put_u16(uint8_t *p, uint16_t v)
{
    p[0] = (uint8_t)v;
    p[1] = (uint8_t)(v >> 8);
}

static uint16_t get_u16(const uint8_t *p)
{
    return (uint16_t)(p[0] | (p[1] << 8));
}

// Whole messages only: several threads write, and a part-written one would end everything.
static void send_msg(uint8_t type, const void *a, size_t alen, const void *b, size_t blen)
{
    uint8_t head[5];
    head[0] = type;
    put_u32(head + 1, (uint32_t)(alen + blen));
    lock_take(&out_lock);
    fwrite(head, 1, sizeof(head), stdout);
    if (alen > 0) {
        fwrite(a, 1, alen, stdout);
    }
    if (blen > 0) {
        fwrite(b, 1, blen, stdout);
    }
    fflush(stdout);
    lock_give(&out_lock);
}

static void send_event(const char *format, ...)
{
    char buf[512];
    va_list ap;
    va_start(ap, format);
    int n = vsnprintf(buf, sizeof(buf), format, ap);
    va_end(ap);
    if (n > 0) {
        send_msg(MSG_EVENT, buf, n < (int)sizeof(buf) ? (size_t)n : sizeof(buf) - 1, NULL, 0);
    }
}

// Connection callbacks.
static void on_stage_failed(int stage, int code)
{
    send_event("{\"type\":\"failed\",\"stage\":\"%s\",\"code\":%d}", LiGetStageName(stage), code);
}

static void on_started(void)
{
    send_event("{\"type\":\"started\"}");
}

static void on_terminated(int code)
{
    send_event("{\"type\":\"terminated\",\"code\":%d}", code);
    lock_take(&out_lock);
    fflush(stdout);
    // The connection is gone, and this callback can't stop it from inside. Exiting ends what's left.
    _Exit(0);
}

static void on_log(const char *format, ...)
{
    // Moonlight's logs name addresses and timings. They only go out when asked for.
    if (getenv("WITTYCOUCH_HOST_LOG")) {
        va_list ap;
        va_start(ap, format);
        vfprintf(stderr, format, ap);
        va_end(ap);
    }
}

static void on_rumble(unsigned short controller, unsigned short low, unsigned short high)
{
    uint8_t p[6];
    put_u16(p, controller);
    put_u16(p + 2, low);
    put_u16(p + 4, high);
    send_msg(MSG_RUMBLE, p, sizeof(p), NULL, 0);
}

static void on_status(int status)
{
    send_event("{\"type\":\"status\",\"poor\":%s}", status == CONN_STATUS_POOR ? "true" : "false");
}

// Video: whole frames, Annex B, exactly as the host sent them.
static int on_video_setup(int format, int width, int height, int rate, void *ctx, int flags)
{
    (void)ctx;
    (void)flags;
    send_event("{\"type\":\"video\",\"format\":%d,\"width\":%d,\"height\":%d,\"fps\":%d}", format, width, height, rate);
    return 0;
}

static int on_video_frame(PDECODE_UNIT du)
{
    // Flags (bit 0: a keyframe), then the host's 90 kHz timestamp, then the frame.
    int total = 5 + du->fullLength;
    if (total > video_cap) {
        uint8_t *grown = realloc(video_buf, (size_t)total + (1 << 16));
        if (!grown) {
            return DR_NEED_IDR;
        }
        video_buf = grown;
        video_cap = total + (1 << 16);
    }
    video_buf[0] = du->frameType == FRAME_TYPE_IDR ? 1 : 0;
    put_u32(video_buf + 1, du->rtpTimestamp);
    int at = 5;
    for (PLENTRY e = du->bufferList; e; e = e->next) {
        memcpy(video_buf + at, e->data, (size_t)e->length);
        at += e->length;
    }
    send_msg(MSG_VIDEO, video_buf, (size_t)at, NULL, 0);
    return DR_OK;
}

// Audio: Opus packets, undecoded.
static int on_audio_init(int config, const POPUS_MULTISTREAM_CONFIGURATION opus, void *ctx, int flags)
{
    (void)config;
    (void)ctx;
    (void)flags;
    send_event("{\"type\":\"audio\",\"rate\":%d,\"channels\":%d,\"streams\":%d,\"coupled\":%d,\"samples\":%d}", opus->sampleRate, opus->channelCount,
               opus->streams, opus->coupledStreams, opus->samplesPerFrame);
    return 0;
}

static void on_audio_sample(char *data, int length)
{
    if (data && length > 0) {
        send_msg(MSG_AUDIO, data, (size_t)length, NULL, 0);
    }
}

static int read_full(void *buf, size_t n)
{
    return fread(buf, 1, n, stdin) == n;
}

static int hex_value(char c)
{
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
}

static int hex16(const char *s, char *out)
{
    if (strlen(s) != 32) {
        return 0;
    }
    for (int i = 0; i < 16; i++) {
        int a = hex_value(s[i * 2]), b = hex_value(s[i * 2 + 1]);
        if (a < 0 || b < 0) {
            return 0;
        }
        out[i] = (char)(a * 16 + b);
    }
    return 1;
}

// The Start message is key=value lines, parsed in place.
static const char *start_value(char *buf, size_t len, const char *key)
{
    size_t klen = strlen(key);
    char *line = buf;
    while (line < buf + len) {
        size_t rest = (size_t)(buf + len - line);
        char *end = memchr(line, 0, rest);
        if (!end) {
            return NULL;
        }
        if (strncmp(line, key, klen) == 0 && line[klen] == '=') {
            return line + klen + 1;
        }
        line = end + 1;
    }
    return NULL;
}

static int start_int(char *buf, size_t len, const char *key, int def)
{
    const char *v = start_value(buf, len, key);
    return v ? atoi(v) : def;
}

static int start(char *msg, uint32_t len)
{
    for (uint32_t i = 0; i < len; i++) {
        if (msg[i] == '\n') {
            msg[i] = 0;
        }
    }
    SERVER_INFORMATION server;
    LiInitializeServerInformation(&server);
    server.address = start_value(msg, len, "address");
    server.serverInfoAppVersion = start_value(msg, len, "appversion");
    server.serverInfoGfeVersion = start_value(msg, len, "gfeversion");
    server.rtspSessionUrl = start_value(msg, len, "rtsp");
    server.serverCodecModeSupport = start_int(msg, len, "scm", SCM_H264);
    const char *key = start_value(msg, len, "rikey"), *iv = start_value(msg, len, "riiv");
    STREAM_CONFIGURATION cfg;
    LiInitializeStreamConfiguration(&cfg);
    cfg.width = start_int(msg, len, "width", 1280);
    cfg.height = start_int(msg, len, "height", 720);
    cfg.fps = start_int(msg, len, "fps", 60);
    cfg.bitrate = start_int(msg, len, "bitrate", 10000);
    cfg.packetSize = start_int(msg, len, "packetsize", 1024);
    cfg.streamingRemotely = start_int(msg, len, "remote", STREAM_CFG_AUTO);
    cfg.audioConfiguration = AUDIO_CONFIGURATION_STEREO;
    cfg.supportedVideoFormats = start_int(msg, len, "formats", VIDEO_FORMAT_H264);
    cfg.clientRefreshRateX100 = cfg.fps * 100;
    cfg.colorSpace = COLORSPACE_REC_709;
    cfg.colorRange = COLOR_RANGE_LIMITED;
    cfg.encryptionFlags = (int)ENCFLG_ALL;
    if (!server.address || !server.serverInfoAppVersion || !key || !iv || !hex16(key, cfg.remoteInputAesKey) || !hex16(iv, cfg.remoteInputAesIv)) {
        send_event("{\"type\":\"failed\",\"stage\":\"start message\",\"code\":0}");
        return 0;
    }
    CONNECTION_LISTENER_CALLBACKS cl;
    LiInitializeConnectionCallbacks(&cl);
    cl.stageFailed = on_stage_failed;
    cl.connectionStarted = on_started;
    cl.connectionTerminated = on_terminated;
    cl.logMessage = on_log;
    cl.rumble = on_rumble;
    cl.connectionStatusUpdate = on_status;
    DECODER_RENDERER_CALLBACKS dr;
    LiInitializeVideoCallbacks(&dr);
    dr.setup = on_video_setup;
    dr.submitDecodeUnit = on_video_frame;
    dr.capabilities = CAPABILITY_DIRECT_SUBMIT;
    AUDIO_RENDERER_CALLBACKS ar;
    LiInitializeAudioCallbacks(&ar);
    ar.init = on_audio_init;
    ar.decodeAndPlaySample = on_audio_sample;
    ar.capabilities = CAPABILITY_DIRECT_SUBMIT;
    return LiStartConnection(&server, &cfg, &cl, &dr, &ar, NULL, 0, NULL, 0) == 0;
}

static void handle_input(uint8_t type, const uint8_t *p, uint32_t n)
{
    switch (type) {
    case CMD_PAD: // controller, active mask, buttons (u32), triggers, four sticks
        if (n == 18) {
            LiSendMultiControllerEvent((short)p[0], (short)get_u16(p + 2), (int)(get_u16(p + 4) | ((uint32_t)get_u16(p + 6) << 16)), p[8], p[9],
                                       (short)get_u16(p + 10), (short)get_u16(p + 12), (short)get_u16(p + 14), (short)get_u16(p + 16));
        }
        break;
    case CMD_MOUSE_MOVE:
        if (n == 4) {
            LiSendMouseMoveEvent((short)get_u16(p), (short)get_u16(p + 2));
        }
        break;
    case CMD_MOUSE_POSITION: // x, y, and the size they're measured against
        if (n == 8) {
            LiSendMousePositionEvent((short)get_u16(p), (short)get_u16(p + 2), (short)get_u16(p + 4), (short)get_u16(p + 6));
        }
        break;
    case CMD_MOUSE_BUTTON:
        if (n == 2) {
            LiSendMouseButtonEvent((char)p[0], (int)p[1]);
        }
        break;
    case CMD_KEY: // Windows virtual key, action, modifiers
        if (n == 4) {
            LiSendKeyboardEvent((short)get_u16(p), (char)p[2], (char)p[3]);
        }
        break;
    case CMD_SCROLL:
        if (n == 2) {
            LiSendHighResScrollEvent((short)get_u16(p));
        }
        break;
    case CMD_HSCROLL:
        if (n == 2) {
            LiSendHighResHScrollEvent((short)get_u16(p));
        }
        break;
    case CMD_TEXT:
        if (n > 0 && n <= 256) {
            LiSendUtf8TextEvent((const char *)p, n);
        }
        break;
    case CMD_IDR:
        LiRequestIdrFrame();
        break;
    default:
        break;
    }
}

#ifdef __linux__
// What comes from a PC is parsed here in C, and that PC may be a stranger's. So this program gives
// up what it doesn't need before it connects to anything: it can't gain privileges, write a core
// file, or open a file at all where the kernel has Landlock. It only needs its pipes and the network.
static void give_things_up(void)
{
    struct rlimit none = {0, 0};
    setrlimit(RLIMIT_CORE, &none);
    setrlimit(RLIMIT_FSIZE, &none);
    prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0);
#ifdef LANDLOCK_ACCESS_FS_EXECUTE
    long abi = syscall(__NR_landlock_create_ruleset, NULL, 0, LANDLOCK_CREATE_RULESET_VERSION);
    if (abi > 0) {
        struct landlock_ruleset_attr attr = {0};
        attr.handled_access_fs = LANDLOCK_ACCESS_FS_EXECUTE | LANDLOCK_ACCESS_FS_WRITE_FILE | LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_READ_DIR |
                                 LANDLOCK_ACCESS_FS_REMOVE_DIR | LANDLOCK_ACCESS_FS_REMOVE_FILE | LANDLOCK_ACCESS_FS_MAKE_CHAR | LANDLOCK_ACCESS_FS_MAKE_DIR |
                                 LANDLOCK_ACCESS_FS_MAKE_REG | LANDLOCK_ACCESS_FS_MAKE_SOCK | LANDLOCK_ACCESS_FS_MAKE_FIFO | LANDLOCK_ACCESS_FS_MAKE_BLOCK |
                                 LANDLOCK_ACCESS_FS_MAKE_SYM;
        long fd = syscall(__NR_landlock_create_ruleset, &attr, sizeof(attr), 0);
        if (fd >= 0) {
            syscall(__NR_landlock_restrict_self, fd, 0);
            close((int)fd);
        }
    }
#endif
}
#endif

int main(int argc, char **argv)
{
    // Half of an install check: it has to run, and say what it is.
    if (argc > 1 && strcmp(argv[1], "--version") == 0) {
        printf("wittycouch-host " HOST_VERSION " protocol %d\n", PROTOCOL_VERSION);
        return 0;
    }
#ifdef __linux__
    prctl(PR_SET_PDEATHSIG, SIGKILL);
    give_things_up();
#endif
#ifdef _WIN32
    // No dialog on a server nobody's looking at, and DLLs only from Windows' own folder, never
    // the current one or this program's.
    SetErrorMode(SEM_FAILCRITICALERRORS | SEM_NOGPFAULTERRORBOX | SEM_NOOPENFILEERRORBOX);
    SetDefaultDllDirectories(LOAD_LIBRARY_SEARCH_SYSTEM32);
    _setmode(_fileno(stdin), _O_BINARY);
    _setmode(_fileno(stdout), _O_BINARY);
#ifdef WITTY_CRYPTO_THREADS
    crypto_threads_init();
#endif
#endif
    lock_init(&out_lock);
    static uint8_t msg[MAX_MESSAGE];
    int streaming = 0;
    send_event("{\"type\":\"hello\",\"version\":\"" HOST_VERSION "\",\"protocol\":%d}", PROTOCOL_VERSION);
    for (;;) {
        uint8_t head[5];
        if (!read_full(head, sizeof(head))) {
            break; // whoever started this has gone
        }
        uint32_t n = (uint32_t)head[1] | ((uint32_t)head[2] << 8) | ((uint32_t)head[3] << 16) | ((uint32_t)head[4] << 24);
        if (n >= MAX_MESSAGE || (n > 0 && !read_full(msg, n))) {
            break;
        }
        if (head[0] == CMD_QUIT) {
            break;
        }
        if (head[0] == CMD_START) {
            if (streaming) {
                continue;
            }
            msg[n] = 0;
            if (!start((char *)msg, n)) {
                return 1;
            }
            streaming = 1;
            continue;
        }
        if (streaming) {
            handle_input(head[0], msg, n);
        }
    }
    if (streaming) {
        LiStopConnection();
    }
    return 0;
}
