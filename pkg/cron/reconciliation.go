package cron

import (
	"fmt"
	"log"
	"time"

	"bsm/pkg/dispatch"
	"bsm/pkg/store"
)

type ReconciliationJob struct {
	pgStore *store.PGStore
	engine  *dispatch.Engine
}

func NewReconciliationJob(pg *store.PGStore, engine *dispatch.Engine) *ReconciliationJob {
	return &ReconciliationJob{
		pgStore: pg,
		engine:  engine,
	}
}

func safeID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func (r *ReconciliationJob) Start() {
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		log.Println("🔍 [Reconciliation Job] Cron Job Phục Hồi Đền Bù đang chạy...")
		for range ticker.C {
			r.reconcileStuckPendingBookings()
		}
	}()
}

func (r *ReconciliationJob) reconcileStuckPendingBookings() {
	stuckBookings, err := r.pgStore.GetStuckPendingBookings(15 * time.Second)
	if err != nil {
		log.Printf("⚠️ [Cron Job] Lỗi kiểm tra cuốc xe kẹt: %v", err)
		return
	}

	for _, b := range stuckBookings {
		log.Printf(fmt.Sprintf("🔄 [Cron Job] Phát hiện Booking #%s bị kẹt PENDING > 15s ➔ Phục hồi lại vào Dispatch Engine", safeID(b.ID)))
		r.engine.EnqueueBooking(b.ID)
	}
}
