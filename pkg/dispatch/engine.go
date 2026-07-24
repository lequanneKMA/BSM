package dispatch

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"bsm/pkg/models"
	"bsm/pkg/store"
	"bsm/pkg/ws"
)

type DriverAction string

const (
	ActionAccept DriverAction = "ACCEPT"
	ActionReject DriverAction = "REJECT"
)

type DriverResponse struct {
	BookingID string
	DriverID  string
	Action    DriverAction
	Token     string
}

type Engine struct {
	store              *store.Store
	redisStore         *store.RedisStore
	pgStore            *store.PGStore
	hub                *ws.Hub
	dispatchChan       chan string
	driverResponseChan chan DriverResponse
	activeDispatches   map[string]context.CancelFunc
	dispatchMu         sync.Mutex
}

func NewEngine(s *store.Store, r *store.RedisStore, pg *store.PGStore, h *ws.Hub) *Engine {
	return &Engine{
		store:              s,
		redisStore:         r,
		pgStore:            pg,
		hub:                h,
		dispatchChan:       make(chan string, 100),
		driverResponseChan: make(chan DriverResponse, 100),
		activeDispatches:   make(map[string]context.CancelFunc),
	}
}

func generateToken() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (e *Engine) Start() {
	go func() {
		for bookingID := range e.dispatchChan {
			go e.processBooking(bookingID)
		}
	}()
}

func (e *Engine) EnqueueBooking(bookingID string) {
	e.dispatchChan <- bookingID
}

func (e *Engine) RespondToBooking(bookingID, driverID string, action DriverAction, token string) {
	e.driverResponseChan <- DriverResponse{
		BookingID: bookingID,
		DriverID:  driverID,
		Action:    action,
		Token:     token,
	}
}

func (e *Engine) broadcastLog(msg string, level string) {
	now := time.Now().Format("15:04:05")
	entry := models.LogEntry{
		Time:    now,
		Message: msg,
		Level:   level,
	}
	e.hub.Broadcast(models.WSMessage{
		Type:    models.WSMsgLog,
		Payload: entry,
	})
	e.hub.Broadcast(models.WSMessage{
		Type:    models.WSMsgStats,
		Payload: e.store.GetStats(),
	})
}

func safeID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func (e *Engine) processBooking(bookingID string) {
	// Cancel any existing active dispatch goroutine for this booking
	e.dispatchMu.Lock()
	if oldCancel, ok := e.activeDispatches[bookingID]; ok {
		oldCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.activeDispatches[bookingID] = cancel
	e.dispatchMu.Unlock()

	defer func() {
		e.dispatchMu.Lock()
		delete(e.activeDispatches, bookingID)
		e.dispatchMu.Unlock()
	}()

	booking, found := e.store.GetBooking(bookingID)
	if !found {
		return
	}

	modeStr := "In-Memory"
	if e.redisStore != nil && e.pgStore != nil {
		modeStr = "PostgreSQL + Redis + RabbitMQ"
	}

	e.broadcastLog(fmt.Sprintf("⚡ [Dispatch Engine] Bắt đầu điều phối Booking #%s (%s)", safeID(booking.ID), modeStr), "info")

	// Construct map of excluded drivers (including previously rejected/timed out ones)
	excludedDrivers := make(map[string]bool)
	for _, id := range booking.ExcludedDriverIDs {
		excludedDrivers[id] = true
	}

	// Fetch Customer-Driver Cooldown list from Redis
	if e.redisStore != nil && booking.CustomerID != "" {
		cCtx, cCancel := context.WithTimeout(context.Background(), 2*time.Second)
		cooldownIDs := e.redisStore.GetCustomerCooldownDrivers(cCtx, booking.CustomerID)
		cCancel()
		for _, id := range cooldownIDs {
			excludedDrivers[id] = true
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Re-sync excluded drivers list from store
		if bStore, ok := e.store.GetBooking(booking.ID); ok {
			if bStore.Status == models.BookingStatusCancelled {
				return
			}
			for _, id := range bStore.ExcludedDriverIDs {
				excludedDrivers[id] = true
			}
		}

		// Use Multi-Factor Scoring Engine Advanced (Distance + Rating + Acceptance Rate + Fatigue + Wallet + VIP Boost + My Route)
		candidates := e.store.FindAndRankDriversAdvanced(booking.CustomerPos, booking.VehicleType, booking.PaymentMethod, booking.CustomerTier, excludedDrivers)
		if len(candidates) == 0 {
			e.broadcastLog(fmt.Sprintf("❌ [Dispatch Engine] Không còn tài xế phù hợp cho Booking #%s (Đã hết hoặc tất cả đều từ chối)", safeID(booking.ID)), "error")
			booking.Status = models.BookingStatusFailed
			e.store.UpdateBooking(*booking)
			if e.pgStore != nil {
				e.pgStore.UpdateBookingStatusDirect(booking.ID, string(models.BookingStatusFailed), "", "")
			}

			e.hub.Broadcast(models.WSMessage{
				Type:    models.WSMsgBookingUpdated,
				Payload: booking,
			})
			return
		}

		// Top candidate with highest Score
		driver := candidates[0]
		excludedDrivers[driver.ID] = true
		booking.Attempt++
		token := generateToken()

		dist := store.CalculateDistance(booking.CustomerPos, driver.Position)
		e.broadcastLog(fmt.Sprintf("🎯 [Ranked #1 Candidate] Đề xuất cuốc #%s cho Tài xế %s (Score: %.1f | Cách: %.2f km | ⭐ %.1f | %d%% nhận đơn)",
			safeID(booking.ID), driver.Name, driver.Score, dist, driver.Rating, int(driver.AcceptanceRate)), "info")

		// Redis Reservation Lock if available
		if e.redisStore != nil {
			rCtx, rCancel := context.WithTimeout(context.Background(), 2*time.Second)
			locked, err := e.redisStore.AtomicReserveDriver(rCtx, driver.ID, booking.ID, 30*time.Second)
			rCancel()
			if err != nil || !locked {
				e.broadcastLog(fmt.Sprintf("⚠️ [Redis Lock] Tài xế %s đang bận hoặc đã bị khóa", driver.Name), "warn")
				continue
			}
			e.broadcastLog(fmt.Sprintf("🔒 [Redis SETNX] Đã khóa giữ chỗ cho tài xế %s (30s)", driver.Name), "info")
		}

		// Atomic Reserve Driver
		success := e.store.AtomicReserveDriver(driver.ID, booking.ID, token)
		if !success {
			if e.redisStore != nil {
				e.redisStore.ReleaseDriverReservation(context.Background(), driver.ID)
			}
			e.broadcastLog(fmt.Sprintf("⚠️ [Lock Failed] Tài xế %s không ở trạng thái IDLE", driver.Name), "warn")
			continue
		}

		if e.pgStore != nil {
			e.pgStore.UpdateBookingStatusDirect(booking.ID, string(models.BookingStatusAssigning), driver.ID, token)
		}

		updatedDriver, _ := e.store.GetDriver(driver.ID)
		updatedBooking, _ := e.store.GetBooking(booking.ID)

		e.hub.Broadcast(models.WSMessage{
			Type:    models.WSMsgDriverUpdated,
			Payload: updatedDriver,
		})
		e.hub.Broadcast(models.WSMessage{
			Type:    models.WSMsgBookingUpdated,
			Payload: updatedBooking,
		})

		if updatedDriver.AutoBotMode == models.BotModeAutoAccept {
			go func(bID, dID, tok string) {
				time.Sleep(800 * time.Millisecond)
				e.RespondToBooking(bID, dID, ActionAccept, tok)
			}(booking.ID, driver.ID, token)
		} else if updatedDriver.AutoBotMode == models.BotModeAutoReject {
			go func(bID, dID, tok string) {
				time.Sleep(800 * time.Millisecond)
				e.RespondToBooking(bID, dID, ActionReject, tok)
			}(booking.ID, driver.ID, token)
		}

		// Set 20-second assignment response window
		timeout := time.After(20 * time.Second)
		accepted := false

	waitLoop:
		for {
			select {
			case <-ctx.Done():
				// Dispatch process cancelled externally (e.g. Booking Cancelled or Re-dispatched)
				if e.redisStore != nil {
					e.redisStore.ReleaseDriverReservation(context.Background(), driver.ID)
				}
				e.store.AtomicReleaseDriver(driver.ID)
				return

			case resp := <-e.driverResponseChan:
				if resp.BookingID == booking.ID && resp.DriverID == driver.ID {
					if resp.Action == ActionAccept {
						if e.store.AtomicAcceptBooking(booking.ID, driver.ID, token) {
							accepted = true
							if e.redisStore != nil {
								e.redisStore.ReleaseDriverReservation(context.Background(), driver.ID)
							}
							if e.pgStore != nil {
								e.pgStore.UpdateBookingStatusDirect(booking.ID, string(models.BookingStatusAccepted), driver.ID, token)
							}
							e.broadcastLog(fmt.Sprintf("✅ [ACCEPTED] Tài xế %s đã CHẤP NHẬN cuốc #%s!", driver.Name, safeID(booking.ID)), "success")

							d, _ := e.store.GetDriver(driver.ID)
							b, _ := e.store.GetBooking(booking.ID)

							e.hub.Broadcast(models.WSMessage{
								Type:    models.WSMsgDriverUpdated,
								Payload: d,
							})
							e.hub.Broadcast(models.WSMessage{
								Type:    models.WSMsgBookingUpdated,
								Payload: b,
							})
							break waitLoop
						}
					} else {
						e.broadcastLog(fmt.Sprintf("🚫 [REJECTED] Tài xế %s TỪ CHỐI cuốc #%s ➔ Tự động chuyển đơn sang tài xế tiếp theo!", driver.Name, safeID(booking.ID)), "warn")
						
						// STRICT EXCLUSION: Permanently add driver to excluded list for this booking
						e.store.AddExcludedDriverToBooking(booking.ID, driver.ID)

						if e.redisStore != nil {
							e.redisStore.ReleaseDriverReservation(context.Background(), driver.ID)
						}
						e.store.AtomicReleaseDriver(driver.ID)
						d, _ := e.store.GetDriver(driver.ID)
						e.hub.Broadcast(models.WSMessage{
							Type:    models.WSMsgDriverUpdated,
							Payload: d,
						})
						break waitLoop
					}
				} else {
					go func(r DriverResponse) {
						e.driverResponseChan <- r
					}(resp)
					time.Sleep(50 * time.Millisecond)
				}

			case <-timeout:
				e.broadcastLog(fmt.Sprintf("⏰ [TIMEOUT] Hết 20s tài xế %s không phản hồi cuốc #%s ➔ Tự động chuyển sang tài xế tiếp theo!", driver.Name, safeID(booking.ID)), "warn")
				
				// STRICT EXCLUSION: Add driver to excluded list for this booking
				e.store.AddExcludedDriverToBooking(booking.ID, driver.ID)

				if e.redisStore != nil {
					e.redisStore.ReleaseDriverReservation(context.Background(), driver.ID)
				}
				e.store.AtomicReleaseDriver(driver.ID)
				d, _ := e.store.GetDriver(driver.ID)
				e.hub.Broadcast(models.WSMessage{
					Type:    models.WSMsgDriverUpdated,
					Payload: d,
				})
				break waitLoop
			}
		}

		if accepted {
			return
		}
	}
}

// CancelBooking handles cancellation. If cancelled by DRIVER, it releases the driver and automatically re-dispatches the booking to the next candidate driver for the customer.
func (e *Engine) CancelBooking(bookingID string, reason string, cancelledBy string) bool {
	// Cancel any active processBooking goroutine for this booking
	e.dispatchMu.Lock()
	if cancelFunc, exists := e.activeDispatches[bookingID]; exists {
		cancelFunc()
		delete(e.activeDispatches, bookingID)
	}
	e.dispatchMu.Unlock()

	booking, found := e.store.GetBooking(bookingID)
	if !found {
		return false
	}

	previousDriverID := booking.DriverID
	customerID := booking.CustomerID

	// 1. If CANCELLED BY DRIVER -> Automatically Re-dispatch to another driver!
	if cancelledBy == "DRIVER" {
		booking.CancelReason = fmt.Sprintf("Tài xế hủy: %s", reason)
		if previousDriverID != "" {
			e.store.AddExcludedDriverToBooking(booking.ID, previousDriverID)
			if e.redisStore != nil {
				e.redisStore.ReleaseDriverReservation(context.Background(), previousDriverID)
				if customerID != "" {
					e.redisStore.SetCustomerDriverCooldown(context.Background(), customerID, previousDriverID, 15*time.Minute)
				}
			}
			e.store.AtomicReleaseDriver(previousDriverID)
			if d, ok := e.store.GetDriver(previousDriverID); ok {
				e.hub.Broadcast(models.WSMessage{
					Type:    models.WSMsgDriverUpdated,
					Payload: d,
				})
			}
		}

		// Reset booking status to PENDING and clear DriverID for automatic re-dispatch
		booking.Status = models.BookingStatusPending
		booking.DriverID = ""
		booking.AssignmentToken = ""
		e.store.UpdateBooking(*booking)

		e.broadcastLog(fmt.Sprintf("🔄 [Auto-Redispatch] Tài xế %s HỦY cuốc #%s (Lý do: %s) ➔ Tự động chuyển đơn sang tài xế khác cho khách!", safeID(previousDriverID), safeID(booking.ID), reason), "warn")
		
		e.hub.Broadcast(models.WSMessage{
			Type:    models.WSMsgBookingUpdated,
			Payload: booking,
		})

		// Re-enqueue booking to find next best driver
		go func(bID string) {
			time.Sleep(500 * time.Millisecond)
			e.EnqueueBooking(bID)
		}(booking.ID)

		return true
	}

	// 2. If CANCELLED BY CUSTOMER -> Cancel booking permanently
	booking.Status = models.BookingStatusCancelled
	booking.CancelReason = reason
	e.store.UpdateBooking(*booking)

	if e.pgStore != nil {
		e.pgStore.UpdateBookingStatusDirect(booking.ID, string(models.BookingStatusCancelled), previousDriverID, "")
	}

	// Release driver status if assigned
	if previousDriverID != "" {
		if e.redisStore != nil {
			e.redisStore.ReleaseDriverReservation(context.Background(), previousDriverID)
			if customerID != "" {
				e.redisStore.SetCustomerDriverCooldown(context.Background(), customerID, previousDriverID, 15*time.Minute)
				e.broadcastLog(fmt.Sprintf("🔒 [Anti-Reassignment] Khóa Cooldown (15p) cho cặp Khách %s & Tài xế %s do Hủy cuốc!", customerID, previousDriverID), "warn")
			}
		}
		e.store.AtomicReleaseDriver(previousDriverID)
		if d, ok := e.store.GetDriver(previousDriverID); ok {
			e.hub.Broadcast(models.WSMessage{
				Type:    models.WSMsgDriverUpdated,
				Payload: d,
			})
		}
	}

	e.hub.Broadcast(models.WSMessage{
		Type:    models.WSMsgBookingUpdated,
		Payload: booking,
	})
	e.broadcastLog(fmt.Sprintf("🚫 [CANCELLED] Khách hàng HỦY cuốc #%s (Lý do: %s)", safeID(booking.ID), reason), "warn")
	return true
}

// CompleteBooking marks a booking as COMPLETED, releases the assigned driver back to IDLE, and broadcasts updates.
func (e *Engine) CompleteBooking(bookingID string) bool {
	e.dispatchMu.Lock()
	if cancelFunc, exists := e.activeDispatches[bookingID]; exists {
		cancelFunc()
		delete(e.activeDispatches, bookingID)
	}
	e.dispatchMu.Unlock()

	booking, found := e.store.GetBooking(bookingID)
	if !found {
		return false
	}

	driverID := booking.DriverID
	booking.Status = models.BookingStatusCompleted
	e.store.UpdateBooking(*booking)

	if e.pgStore != nil {
		e.pgStore.UpdateBookingStatusDirect(booking.ID, string(models.BookingStatusCompleted), driverID, "")
	}

	if driverID != "" {
		if e.redisStore != nil {
			e.redisStore.ReleaseDriverReservation(context.Background(), driverID)
		}
		e.store.AtomicReleaseDriver(driverID)
		if d, ok := e.store.GetDriver(driverID); ok {
			d.TotalTrips++
			e.store.UpdateDriver(*d)
			e.hub.Broadcast(models.WSMessage{
				Type:    models.WSMsgDriverUpdated,
				Payload: d,
			})
		}
	}

	e.hub.Broadcast(models.WSMessage{
		Type:    models.WSMsgBookingUpdated,
		Payload: booking,
	})
	e.hub.Broadcast(models.WSMessage{
		Type:    models.WSMsgStats,
		Payload: e.store.GetStats(),
	})
	e.broadcastLog(fmt.Sprintf("🏁 [COMPLETED] Cuốc xe #%s đã HOÀN THÀNH thành công! Tài xế đã rảnh rỗi.", safeID(booking.ID)), "success")
	return true
}
