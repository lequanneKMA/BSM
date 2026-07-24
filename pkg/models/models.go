package models

import "time"

type DriverStatus string

const (
	DriverStatusIdle      DriverStatus = "IDLE"
	DriverStatusAssigning DriverStatus = "ASSIGNING"
	DriverStatusBusy      DriverStatus = "BUSY"
)

type AutoBotMode string

const (
	BotModeManual      AutoBotMode = "MANUAL"
	BotModeAutoAccept  AutoBotMode = "AUTO_ACCEPT"
	BotModeAutoReject  AutoBotMode = "AUTO_REJECT"
	BotModeAutoTimeout AutoBotMode = "AUTO_TIMEOUT"
)

type BookingStatus string

const (
	BookingStatusPending   BookingStatus = "PENDING"
	BookingStatusAssigning BookingStatus = "ASSIGNING"
	BookingStatusAccepted  BookingStatus = "ACCEPTED"
	BookingStatusRejected  BookingStatus = "REJECTED"
	BookingStatusTimeout   BookingStatus = "TIMEOUT"
	BookingStatusCompleted BookingStatus = "COMPLETED"
	BookingStatusCancelled BookingStatus = "CANCELLED"
	BookingStatusFailed    BookingStatus = "FAILED"
)

type Position struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Driver struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Position         Position     `json:"position"`
	Status           DriverStatus `json:"status"`
	AutoBotMode      AutoBotMode  `json:"autoBotMode"`
	CurrentBookingID string       `json:"currentBookingId,omitempty"`
	Rating           float64      `json:"rating"`           // e.g. 4.8
	AcceptanceRate   float64      `json:"acceptanceRate"`   // e.g. 95.5 (%)
	TotalTrips       int          `json:"totalTrips"`       // e.g. 1250
	VehicleType      string       `json:"vehicleType"`      // e.g. "Xe Máy", "Ô tô 4 chỗ", "Ô tô 7 chỗ"
	Score            float64      `json:"score"`            // Computed dispatch priority score
	WalletBalance    int64        `json:"walletBalance"`    // Ví tiền chiết khấu VND
	DrivingMinutes   int          `json:"drivingMinutes"`   // Thời gian lái xe trong ngày (phút)
	HomeDestination  *Position    `json:"homeDestination,omitempty"` // Vị trí điểm đến tiện đường về nhà
	LastMovedAt      time.Time    `json:"lastMovedAt"`      // Thời điểm ghi nhận GPS di chuyển lần cuối
}

type Booking struct {
	ID                 string        `json:"id"`
	CustomerID         string        `json:"customerId"`
	CustomerPos        Position      `json:"customerPos"`
	DestinationPos     Position      `json:"destinationPos"`
	VehicleType        string        `json:"vehicleType,omitempty"`
	PaymentMethod      string        `json:"paymentMethod,omitempty"` // "CASH", "CARD", "EWALLET"
	CustomerTier       string        `json:"customerTier,omitempty"`  // "VIP", "PLATINUM", "REGULAR"
	CancelReason       string        `json:"cancelReason,omitempty"`  // Lý do hủy cuốc
	DriverID           string        `json:"driverId,omitempty"`
	Status             BookingStatus `json:"status"`
	AssignmentToken    string        `json:"assignmentToken,omitempty"`
	Attempt            int           `json:"attempt"`
	CandidateDriverIDs []string      `json:"candidateDriverIds,omitempty"`
	ExcludedDriverIDs  []string      `json:"excludedDriverIds,omitempty"` // Strictly rejected drivers for this booking
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

type WSMessageType string

const (
	WSMsgInit           WSMessageType = "INIT"
	WSMsgDriverUpdated  WSMessageType = "DRIVER_UPDATED"
	WSMsgDriverRemoved  WSMessageType = "DRIVER_REMOVED"
	WSMsgBookingUpdated WSMessageType = "BOOKING_UPDATED"
	WSMsgBookingRemoved WSMessageType = "BOOKING_REMOVED"
	WSMsgLog            WSMessageType = "LOG"
	WSMsgStats          WSMessageType = "STATS"
)

type WSMessage struct {
	Type    WSMessageType `json:"type"`
	Payload interface{}   `json:"payload"`
}

type LogEntry struct {
	Time    string `json:"time"`
	Message string `json:"message"`
	Level   string `json:"level"` // "info", "success", "warn", "error"
}

type SystemStats struct {
	TotalDrivers     int `json:"totalDrivers"`
	IdleDrivers      int `json:"idleDrivers"`
	BusyDrivers      int `json:"busyDrivers"`
	TotalBookings    int `json:"totalBookings"`
	PendingBookings  int `json:"pendingBookings"`
	AcceptedBookings int `json:"acceptedBookings"`
	FailedBookings   int `json:"failedBookings"`
}
