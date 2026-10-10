package jobs

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestStartStopsPreviousJob(t *testing.T) {
	r := NewRunner(wrapperspb.String("idle"))
	first := r.Start(wrapperspb.String("running"), func(j *Job[*wrapperspb.StringValue]) {
		<-j.Ctx.Done()
		j.Update(func(st *wrapperspb.StringValue) { st.Value = "stopped" })
	})
	r.Start(wrapperspb.String("running"), func(j *Job[*wrapperspb.StringValue]) {
		j.Update(func(st *wrapperspb.StringValue) { st.Value = "done" })
	})
	select {
	case <-first.Done():
	default:
		t.Fatal("first job still running after a new Start")
	}
	if got := r.Wait(context.Background(), 1, 1, 1).Value; got != "done" {
		t.Fatalf("status = %q, want done", got)
	}
}

func TestSleepCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if Sleep(ctx, time.Second) || time.Since(start) > 100*time.Millisecond {
		t.Fatal("Sleep ignored cancellation")
	}
}
