# WittyVisualizer for Qt: the same effects as the web app, on QRhi, so they run on Direct3D
# 11 on Windows, Metal on Apple and Vulkan or OpenGL on Linux. An app adds this folder with
# add_subdirectory, links to wittyvisualizer, and shows a wv::VisualizerWidget.
#
# Effects aren't copied here. Each file in ../effects is wrapped with the first lines Qt's
# shader tools want, at configure time, and baked for every graphics API at build time.
find_package(Qt6 6.7 REQUIRED COMPONENTS Core Gui GuiPrivate Widgets ShaderTools)
set(QT_NO_PRIVATE_MODULE_WARNING ON)
set(WV_ROOT "${CMAKE_CURRENT_LIST_DIR}/..")
set(WV_GEN "${CMAKE_CURRENT_BINARY_DIR}/wvshaders")
file(MAKE_DIRECTORY "${WV_GEN}")

set(WV_HEADER "#version 440
#define WV_UBO layout(std140, binding = 0)
#define WV_SAMPLER(n) layout(binding = n) uniform sampler2D
#define WV_IN(n) layout(location = n) in
#define WV_OUT(n) layout(location = n) out
#define WV_VOUT(n) layout(location = n) out
#define WV_VERTEX_ID gl_VertexIndex
")

function(wv_read_text out path)
  file(READ "${path}" text)
  string(REPLACE "\r\n" "\n" text "${text}")
  set(${out} "${text}" PARENT_SCOPE)
endfunction()

wv_read_text(WV_FRAME "${WV_ROOT}/shaders/frame.glsl")
wv_read_text(WV_LIB "${WV_ROOT}/shaders/lib.glsl")
wv_read_text(WV_PASSES "${WV_ROOT}/shaders/passes.glsl")

# Cuts a section out of passes.glsl, as shaders.ts does.
function(wv_section out name)
  set(mark "//@ ${name}\n")
  string(FIND "${WV_PASSES}" "${mark}" at)
  if(at LESS 0)
    message(FATAL_ERROR "no shader section ${name}")
  endif()
  string(LENGTH "${mark}" skip)
  math(EXPR begin "${at} + ${skip}")
  string(SUBSTRING "${WV_PASSES}" ${begin} -1 rest)
  string(FIND "${rest}" "\n//@ " end)
  string(SUBSTRING "${rest}" 0 ${end} text)
  set(${out} "${text}\n" PARENT_SCOPE)
endfunction()

function(wv_write name text)
  # Only when it changed, so a configure doesn't bake everything again.
  file(CONFIGURE OUTPUT "${WV_GEN}/${name}" CONTENT "${text}" @ONLY NEWLINE_STYLE LF)
endfunction()

wv_section(WV_EFFECT_HEAD effect-head)
wv_section(WV_EFFECT_TAIL effect-tail)
wv_section(WV_FULL_VERT fullscreen-vert)
wv_section(WV_QUAD_VERT quad-vert)
wv_section(WV_QUAD_FRAG quad-frag)
wv_section(WV_COMPOSITE composite)
wv_section(WV_GUARD guard-frag)
wv_section(WV_PRESENT present-frag)

set(WV_SHADER_FILES "")
wv_write(fullscreen.vert "${WV_HEADER}${WV_FRAME}${WV_FULL_VERT}")
wv_write(quad.frag "${WV_HEADER}${WV_QUAD_FRAG}")
wv_write(guard.frag "${WV_HEADER}${WV_FRAME}${WV_COMPOSITE}${WV_GUARD}")
wv_write(present.frag "${WV_HEADER}${WV_FRAME}${WV_COMPOSITE}${WV_PRESENT}")
list(APPEND WV_SHADER_FILES "${WV_GEN}/fullscreen.vert" "${WV_GEN}/quad.frag" "${WV_GEN}/guard.frag" "${WV_GEN}/present.frag")

# One effect file becomes its fragment shader and, if it draws geometry, a vertex shader.
function(wv_effect_shaders id path)
  wv_read_text(src "${path}")
  set(flags "")
  string(FIND "${src}" "\"feedback\": true" hasFeedback)
  if(hasFeedback GREATER -1)
    set(flags "#define WV_FEEDBACK\n")
  endif()
  wv_write(${id}.frag "${WV_HEADER}${flags}${WV_FRAME}${WV_LIB}${WV_EFFECT_HEAD}${src}\n${WV_EFFECT_TAIL}")
  set(files "${WV_GEN}/${id}.frag")
  string(FIND "${src}" "\"geometry\"" hasGeometry)
  if(hasGeometry GREATER -1)
    wv_write(${id}.quad.vert "${WV_HEADER}#define WV_QUAD\n${WV_FRAME}${WV_LIB}${src}\n${WV_QUAD_VERT}")
    list(APPEND files "${WV_GEN}/${id}.quad.vert")
  endif()
  set(WV_SHADER_FILES "${WV_SHADER_FILES};${files}" PARENT_SCOPE)
endfunction()

file(GLOB WV_EFFECTS CONFIGURE_DEPENDS "${WV_ROOT}/effects/*.glsl")
set(WV_EFFECT_NAMES "")
foreach(path ${WV_EFFECTS})
  get_filename_component(id "${path}" NAME_WE)
  wv_effect_shaders(${id} "${path}")
  list(APPEND WV_EFFECT_NAMES "${id}.glsl")
endforeach()
# A test can bring in shaders that must never ship.
if(WITTYVISUALIZER_TESTS)
  wv_effect_shaders(strobe "${CMAKE_CURRENT_LIST_DIR}/tests/strobe.glsl")
endif()

qt_add_library(wittyvisualizer STATIC
    wvanalysis.h wvanalysis.cpp
    wvengine.h wvengine.cpp
    wvtap.h
    wvrenderer.h wvrenderer.cpp
    wvwidget.h wvwidget.cpp
)
target_compile_features(wittyvisualizer PUBLIC cxx_std_17)
set_target_properties(wittyvisualizer PROPERTIES AUTOMOC ON)
target_link_libraries(wittyvisualizer PUBLIC Qt6::Core Qt6::Gui Qt6::GuiPrivate Qt6::Widgets)
target_include_directories(wittyvisualizer PUBLIC "${CMAKE_CURRENT_LIST_DIR}")

# GLES 3 and GL 3.3 at the least: the effects use texelFetch and vertices with no data.
qt_add_shaders(wittyvisualizer "wv_shaders"
    PREFIX "/wv/shaders"
    BASE "${WV_GEN}"
    GLSL "300es,330"
    HLSL 50
    MSL 12
    FILES ${WV_SHADER_FILES}
)

# The descriptions are read from the same files at run time.
qt_add_resources(wittyvisualizer "wv_effects"
    PREFIX "/wv/effects"
    BASE "${WV_ROOT}/effects"
    FILES ${WV_EFFECTS} "${WV_ROOT}/effects/palettes.json"
)

if(WITTYVISUALIZER_TESTS)
  find_package(Qt6 REQUIRED COMPONENTS Test)
  qt_add_executable(wvtests tests/tst_visualizer.cpp)
  set_target_properties(wvtests PROPERTIES WIN32_EXECUTABLE FALSE AUTOMOC ON)
  target_link_libraries(wvtests PRIVATE wittyvisualizer Qt6::Test)
  target_compile_definitions(wvtests PRIVATE WV_SOURCE_DIR="${WV_ROOT}")
endif()
