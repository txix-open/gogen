package main

import (
	random "math/rand"
	"sync"
	"time"
)

var (
	rand *random.Rand
)

func init() {
	src := random.NewSource(time.Now().UnixNano() / 2).(random.Source64)
	rand = random.New(&lockedSource{src: src})
}

type lockedSource struct {
	lk  sync.Mutex
	src random.Source64
}

func (r *lockedSource) Int63() (n int64) {
	r.lk.Lock()
	n = r.src.Int63()
	r.lk.Unlock()
	return
}

func (r *lockedSource) Uint64() (n uint64) {
	r.lk.Lock()
	n = r.src.Uint64()
	r.lk.Unlock()
	return
}

func (r *lockedSource) Seed(seed int64) {
	r.lk.Lock()
	r.src.Seed(seed)
	r.lk.Unlock()
}
