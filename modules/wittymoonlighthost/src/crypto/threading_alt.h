// Mbed TLS's locks on Windows, where it has none of its own: see threads_windows.c.
#pragma once

typedef struct mbedtls_threading_mutex_t {
    void *srw; // the size of an SRWLOCK, without bringing all of windows.h into Mbed TLS
} mbedtls_threading_mutex_t;
