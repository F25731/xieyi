package main

import (
	"context"
	"sync/atomic"
	"time"
)

type Stats struct {
	StartedAt        time.Time `json:"startedAt"`
	TotalRequests    uint64    `json:"totalRequests"`
	SuccessRequests  uint64    `json:"successRequests"`
	FailedRequests   uint64    `json:"failedRequests"`
	UpstreamRequests uint64    `json:"upstreamRequests"`
	QueueRejected    uint64    `json:"queueRejected"`
	InFlight         int64     `json:"inFlight"`
	TotalLatencyMs   uint64    `json:"totalLatencyMs"`
	LastError         string    `json:"lastError"`
}

type Dispatcher struct {
	jobs  chan ParseJob
	store *Store
	stats *Stats
}

type ParseJob struct {
	Input string
	APIID string
	Resp  chan ParseResult
}

func NewDispatcher(store *Store, stats *Stats, workers, queueSize int) *Dispatcher {
	d := &Dispatcher{jobs: make(chan ParseJob, queueSize), store: store, stats: stats}
	for i := 0; i < workers; i++ {
		go d.worker()
	}
	return d
}

func (d *Dispatcher) Submit(ctx context.Context, input, apiID string) ParseResult {
	atomic.AddUint64(&d.stats.TotalRequests, 1)
	resp := make(chan ParseResult, 1)
	job := ParseJob{Input: input, APIID: apiID, Resp: resp}
	select {
	case d.jobs <- job:
	case <-ctx.Done():
		return ParseResult{OK: false, Status: 499, Error: ctx.Err().Error()}
	default:
		atomic.AddUint64(&d.stats.QueueRejected, 1)
		return ParseResult{OK: false, Status: 429, Error: "worker queue is full"}
	}
	select {
	case result := <-resp:
		if result.OK {
			atomic.AddUint64(&d.stats.SuccessRequests, 1)
		} else {
			atomic.AddUint64(&d.stats.FailedRequests, 1)
			d.stats.LastError = result.Error
		}
		if result.DurationMs > 0 {
			atomic.AddUint64(&d.stats.TotalLatencyMs, uint64(result.DurationMs))
		}
		return result
	case <-ctx.Done():
		atomic.AddUint64(&d.stats.FailedRequests, 1)
		return ParseResult{OK: false, Status: 499, Error: ctx.Err().Error()}
	}
}

func (d *Dispatcher) worker() {
	for job := range d.jobs {
		atomic.AddInt64(&d.stats.InFlight, 1)
		result := parseMedia(context.Background(), d.store.Get(), d.stats, job.Input, job.APIID)
		atomic.AddInt64(&d.stats.InFlight, -1)
		job.Resp <- result
	}
}

func (d *Dispatcher) QueueLen() int {
	return len(d.jobs)
}

func (d *Dispatcher) QueueCap() int {
	return cap(d.jobs)
}
