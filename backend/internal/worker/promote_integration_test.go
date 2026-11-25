//go:build integration

package worker

import (
	"context"
	"errors"
	"testing"
)

func TestSweepAsksForStuckApprovalsToBePromoted(t *testing.T) {
	e := setup(t)
	calls := 0
	e.w.PromoteApproved = func(context.Context) (int, error) { calls++; return 2, nil }
	res, err := e.w.Sweep(context.Background())
	if err != nil || calls != 1 || res.Promoted != 2 {
		t.Fatalf("Sweep = %+v, %v after %d calls", res, err, calls)
	}
}

func TestSweepGoesOnWhenPromotionFails(t *testing.T) {
	e := setup(t)
	e.w.PromoteApproved = func(context.Context) (int, error) { return 0, errors.New("db down") }
	if _, err := e.w.Sweep(context.Background()); err != nil {
		t.Fatalf("a failing promotion stopped the sweep: %v", err)
	}
}
