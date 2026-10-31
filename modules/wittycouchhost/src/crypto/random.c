// Where Mbed TLS gets its random numbers: the operating system, with no files involved.
#ifdef __linux__
#define _GNU_SOURCE
#endif

#include <psa/crypto.h>

#if defined(_WIN32)
#include <windows.h>
#include <bcrypt.h>
#elif defined(__APPLE__)
#include <stdlib.h>
#else
#include <errno.h>
#include <sys/random.h>
#endif

psa_status_t mbedtls_psa_external_get_random(mbedtls_psa_external_random_context_t *context, uint8_t *output, size_t output_size, size_t *output_length)
{
    (void)context;
    *output_length = 0;
#if defined(_WIN32)
    if (output_size > 0xFFFFFFFFu || BCryptGenRandom(NULL, output, (ULONG)output_size, BCRYPT_USE_SYSTEM_PREFERRED_RNG) != 0) {
        return PSA_ERROR_INSUFFICIENT_ENTROPY;
    }
#elif defined(__APPLE__)
    arc4random_buf(output, output_size);
#else
    for (size_t done = 0; done < output_size;) {
        ssize_t got = getrandom(output + done, output_size - done, 0);
        if (got < 0) {
            if (errno == EINTR) {
                continue;
            }
            return PSA_ERROR_INSUFFICIENT_ENTROPY;
        }
        done += (size_t)got;
    }
#endif
    *output_length = output_size;
    return PSA_SUCCESS;
}
