// Mbed TLS's locks on Windows. This has to run before anything else uses Mbed TLS.
#include <windows.h>

#include <mbedtls/threading.h>

static void lock_init_(mbedtls_threading_mutex_t *m)
{
    InitializeSRWLock((PSRWLOCK)&m->srw);
}

static void lock_free_(mbedtls_threading_mutex_t *m)
{
    (void)m; // an SRW lock holds nothing that needs freeing
}

static int lock_take_(mbedtls_threading_mutex_t *m)
{
    AcquireSRWLockExclusive((PSRWLOCK)&m->srw);
    return 0;
}

static int lock_give_(mbedtls_threading_mutex_t *m)
{
    ReleaseSRWLockExclusive((PSRWLOCK)&m->srw);
    return 0;
}

void crypto_threads_init(void)
{
    mbedtls_threading_set_alt(lock_init_, lock_free_, lock_take_, lock_give_);
}
