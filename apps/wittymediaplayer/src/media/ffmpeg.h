// The FFmpeg libraries, in one place.
#pragma once

extern "C" {
#include <libavcodec/avcodec.h>
#include <libavfilter/avfilter.h>
#include <libavfilter/buffersink.h>
#include <libavfilter/buffersrc.h>
#include <libavformat/avformat.h>
#include <libavformat/avio.h>
#include <libavutil/avutil.h>
#include <libavutil/channel_layout.h>
#include <libavutil/hwcontext.h>
#include <libavutil/imgutils.h>
#include <libavutil/opt.h>
#include <libavutil/pixdesc.h>
#include <libswresample/swresample.h>
#include <libswscale/swscale.h>
}

#include <memory>

namespace wmp {

struct FrameDeleter {
    void operator()(AVFrame *f) const { av_frame_free(&f); }
};
using FramePtr = std::shared_ptr<AVFrame>;

inline FramePtr shareFrame(AVFrame *f)
{
    return FramePtr(f, FrameDeleter{});
}

} // namespace wmp
