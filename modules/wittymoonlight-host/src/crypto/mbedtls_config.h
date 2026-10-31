// Mbed TLS with only what Moonlight's streaming core asks it for: AES in GCM and CBC with PKCS#7
// padding. No certificates, no parsers, no public keys and no file access, so less to attack and
// less to carry. Keys only ever live in memory.
#pragma once

#define MBEDTLS_HAVE_ASM
#define MBEDTLS_PLATFORM_C
#define MBEDTLS_AESNI_C
#define MBEDTLS_AESCE_C
#define MBEDTLS_AES_C
#define MBEDTLS_CIPHER_C
#define MBEDTLS_CIPHER_MODE_CBC
#define MBEDTLS_CIPHER_PADDING_PKCS7
#define MBEDTLS_GCM_C
#define MBEDTLS_PSA_CRYPTO_C

// Random numbers come straight from the operating system: see random.c. Mbed TLS's own way can
// fall back to opening /dev/urandom, which this program has given up the right to do.
#define MBEDTLS_PSA_CRYPTO_EXTERNAL_RNG

// The core's threads all use this at once, and its keys live in one store they all share.
#define MBEDTLS_THREADING_C
#ifdef _WIN32
#define MBEDTLS_THREADING_ALT
#else
#define MBEDTLS_THREADING_PTHREAD
#endif
