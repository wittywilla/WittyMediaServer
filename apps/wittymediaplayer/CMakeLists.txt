cmake_minimum_required(VERSION 3.21)
project(WittyMediaPlayer VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)

# QRhi, which draws the picture, sits in GuiPrivate: the player is tied to this Qt version.
set(QT_NO_PRIVATE_MODULE_WARNING ON)
# Qt is linked as shared libraries, so anyone can replace them (the LGPL route).
find_package(Qt6 6.7 REQUIRED COMPONENTS Core Gui GuiPrivate Widgets Multimedia ShaderTools Test)

# FFmpeg's libraries and headers: FFMPEG_DIR is what scripts/setup-toolchain.sh built.
set(FFMPEG_DIR "" CACHE PATH "Folder with FFmpeg's include, lib and bin")
if(NOT FFMPEG_DIR)
    message(FATAL_ERROR "Set FFMPEG_DIR to the folder FFmpeg was built into.")
endif()
add_library(ffmpeg INTERFACE)
target_include_directories(ffmpeg INTERFACE "${FFMPEG_DIR}/include")
target_link_directories(ffmpeg INTERFACE "${FFMPEG_DIR}/lib" "${FFMPEG_DIR}/bin")
target_link_libraries(ffmpeg INTERFACE avformat avcodec avfilter swresample swscale avutil)

qt_add_library(wmpcore STATIC
    src/media/mediaspec.h
    src/media/ffmpeg.h
    src/media/extentreader.h src/media/extentreader.cpp
    src/media/audiooutput.h src/media/audiooutput.cpp
    src/media/player.h src/media/player.cpp
    src/media/vlcplayback.h src/media/vlcplayback.cpp
    src/video/videoview.h src/video/videoview.cpp
    src/engine/engineclient.h src/engine/engineclient.cpp
    src/disc/disccontroller.h src/disc/disccontroller.cpp
    src/ui/theme.h src/ui/theme.cpp
    src/ui/controlbar.h src/ui/controlbar.cpp
    src/ui/subtitlepaint.h src/ui/subtitlepaint.cpp
    src/ui/imagedialog.h src/ui/imagedialog.cpp
    src/ui/settingsdialog.h src/ui/settingsdialog.cpp
    src/ui/mainwindow.h src/ui/mainwindow.cpp
    src/testscript.h src/testscript.cpp
)
target_compile_definitions(wmpcore PUBLIC WMP_VERSION="${PROJECT_VERSION}" QT_NO_CAST_FROM_BYTEARRAY)
target_link_libraries(wmpcore PUBLIC Qt6::Core Qt6::Gui Qt6::GuiPrivate Qt6::Widgets Qt6::Multimedia ffmpeg)
target_include_directories(wmpcore PUBLIC src)

qt_add_shaders(wmpcore "wmp_shaders"
    PREFIX "/"
    BASE src
    FILES
        src/shaders/quad.vert
        src/shaders/video.frag
        src/shaders/overlay.frag
)

qt_add_resources(wmpcore "wmp_themes"
    PREFIX "/themes"
    BASE resources/themes
    FILES
        resources/themes/dark.json
        resources/themes/dark-blue.json
        resources/themes/dark-gold.json
        resources/themes/dark-green.json
        resources/themes/dark-orange.json
        resources/themes/dark-pink.json
        resources/themes/dark-purple.json
        resources/themes/dark-red.json
        resources/themes/dark-teal.json
        resources/themes/oled.json
)

qt_add_executable(wittymediaplayer WIN32 src/main.cpp)
target_link_libraries(wittymediaplayer PRIVATE wmpcore)

enable_testing()
qt_add_executable(wmptests tests/tst_core.cpp)
set_target_properties(wmptests PROPERTIES WIN32_EXECUTABLE FALSE)
target_link_libraries(wmptests PRIVATE wmpcore Qt6::Test)
add_test(NAME wmptests COMMAND wmptests)
