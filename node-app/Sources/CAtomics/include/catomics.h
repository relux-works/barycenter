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

// CAS for the ring's clear watermark (M7): acq_rel on success, acquire on
// failure. Returns true when *p was *expected and is now desired; otherwise
// the current value is written back into *expected.
static inline _Bool ca_compare_exchange(volatile void *p, int64_t *expected, int64_t desired) {
    return atomic_compare_exchange_strong_explicit(
        (volatile _Atomic int64_t *)p, expected, desired,
        memory_order_acq_rel, memory_order_acquire);
}

#endif
