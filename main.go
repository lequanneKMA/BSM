package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bsm/pkg/cron"
	"bsm/pkg/dispatch"
	"bsm/pkg/models"
	"bsm/pkg/mq"
	"bsm/pkg/outbox"
	"bsm/pkg/store"
	"bsm/pkg/ws"

	"github.com/gorilla/websocket"
)

var (
	memStore       *store.Store
	redisStore     *store.RedisStore
	pgStore        *store.PGStore
	mqClient       *mq.MQClient
	wsHub          *ws.Hub
	dispatchEngine *dispatch.Engine
)

func safeID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func main() {
	rand.Seed(time.Now().UnixNano())

	memStore = store.NewStore()
	wsHub = ws.NewHub()

	initEnterpriseServices()

	if pgStore != nil {
		pgStore.ClearOldBookings()
		log.Println("🧹 [PostgreSQL] Đã dọn dẹp toàn bộ dữ liệu cuốc xe cũ từ các lượt test trước!")
	}

	dispatchEngine = dispatch.NewEngine(memStore, redisStore, pgStore, wsHub)

	go wsHub.Run()
	dispatchEngine.Start()

	if pgStore != nil && mqClient != nil {
		outboxPub := outbox.NewPublisher(pgStore, mqClient)
		outboxPub.Start()

		cronJob := cron.NewReconciliationJob(pgStore, dispatchEngine)
		cronJob.Start()

		mqClient.Consume(func(body []byte) {
			var b models.Booking
			if err := json.Unmarshal(body, &b); err == nil {
				log.Printf("📥 [RabbitMQ Consumer] Nhận message booking.created cho Booking #%s", safeID(b.ID))
				dispatchEngine.EnqueueBooking(b.ID)
			}
		})
	}

	// Seed 100 sample drivers across Hanoi
	seedHanoiDrivers()

	// Start realistic idle driver patrol GPS simulation
	startIdlePatrolSimulation()

	http.HandleFunc("/ws", handleWS)
	http.HandleFunc("/api/drivers", handleDrivers)
	http.HandleFunc("/api/drivers/", handleDriverAction)
	http.HandleFunc("/api/bookings", handleBookings)
	http.HandleFunc("/api/bookings/", handleBookingAction)
	http.HandleFunc("/api/simulation/stress-test", handleStressTest)
	http.HandleFunc("/api/stats", handleStats)
	http.HandleFunc("/api/admin/infra-status", handleAdminInfraStatus)
	http.HandleFunc("/api/admin/clear-cooldowns", handleAdminClearCooldowns)
	http.HandleFunc("/api/admin/clear-bookings", handleAdminClearBookings)
	http.HandleFunc("/api/admin/drivers/deposit-all", handleAdminDepositAll)
	http.HandleFunc("/api/admin/drivers/reset-fatigue", handleAdminResetFatigue)
	http.HandleFunc("/api/admin/drivers/auto-accept-all", handleAdminAutoAcceptAll)

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("🚀 [Server Started] Dashboard Hà Nội running at: http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func initEnterpriseServices() {
	var err error
	pgConnStr := "postgres://bsm_user:bsm_password@localhost:5432/bsm_db?sslmode=disable"
	pgStore, err = store.NewPGStore(pgConnStr)
	if err != nil {
		log.Printf("ℹ️ [System] PostgreSQL chưa bật (%v) -> Dùng Fallback In-Memory", err)
		pgStore = nil
	} else {
		log.Println("🐘 [PostgreSQL] Kết nối Database thành công! (Transactional Outbox Active)")
	}

	redisAddr := "localhost:6379"
	redisStore, err = store.NewRedisStore(redisAddr)
	if err != nil {
		log.Printf("ℹ️ [System] Redis chưa bật (%v) -> Dùng Fallback In-Memory", err)
		redisStore = nil
	} else {
		log.Println("🔴 [Redis] Kết nối Redis Cluster thành công! (SETNX Lock Active)")
	}

	rabbitURL := "amqp://guest:guest@localhost:5672/"
	mqClient, err = mq.NewMQClient(rabbitURL)
	if err != nil {
		log.Printf("ℹ️ [System] RabbitMQ chưa bật (%v) -> Dùng Fallback Channel Queue", err)
		mqClient = nil
	} else {
		log.Println("🐰 [RabbitMQ] Kết nối RabbitMQ Exchange thành công! (Queue Active)")
	}
}

func seedHanoiDrivers() {
	firstNames := []string{"Nguyễn", "Trần", "Lê", "Phạm", "Hoàng", "Phan", "Vũ", "Đặng", "Bùi", "Đỗ"}
	middleNames := []string{"Văn", "Thị", "Hoàng", "Minh", "Đức", "Anh", "Quang", "Tuấn", "Thành", "Hữu"}
	lastNames := []string{"A", "B", "C", "D", "Hùng", "Dũng", "Nam", "Hải", "Phong", "Thắng", "Kiên", "Linh", "Sơn"}
	vehicles := []string{"Xe Máy 🛵", "Ô tô 4 chỗ 🚗", "Ô tô 7 chỗ 🚙"}

	// Hanoi Center Coordinates: ~21.0285, 105.8542
	baseLat := 21.0285
	baseLng := 105.8542

	for i := 1; i <= 20; i++ {
		name := fmt.Sprintf("Tài xế %s %s %s",
			firstNames[rand.Intn(len(firstNames))],
			middleNames[rand.Intn(len(middleNames))],
			lastNames[rand.Intn(len(lastNames))],
		)

		// Spread drivers randomly across Hanoi (radius ~ 4-5 km)
		latOffset := (rand.Float64() - 0.5) * 0.08
		lngOffset := (rand.Float64() - 0.5) * 0.08

		// Random metrics
		rating := 4.5 + rand.Float64()*0.5
		rating = float64(int(rating*10)) / 10.0 // Round to 1 decimal

		acceptanceRate := 82.0 + rand.Float64()*18.0
		acceptanceRate = float64(int(acceptanceRate*10)) / 10.0

		totalTrips := 120 + rand.Intn(2800)
		vehicle := vehicles[rand.Intn(len(vehicles))]

		// 15 Auto Accept, 5 Manual Accept
		botMode := models.BotModeAutoAccept
		if i > 15 {
			botMode = models.BotModeManual
		}

		driver := models.Driver{
			ID:             fmt.Sprintf("drv_hn_%d", i),
			Name:           name,
			Position:       models.Position{Lat: baseLat + latOffset, Lng: baseLng + lngOffset},
			Status:         models.DriverStatusIdle,
			AutoBotMode:    botMode,
			Rating:         rating,
			AcceptanceRate: acceptanceRate,
			TotalTrips:     totalTrips,
			VehicleType:    vehicle,
			WalletBalance:  int64(50000 + rand.Intn(200000)), // Ví chiết khấu mẫu 50k - 250k
			DrivingMinutes: rand.Intn(180),                  // Lái xe 0 - 180 phút
			LastMovedAt:    time.Now(),
		}

		memStore.AddDriver(driver)
	}

	log.Printf("🚕 [Hà Nội Setup] Đã nạp thành công 20 tài xế mẫu với dữ liệu Ví tiền & Thời gian lái xe!")
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	wsHub.ServeWS(w, r, func(conn *websocket.Conn) {
		conn.WriteJSON(models.WSMessage{
			Type: models.WSMsgInit,
			Payload: map[string]interface{}{
				"drivers":  memStore.GetAllDrivers(),
				"bookings": memStore.GetAllBookings(),
				"stats":    memStore.GetStats(),
			},
		})
	})
}

func handleDrivers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(memStore.GetAllDrivers())
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Name     string             `json:"name"`
			Position models.Position    `json:"position"`
			BotMode  models.AutoBotMode `json:"autoBotMode"`
		}
		vehicles := []string{"Xe Máy 🛵", "Ô tô 4 chỗ 🚗", "Ô tô 7 chỗ 🚙"}
		if req.Position.Lat == 0 {
			req.Position = models.Position{
				Lat: 21.0285 + (rand.Float64()-0.5)*0.06,
				Lng: 105.8542 + (rand.Float64()-0.5)*0.06,
			}
		}

		driverID := fmt.Sprintf("drv_%d", time.Now().UnixNano()%100000)
		if req.Name == "" {
			req.Name = fmt.Sprintf("Tài xế mới #%s", driverID[4:])
		}
		if req.BotMode == "" {
			req.BotMode = models.BotModeAutoAccept
		}

		driver := models.Driver{
			ID:             driverID,
			Name:           req.Name,
			Position:       req.Position,
			Status:         models.DriverStatusIdle,
			AutoBotMode:    req.BotMode,
			Rating:         4.5 + rand.Float64()*0.5,
			AcceptanceRate: 85.0 + rand.Float64()*15.0,
			TotalTrips:     100 + rand.Intn(1000),
			VehicleType:    vehicles[rand.Intn(len(vehicles))],
			WalletBalance:  int64(50000 + rand.Intn(200000)),
			DrivingMinutes: rand.Intn(150),
			LastMovedAt:    time.Now(),
		}

		created := memStore.AddDriver(driver)
		wsHub.Broadcast(models.WSMessage{
			Type:    models.WSMsgDriverUpdated,
			Payload: created,
		})
		wsHub.Broadcast(models.WSMessage{
			Type:    models.WSMsgStats,
			Payload: memStore.GetStats(),
		})

		json.NewEncoder(w).Encode(created)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleDriverAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/drivers/"), "/")

	if len(parts) == 1 && parts[0] != "" {
		driverID := parts[0]
		if r.Method == http.MethodDelete {
			if memStore.DeleteDriver(driverID) {
				wsHub.Broadcast(models.WSMessage{
					Type:    models.WSMsgDriverRemoved,
					Payload: driverID,
				})
				wsHub.Broadcast(models.WSMessage{
					Type:    models.WSMsgStats,
					Payload: memStore.GetStats(),
				})
				json.NewEncoder(w).Encode(map[string]bool{"success": true})
				return
			}
			http.Error(w, "Driver not found", http.StatusNotFound)
			return
		}
		if r.Method == http.MethodPut || r.Method == http.MethodPatch {
			var req struct {
				Name           string             `json:"name"`
				VehicleType    string             `json:"vehicleType"`
				Rating         float64            `json:"rating"`
				AcceptanceRate float64            `json:"acceptanceRate"`
				WalletBalance  int64              `json:"walletBalance"`
				DrivingMinutes int                `json:"drivingMinutes"`
				AutoBotMode    models.AutoBotMode `json:"autoBotMode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				if d, ok := memStore.GetDriver(driverID); ok {
					if req.Name != "" {
						d.Name = req.Name
					}
					if req.VehicleType != "" {
						d.VehicleType = req.VehicleType
					}
					if req.Rating > 0 {
						d.Rating = req.Rating
					}
					if req.AcceptanceRate > 0 {
						d.AcceptanceRate = req.AcceptanceRate
					}
					if req.WalletBalance >= 0 {
						d.WalletBalance = req.WalletBalance
					}
					if req.DrivingMinutes >= 0 {
						d.DrivingMinutes = req.DrivingMinutes
					}
					if req.AutoBotMode != "" {
						d.AutoBotMode = req.AutoBotMode
					}
					updated := memStore.UpdateDriver(*d)
					wsHub.Broadcast(models.WSMessage{
						Type:    models.WSMsgDriverUpdated,
						Payload: updated,
					})
					json.NewEncoder(w).Encode(updated)
					return
				}
			}
			http.Error(w, "Driver not found or invalid body", http.StatusBadRequest)
			return
		}
		http.Error(w, "Driver not found", http.StatusNotFound)
		return
	}

	if len(parts) == 2 && parts[1] == "position" && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
		driverID := parts[0]
		var req struct {
			Position models.Position `json:"position"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if driver, ok := memStore.GetDriver(driverID); ok {
				driver.Position = req.Position
				updated := memStore.UpdateDriver(*driver)
				wsHub.Broadcast(models.WSMessage{
					Type:    models.WSMsgDriverUpdated,
					Payload: updated,
				})
				json.NewEncoder(w).Encode(updated)
				return
			}
		}
	}

	if len(parts) == 2 && parts[1] == "bot-mode" && r.Method == http.MethodPost {
		driverID := parts[0]
		var req struct {
			BotMode models.AutoBotMode `json:"autoBotMode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if driver, ok := memStore.GetDriver(driverID); ok {
				driver.AutoBotMode = req.BotMode
				updated := memStore.UpdateDriver(*driver)
				wsHub.Broadcast(models.WSMessage{
					Type:    models.WSMsgDriverUpdated,
					Payload: updated,
				})
				json.NewEncoder(w).Encode(updated)
				return
			}
		}
	}

	if len(parts) == 1 && r.Method == http.MethodDelete {
		driverID := parts[0]
		deleted := memStore.DeleteDriver(driverID)
		if deleted {
			wsHub.Broadcast(models.WSMessage{
				Type:    models.WSMsgDriverRemoved,
				Payload: driverID,
			})
			wsHub.Broadcast(models.WSMessage{
				Type:    models.WSMsgStats,
				Payload: memStore.GetStats(),
			})
		}
		json.NewEncoder(w).Encode(map[string]bool{"success": deleted})
		return
	}

	http.Error(w, "Invalid route", http.StatusBadRequest)
}

func handleBookings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(memStore.GetAllBookings())
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			CustomerPos    models.Position `json:"customerPos"`
			DestinationPos models.Position `json:"destinationPos"`
			VehicleType    string          `json:"vehicleType"`
			PaymentMethod  string          `json:"paymentMethod"`
			CustomerTier   string          `json:"customerTier"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		// Default Hanoi Center if zero
		if req.CustomerPos.Lat == 0 {
			req.CustomerPos = models.Position{
				Lat: 21.0285 + (rand.Float64()-0.5)*0.03,
				Lng: 105.8542 + (rand.Float64()-0.5)*0.03,
			}
		}

		if req.PaymentMethod == "" {
			req.PaymentMethod = "CASH"
		}
		if req.CustomerTier == "" {
			req.CustomerTier = "REGULAR"
		}

		bookingID := fmt.Sprintf("bk_%d", time.Now().UnixNano()%1000000)
		booking := models.Booking{
			ID:             bookingID,
			CustomerID:     fmt.Sprintf("cus_%d", rand.Intn(900)+100),
			CustomerPos:    req.CustomerPos,
			DestinationPos: req.DestinationPos,
			VehicleType:    req.VehicleType,
			PaymentMethod:  req.PaymentMethod,
			CustomerTier:   req.CustomerTier,
			Status:         models.BookingStatusPending,
		}

		created := memStore.AddBooking(booking)

		if pgStore != nil {
			if err := pgStore.CreateBookingWithOutbox(*created); err != nil {
				log.Printf("⚠️ [Postgres Outbox] Lỗi ghi outbox: %v", err)
			} else {
				log.Printf("📦 [Postgres Outbox] Đã ghi Booking #%s + Outbox Event thành công!", safeID(created.ID))
			}
		} else {
			dispatchEngine.EnqueueBooking(booking.ID)
		}

		wsHub.Broadcast(models.WSMessage{
			Type:    models.WSMsgBookingUpdated,
			Payload: created,
		})

		json.NewEncoder(w).Encode(created)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleBookingAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/bookings/"), "/")

	if len(parts) == 2 && parts[1] == "respond" && r.Method == http.MethodPost {
		bookingID := parts[0]
		var req struct {
			DriverID string                `json:"driverId"`
			Action   dispatch.DriverAction `json:"action"`
			Token    string                `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			dispatchEngine.RespondToBooking(bookingID, req.DriverID, req.Action, req.Token)
			json.NewEncoder(w).Encode(map[string]bool{"queued": true})
			return
		}
	}

	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		bookingID := parts[0]
		var req struct {
			Reason      string `json:"reason"`
			CancelledBy string `json:"cancelledBy"` // "CUSTOMER" or "DRIVER"
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.CancelledBy == "" {
			req.CancelledBy = "CUSTOMER"
		}
		if req.Reason == "" {
			req.Reason = "Khách hàng đổi ý"
		}

		success := dispatchEngine.CancelBooking(bookingID, req.Reason, req.CancelledBy)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": success, "bookingId": bookingID})
		return
	}

	if len(parts) == 2 && parts[1] == "complete" && r.Method == http.MethodPost {
		bookingID := parts[0]
		success := dispatchEngine.CompleteBooking(bookingID)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": success, "bookingId": bookingID})
		return
	}

	http.Error(w, "Invalid action", http.StatusBadRequest)
}

func handleStressTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	driverCount := 5
	bookingCount := 5

	if qVal := r.URL.Query().Get("drivers"); qVal != "" {
		if v, err := strconv.Atoi(qVal); err == nil {
			driverCount = v
		}
	}
	if qVal := r.URL.Query().Get("bookings"); qVal != "" {
		if v, err := strconv.Atoi(qVal); err == nil {
			bookingCount = v
		}
	}

	vehicles := []string{"Xe Máy 🛵", "Ô tô 4 chỗ 🚗", "Ô tô 7 chỗ 🚙"}

	for i := 0; i < driverCount; i++ {
		driverID := fmt.Sprintf("drv_st_%d", time.Now().UnixNano()%100000+int64(i))
		d := models.Driver{
			ID:   driverID,
			Name: fmt.Sprintf("Tài xế BãoCuốc #%d", i+1),
			Position: models.Position{
				Lat: 21.0285 + (rand.Float64()-0.5)*0.05,
				Lng: 105.8542 + (rand.Float64()-0.5)*0.05,
			},
			Status:         models.DriverStatusIdle,
			AutoBotMode:    models.BotModeAutoAccept,
			Rating:         4.8,
			AcceptanceRate: 95.0,
			TotalTrips:     500,
			VehicleType:    vehicles[rand.Intn(len(vehicles))],
			WalletBalance:  int64(100000 + rand.Intn(150000)),
			DrivingMinutes: rand.Intn(100),
		}
		created := memStore.AddDriver(d)
		wsHub.Broadcast(models.WSMessage{
			Type:    models.WSMsgDriverUpdated,
			Payload: created,
		})
	}

	go func() {
		for i := 0; i < bookingCount; i++ {
			time.Sleep(1200 * time.Millisecond)
			bookingID := fmt.Sprintf("bk_st_%d", time.Now().UnixNano()%100000+int64(i))
			b := models.Booking{
				ID:         bookingID,
				CustomerID: fmt.Sprintf("cus_st_%d", i+1),
				CustomerPos: models.Position{
					Lat: 21.0285 + (rand.Float64()-0.5)*0.04,
					Lng: 105.8542 + (rand.Float64()-0.5)*0.04,
				},
				DestinationPos: models.Position{
					Lat: 21.0350 + (rand.Float64()-0.5)*0.04,
					Lng: 105.8600 + (rand.Float64()-0.5)*0.04,
				},
				Status:        models.BookingStatusPending,
				VehicleType:   vehicles[rand.Intn(len(vehicles))],
				PaymentMethod: "CASH",
				CustomerTier:  "REGULAR",
			}
			created := memStore.AddBooking(b)

			if pgStore != nil {
				pgStore.CreateBookingWithOutbox(*created)
			} else {
				dispatchEngine.EnqueueBooking(created.ID)
			}

			wsHub.Broadcast(models.WSMessage{
				Type:    models.WSMsgBookingUpdated,
				Payload: created,
			})
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"spawnedDrivers":  driverCount,
		"spawnedBookings": bookingCount,
	})
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(memStore.GetStats())
}

func handleAdminInfraStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	pgConn := pgStore != nil
	redisConn := redisStore != nil
	mqConn := mqClient != nil

	outboxCount := 0
	if pgConn {
		outboxCount = pgStore.GetOutboxCount()
	}

	cooldownCount := 0
	if redisConn {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cooldownCount = redisStore.GetActiveCooldownCount(ctx)
		cancel()
	}

	resp := map[string]interface{}{
		"postgresStatus": pgConn,
		"redisStatus":    redisConn,
		"rabbitmqStatus": mqConn,
		"cronActive":     pgConn,
		"outboxEvents":   outboxCount,
		"cooldownKeys":   cooldownCount,
		"totalDrivers":   len(memStore.GetAllDrivers()),
		"totalBookings":  len(memStore.GetAllBookings()),
		"goroutines":     runtime.NumGoroutine(),
	}

	json.NewEncoder(w).Encode(resp)
}

func handleAdminClearCooldowns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cleared := 0
	if redisStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cleared = redisStore.ClearAllCooldowns(ctx)
		cancel()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "clearedKeys": cleared})
}

func handleAdminClearBookings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	memStore.ClearCompletedOrCancelledBookings()
	if pgStore != nil {
		pgStore.ClearOldBookings()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func handleAdminDepositAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	memStore.DepositAllDrivers(100000)
	for _, d := range memStore.GetAllDrivers() {
		wsHub.Broadcast(models.WSMessage{
			Type:    models.WSMsgDriverUpdated,
			Payload: d,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "deposited": 100000})
}

func handleAdminResetFatigue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	memStore.ResetAllDriversFatigue()
	for _, d := range memStore.GetAllDrivers() {
		wsHub.Broadcast(models.WSMessage{
			Type:    models.WSMsgDriverUpdated,
			Payload: d,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func handleAdminAutoAcceptAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	memStore.SetAllDriversAutoAccept()
	for _, d := range memStore.GetAllDrivers() {
		wsHub.Broadcast(models.WSMessage{
			Type:    models.WSMsgDriverUpdated,
			Payload: d,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func startIdlePatrolSimulation() {
	ticker := time.NewTicker(1500 * time.Millisecond)
	go func() {
		for range ticker.C {
			allDrivers := memStore.GetAllDrivers()
			for _, d := range allDrivers {
				if d.Status != models.DriverStatusIdle {
					continue
				}

				// Active Cruising: Xe Máy ~ 65% chance to cruise, Ô tô ~ 40% chance
				isBike := strings.Contains(strings.ToLower(d.VehicleType), "máy") || strings.Contains(strings.ToLower(d.VehicleType), "bike")
				chance := rand.Float64()

				shouldMove := false
				offsetScale := 0.0004
				if isBike && chance < 0.65 {
					shouldMove = true
					offsetScale = 0.0007
				} else if !isBike && chance < 0.40 {
					shouldMove = true
					offsetScale = 0.00045
				}

				if shouldMove {
					latOffset := (rand.Float64() - 0.5) * offsetScale
					lngOffset := (rand.Float64() - 0.5) * offsetScale

					d.Position.Lat += latOffset
					d.Position.Lng += lngOffset
					d.LastMovedAt = time.Now()

					updated := memStore.UpdateDriver(*d)
					wsHub.Broadcast(models.WSMessage{
						Type:    models.WSMsgDriverUpdated,
						Payload: updated,
					})
				}
			}
		}
	}()
}
