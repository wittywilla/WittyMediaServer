cmake_minimum_required(VERSION 3.13...4.0)
project(wittymoonlight-host LANGUAGES C)

option(USE_MBEDTLS "Build in the Mbed TLS that scripts/fetch-core.sh fetches. Off, this links the system's OpenSSL." ON)
option(WITTY_STATIC "Link everything statically, C library included. For musl: see the README about glibc." OFF)

if(NOT EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/third_party/moonlight-common-c/CMakeLists.txt")
  message(FATAL_ERROR "Run scripts/fetch-core.sh first, or use scripts/build.sh, which does.")
endif()

set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# What comes from a PC is parsed here in C, so everything linked gets the compiler's protections.
if(NOT MSVC)
  add_compile_options(-fstack-protector-strong -ffunction-sections)
endif()
if(NOT MSVC AND NOT MINGW)
  # Not on Windows, where it puts this program's zeroed 1 MiB buffer into the file itself.
  add_compile_options(-fdata-sections)
endif()
if(MINGW)
  # GCC doesn't align AVX values on the stack on Windows (its bug 54412), and nanors uses AVX when
  # the processor has it: without this, the stream's error correction can crash.
  add_compile_options(-Wa,-muse-unaligned-vector-move)
endif()
if(CMAKE_SYSTEM_NAME STREQUAL "Linux")
  add_compile_options($<$<CONFIG:Release>:-U_FORTIFY_SOURCE> $<$<CONFIG:Release>:-D_FORTIFY_SOURCE=2>)
endif()

if(USE_MBEDTLS)
  if(NOT EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/third_party/mbedtls/CMakeLists.txt")
    message(FATAL_ERROR "Run scripts/fetch-core.sh first, or use scripts/build.sh, which does.")
  endif()
  set(ENABLE_PROGRAMS OFF CACHE BOOL "" FORCE)
  set(ENABLE_TESTING OFF CACHE BOOL "" FORCE)
  set(MBEDTLS_FATAL_WARNINGS OFF CACHE BOOL "" FORCE)
  set(MBEDTLS_CONFIG_FILE "${CMAKE_CURRENT_SOURCE_DIR}/src/crypto/mbedtls_config.h" CACHE FILEPATH "" FORCE)
  if(NOT WIN32)
    set(LINK_WITH_PTHREAD ON CACHE BOOL "" FORCE)
  endif()
  # Always the one fetched at its pinned commit, never another that's installed on this computer.
  set(CMAKE_DISABLE_FIND_PACKAGE_MbedTLS ON)
  add_subdirectory(third_party/mbedtls EXCLUDE_FROM_ALL)
  target_include_directories(mbedcrypto PUBLIC src/crypto)
  target_sources(mbedcrypto PRIVATE "${CMAKE_CURRENT_SOURCE_DIR}/src/crypto/random.c")
endif()

# Moonlight's streaming core reads USE_MBEDTLS for itself, and links the mbedcrypto target above.
add_subdirectory(third_party/moonlight-common-c)

add_executable(wittymoonlight-host src/main.c)
target_include_directories(wittymoonlight-host PRIVATE third_party/moonlight-common-c/src)
target_link_libraries(wittymoonlight-host PRIVATE moonlight-common-c)
set_target_properties(wittymoonlight-host PROPERTIES C_STANDARD 11)
if(NOT MSVC)
  target_compile_options(wittymoonlight-host PRIVATE -Wall -Wextra -O2)
endif()
if(USE_MBEDTLS AND WIN32)
  target_sources(wittymoonlight-host PRIVATE src/crypto/threads_windows.c)
  target_compile_definitions(wittymoonlight-host PRIVATE WITTY_CRYPTO_THREADS)
  target_link_libraries(wittymoonlight-host PRIVATE mbedcrypto)
endif()

if(MINGW)
  # A single file that needs only what's part of Windows, and takes ASLR and DEP in full.
  target_link_options(wittymoonlight-host PRIVATE -static -fstack-protector-strong -Wl,--gc-sections,--dynamicbase,--nxcompat,--high-entropy-va)
elseif(APPLE)
  target_link_options(wittymoonlight-host PRIVATE -Wl,-dead_strip)
elseif(NOT MSVC)
  target_link_options(wittymoonlight-host PRIVATE -Wl,--gc-sections,-z,relro,-z,now,-z,noexecstack)
endif()
if(WITTY_STATIC)
  # musl's threads get 128 KiB by default, which is a lot less than the core has been run with.
  target_link_options(wittymoonlight-host PRIVATE -static-pie -Wl,-z,stack-size=1048576)
endif()
if(NOT MSVC AND NOT APPLE)
  target_link_options(wittymoonlight-host PRIVATE $<$<CONFIG:Release>:-s>)
endif()
