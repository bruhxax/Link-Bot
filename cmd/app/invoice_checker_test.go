package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestInvoiceCheckerSkipsOverlappingRuns(t *testing.T) {
	scheduler := setupInvoiceChecker(nil, nil, nil)
	started := make(chan struct{}, 2)
	finish := make(chan struct{})
	var runs atomic.Int32
	id, err := scheduler.AddFunc("*/5 * * * * *", func() {
		runs.Add(1)
		started <- struct{}{}
		<-finish
	})
	if err != nil {
		t.Fatal(err)
	}
	job := scheduler.Entry(id).WrappedJob
	done := make(chan struct{})
	go func() { job.Run(); close(done) }()
	defer func() { close(finish); <-done }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first invoice check did not start")
	}
	skipped := make(chan struct{})
	go func() { job.Run(); close(skipped) }()
	select {
	case <-skipped:
	case <-time.After(time.Second):
		t.Fatal("overlapping invoice check was not skipped")
	}
	if runs.Load() != 1 {
		t.Fatalf("ran %d overlapping checks", runs.Load())
	}
}
