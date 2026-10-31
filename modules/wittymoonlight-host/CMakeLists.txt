cmake_minimum_required(VERSION 3.13...4.0)
project(wittymoonlight-host LANGUAGES C)

# Moonlight's streaming core, fetched at a pinned commit by scripts/fetch-core.sh.
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
add_subdirectory(third_party/moonlight-common-c)

add_executable(wittymoonlight-host src/main.c)
target_include_directories(wittymoonlight-host PRIVATE third_party/moonlight-common-c/src)
target_link_libraries(wittymoonlight-host PRIVATE moonlight-common-c)
set_target_properties(wittymoonlight-host PROPERTIES C_STANDARD 11)
if(NOT MSVC)
  target_compile_options(wittymoonlight-host PRIVATE -Wall -Wextra -O2)
endif()
if(WIN32 AND NOT MSVC)
  target_link_options(wittymoonlight-host PRIVATE -static)
endif()
