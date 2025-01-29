package main

import (
	random "math/rand"
	"sync"
	"time"
)

var (
	rand *random.Rand
)

// nolint:gochecknoinits
func init() {
	src, _ := random.NewSource(time.Now().UnixNano()).(random.Source64)
	rand = random.New(&lockedSource{src: src}) // nolint:gosec
}

type lockedSource struct {
	lk  sync.Mutex
	src random.Source64
}

func (r *lockedSource) Int63() int64 {
	r.lk.Lock()
	n := r.src.Int63()
	r.lk.Unlock()
	return n
}

func (r *lockedSource) Uint64() uint64 {
	r.lk.Lock()
	n := r.src.Uint64()
	r.lk.Unlock()
	return n
}

// nolint:nonamedreturns
func (r *lockedSource) Seed(seed int64) {
	r.lk.Lock()
	r.src.Seed(seed)
	r.lk.Unlock()
}
