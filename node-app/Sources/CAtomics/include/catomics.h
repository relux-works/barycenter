// Acquire/release int64 atomics over raw memory for the SPSC ring buffer.
// void* signatures because Swift cannot express _Atomic fields directly.
#ifndef DUET_CATOMICS_H
#define DUET_CATOMICS_H

#include <stdatomic.h>
#include <stdint.h>

static inline int64_t ca_load_acquire(const volatile void *p) {
    return atomic_load_explicit((const volatile _Atomic int64_t *)p, memory_order_acquire);
}

static inline void ca_store_release(volatile void *p, int64_t v) {
    atomic_store_explicit((volatile _Atomic int64_t *)p, v, memory_order_release);
}

#endif
