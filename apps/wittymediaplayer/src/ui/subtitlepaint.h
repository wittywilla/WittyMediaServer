// Text subtitles as pictures to lay over the video.
#pragma once

#include "../video/videoview.h"

#include <QSize>
#include <QString>

namespace wmp {

// White words with a dark edge, low in the picture.
Overlay textOverlay(const QString &text, const QSize &frame, uint64_t id);

} // namespace wmp
