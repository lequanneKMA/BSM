package dispatch_test

import (
	"testing"

	"bsm/pkg/dispatch"
	"bsm/pkg/models"
	"bsm/pkg/store"
	"bsm/pkg/ws"
)

func TestFindAndRankDriversAdvanced_FatigueFilter(t *testing.T) {
	memStore := store.NewStore()

	// Driver 1: Idle, 100 mins driving (Eligible)
	memStore.AddDriver(models.Driver{
		ID:             "drv_active",
		Name:           "Tài xế Khỏe Mạnh",
		Position:       models.Position{Lat: 21.0285, Lng: 105.8542},
		Status:         models.DriverStatusIdle,
		Rating:         4.8,
		AcceptanceRate: 90.0,
		DrivingMinutes: 100,
		WalletBalance:  100000,
	})

	// Driver 2: Idle, 250 mins driving (Exceeded 240m continuous driving limit -> Ineligible)
	memStore.AddDriver(models.Driver{
		ID:             "drv_tired",
		Name:           "Tài xế Mệt Mỏi",
		Position:       models.Position{Lat: 21.0285, Lng: 105.8542},
		Status:         models.DriverStatusIdle,
		Rating:         5.0,
		AcceptanceRate: 100.0,
		DrivingMinutes: 250,
		WalletBalance:  500000,
	})

	pos := models.Position{Lat: 21.0285, Lng: 105.8542}
	candidates := memStore.FindAndRankDriversAdvanced(pos, "", "CASH", "REGULAR", map[string]bool{})

	if len(candidates) != 1 {
		t.Fatalf("Kỳ vọng 1 tài xế đủ điều kiện, thực tế: %d", len(candidates))
	}

	if candidates[0].ID != "drv_active" {
		t.Errorf("Kỳ vọng gán cho drv_active, thực tế: %s", candidates[0].ID)
	}
}

func TestFindAndRankDriversAdvanced_WalletBalanceFilter(t *testing.T) {
	memStore := store.NewStore()

	// Driver 1: Wallet 5,000 VND (< 20,000 min deposit for CASH trip)
	memStore.AddDriver(models.Driver{
		ID:             "drv_low_wallet",
		Name:           "Tài xế Ít Tiền Ví",
		Position:       models.Position{Lat: 21.0285, Lng: 105.8542},
		Status:         models.DriverStatusIdle,
		WalletBalance:  5000,
		Rating:         5.0,
		AcceptanceRate: 100.0,
	})

	// Driver 2: Wallet 50,000 VND (>= 20,000)
	memStore.AddDriver(models.Driver{
		ID:             "drv_ok_wallet",
		Name:           "Tài xế Đủ Tiền Ví",
		Position:       models.Position{Lat: 21.0285, Lng: 105.8542},
		Status:         models.DriverStatusIdle,
		WalletBalance:  50000,
		Rating:         4.5,
		AcceptanceRate: 85.0,
	})

	pos := models.Position{Lat: 21.0285, Lng: 105.8542}
	candidates := memStore.FindAndRankDriversAdvanced(pos, "", "CASH", "REGULAR", map[string]bool{})

	if len(candidates) != 1 {
		t.Fatalf("Kỳ vọng 1 tài xế đủ tiền ví nhận đơn CASH, thực tế: %d", len(candidates))
	}

	if candidates[0].ID != "drv_ok_wallet" {
		t.Errorf("Kỳ vọng drv_ok_wallet nhận đơn CASH, thực tế: %s", candidates[0].ID)
	}
}

func TestFindAndRankDriversAdvanced_VIPBoost(t *testing.T) {
	memStore := store.NewStore()

	// Driver 1: Further distance (1.0 km), Rating 4.0
	memStore.AddDriver(models.Driver{
		ID:             "drv_1",
		Name:           "Tài xế 1",
		Position:       models.Position{Lat: 21.0375, Lng: 105.8542},
		Status:         models.DriverStatusIdle,
		Rating:         4.0,
		AcceptanceRate: 80.0,
		WalletBalance:  100000,
	})

	pos := models.Position{Lat: 21.0285, Lng: 105.8542}

	// Regular Tier Search
	candidatesReg := memStore.FindAndRankDriversAdvanced(pos, "", "CARD", "REGULAR", map[string]bool{})
	scoreReg := candidatesReg[0].Score

	// VIP Tier Search (Gets +10 Boost)
	candidatesVIP := memStore.FindAndRankDriversAdvanced(pos, "", "CARD", "VIP", map[string]bool{})
	scoreVIP := candidatesVIP[0].Score

	if scoreVIP <= scoreReg {
		t.Errorf("Kỳ vọng Khách VIP có điểm Score cao hơn Regular (+10 pts boost). Regular: %.1f, VIP: %.1f", scoreReg, scoreVIP)
	}
}

func TestCancelBooking_CustomerCancellation(t *testing.T) {
	memStore := store.NewStore()
	hub := ws.NewHub()

	engine := dispatch.NewEngine(memStore, nil, nil, hub)
	engine.Start()

	driver := models.Driver{
		ID:             "drv_cust_cancel",
		Name:           "Tài xế Bị Khách Hủy",
		Position:       models.Position{Lat: 21.0285, Lng: 105.8542},
		Status:         models.DriverStatusAssigning,
		Rating:         4.9,
		AcceptanceRate: 98.0,
		WalletBalance:  100000,
	}
	memStore.AddDriver(driver)

	booking := models.Booking{
		ID:          "bk_cust_cancel_1",
		CustomerID:  "cus_test_101",
		CustomerPos: models.Position{Lat: 21.0285, Lng: 105.8542},
		DriverID:    "drv_cust_cancel",
		Status:      models.BookingStatusAssigning,
	}
	memStore.AddBooking(booking)

	// Cancel booking by customer (Reason: Khách đợi quá lâu)
	success := engine.CancelBooking("bk_cust_cancel_1", "Khách đợi quá lâu / Đặt lại", "CUSTOMER")
	if !success {
		t.Fatalf("Kỳ vọng Hủy cuốc bởi Khách hàng thành công")
	}

	bUpdated, _ := memStore.GetBooking("bk_cust_cancel_1")
	if bUpdated.Status != models.BookingStatusCancelled {
		t.Errorf("Kỳ vọng trạng thái Booking là CANCELLED, thực tế: %s", bUpdated.Status)
	}
	if bUpdated.CancelReason != "Khách đợi quá lâu / Đặt lại" {
		t.Errorf("Kỳ vọng lý do hủy khớp với lý do của Khách hàng, thực tế: %s", bUpdated.CancelReason)
	}

	dUpdated, _ := memStore.GetDriver("drv_cust_cancel")
	if dUpdated.Status != models.DriverStatusIdle {
		t.Errorf("Kỳ vọng trạng thái Tài xế quay về IDLE, thực tế: %s", dUpdated.Status)
	}
}

func TestCancelBooking_DriverCancellation(t *testing.T) {
	memStore := store.NewStore()
	hub := ws.NewHub()

	engine := dispatch.NewEngine(memStore, nil, nil, hub)
	engine.Start()

	driver := models.Driver{
		ID:             "drv_self_cancel",
		Name:           "Tài xế Tự Hủy Cuốc",
		Position:       models.Position{Lat: 21.0285, Lng: 105.8542},
		Status:         models.DriverStatusAssigning,
		Rating:         4.5,
		AcceptanceRate: 88.0,
		WalletBalance:  100000,
	}
	memStore.AddDriver(driver)

	booking := models.Booking{
		ID:          "bk_drv_cancel_1",
		CustomerID:  "cus_test_202",
		CustomerPos: models.Position{Lat: 21.0285, Lng: 105.8542},
		DriverID:    "drv_self_cancel",
		Status:      models.BookingStatusAssigning,
	}
	memStore.AddBooking(booking)

	// Cancel booking by driver (Reason: Xe hỏng / Thủng lốp) -> Triggers Auto-Redispatch to next driver
	success := engine.CancelBooking("bk_drv_cancel_1", "Tài xế gặp sự cố xe / Thủng lốp", "DRIVER")
	if !success {
		t.Fatalf("Kỳ vọng Hủy cuốc bởi Tài xế thành công")
	}

	bUpdated, _ := memStore.GetBooking("bk_drv_cancel_1")
	if bUpdated.Status != models.BookingStatusPending && bUpdated.Status != models.BookingStatusAssigning {
		t.Errorf("Kỳ vọng Booking chuyển về PENDING/ASSIGNING để tự động tìm tài xế mới, thực tế: %s", bUpdated.Status)
	}

	dUpdated, _ := memStore.GetDriver("drv_self_cancel")
	if dUpdated.Status != models.DriverStatusIdle {
		t.Errorf("Kỳ vọng tài xế hủy cuốc quay về trạng thái IDLE, thực tế: %s", dUpdated.Status)
	}
}

func TestCompleteBooking(t *testing.T) {
	memStore := store.NewStore()
	hub := ws.NewHub()

	engine := dispatch.NewEngine(memStore, nil, nil, hub)
	engine.Start()

	driver := models.Driver{
		ID:         "drv_comp_1",
		Name:       "Tài xế Hoàn Thành",
		Status:     models.DriverStatusBusy,
		TotalTrips: 10,
	}
	memStore.AddDriver(driver)

	booking := models.Booking{
		ID:       "bk_comp_1",
		DriverID: "drv_comp_1",
		Status:   models.BookingStatusAccepted,
	}
	memStore.AddBooking(booking)

	success := engine.CompleteBooking("bk_comp_1")
	if !success {
		t.Fatalf("Kỳ vọng CompleteBooking thành công")
	}

	bUpdated, _ := memStore.GetBooking("bk_comp_1")
	if bUpdated.Status != models.BookingStatusCompleted {
		t.Errorf("Kỳ vọng Booking status là COMPLETED, thực tế: %s", bUpdated.Status)
	}

	dUpdated, _ := memStore.GetDriver("drv_comp_1")
	if dUpdated.Status != models.DriverStatusIdle {
		t.Errorf("Kỳ vọng tài xế được giải phóng về IDLE, thực tế: %s", dUpdated.Status)
	}
	if dUpdated.TotalTrips != 11 {
		t.Errorf("Kỳ vọng tổng số chuyến tăng lên 11, thực tế: %d", dUpdated.TotalTrips)
	}
}
