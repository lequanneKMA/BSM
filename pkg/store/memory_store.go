package store

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"bsm/pkg/models"
)

type Store struct {
	mu       sync.RWMutex
	drivers  map[string]*models.Driver
	bookings map[string]*models.Booking
}

func NewStore() *Store {
	return &Store{
		drivers:  make(map[string]*models.Driver),
		bookings: make(map[string]*models.Booking),
	}
}

// CalculateDistance returns distance between two points in km using Haversine formula
func CalculateDistance(p1, p2 models.Position) float64 {
	const R = 6371.0 // Earth radius in km
	dLat := (p2.Lat - p1.Lat) * math.Pi / 180.0
	dLng := (p2.Lng - p1.Lng) * math.Pi / 180.0

	lat1 := p1.Lat * math.Pi / 180.0
	lat2 := p2.Lat * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLng/2)*math.Sin(dLng/2)*math.Cos(lat1)*math.Cos(lat2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

func (s *Store) AddDriver(d models.Driver) *models.Driver {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := d
	if copied.Status == "" {
		copied.Status = models.DriverStatusIdle
	}
	if copied.AutoBotMode == "" {
		copied.AutoBotMode = models.BotModeManual
	}
	if copied.Rating == 0 {
		copied.Rating = 4.8
	}
	if copied.AcceptanceRate == 0 {
		copied.AcceptanceRate = 95.0
	}
	if copied.TotalTrips == 0 {
		copied.TotalTrips = 350
	}
	if copied.VehicleType == "" {
		copied.VehicleType = "Xe Máy"
	}
	s.drivers[copied.ID] = &copied
	return &copied
}

func (s *Store) UpdateDriver(d models.Driver) *models.Driver {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.drivers[d.ID]; ok {
		existing.Name = d.Name
		existing.Position = d.Position
		existing.Status = d.Status
		existing.AutoBotMode = d.AutoBotMode
		existing.CurrentBookingID = d.CurrentBookingID
		existing.Rating = d.Rating
		existing.AcceptanceRate = d.AcceptanceRate
		existing.TotalTrips = d.TotalTrips
		existing.VehicleType = d.VehicleType
		existing.Score = d.Score
		return existing
	}
	s.drivers[d.ID] = &d
	return &d
}

func (s *Store) GetDriver(id string) (*models.Driver, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.drivers[id]
	if !ok {
		return nil, false
	}
	copied := *d
	return &copied, true
}

func (s *Store) ClearCompletedOrCancelledBookings() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, b := range s.bookings {
		if b.Status == models.BookingStatusCompleted || b.Status == models.BookingStatusCancelled || b.Status == models.BookingStatusFailed {
			delete(s.bookings, id)
		}
	}
}

func (s *Store) ClearAllBookingsAndResetDrivers() []*models.Driver {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.bookings = make(map[string]*models.Booking)

	updatedDrivers := make([]*models.Driver, 0, len(s.drivers))
	for _, d := range s.drivers {
		d.Status = models.DriverStatusIdle
		d.CurrentBookingID = ""
		copied := *d
		updatedDrivers = append(updatedDrivers, &copied)
	}

	return updatedDrivers
}

func (s *Store) DeleteDriver(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.drivers[id]; ok {
		delete(s.drivers, id)
		return true
	}
	return false
}

func (s *Store) DepositAllDrivers(amount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.drivers {
		d.WalletBalance += amount
	}
}

func (s *Store) ResetAllDriversFatigue() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.drivers {
		d.DrivingMinutes = 0
	}
}

func (s *Store) SetAllDriversAutoAccept() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.drivers {
		d.AutoBotMode = models.BotModeAutoAccept
	}
}

func (s *Store) GetAllDrivers() []*models.Driver {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*models.Driver, 0, len(s.drivers))
	for _, d := range s.drivers {
		copied := *d
		list = append(list, &copied)
	}
	return list
}

func (s *Store) AddBooking(b models.Booking) *models.Booking {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := b
	now := time.Now()
	copied.CreatedAt = now
	copied.UpdatedAt = now
	if copied.Status == "" {
		copied.Status = models.BookingStatusPending
	}
	s.bookings[copied.ID] = &copied
	return &copied
}

func (s *Store) UpdateBooking(b models.Booking) *models.Booking {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.bookings[b.ID]; ok {
		existing.Status = b.Status
		existing.CancelReason = b.CancelReason
		if b.PaymentMethod != "" {
			existing.PaymentMethod = b.PaymentMethod
		}
		if b.CustomerTier != "" {
			existing.CustomerTier = b.CustomerTier
		}
		existing.DriverID = b.DriverID
		existing.AssignmentToken = b.AssignmentToken
		existing.Attempt = b.Attempt
		existing.CandidateDriverIDs = b.CandidateDriverIDs
		existing.ExcludedDriverIDs = b.ExcludedDriverIDs
		existing.UpdatedAt = time.Now()
		return existing
	}
	b.UpdatedAt = time.Now()
	s.bookings[b.ID] = &b
	return &b
}

func (s *Store) GetBooking(id string) (*models.Booking, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.bookings[id]
	if !ok {
		return nil, false
	}
	copied := *b
	return &copied, true
}

func (s *Store) GetAllBookings() []*models.Booking {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*models.Booking, 0, len(s.bookings))
	for _, b := range s.bookings {
		copied := *b
		list = append(list, &copied)
	}
	return list
}

type DriverScoreItem struct {
	Driver   *models.Driver
	Distance float64
	Score    float64
}

// FindAndRankDrivers finds candidate drivers and ranks them using Multi-Factor Scoring Engine:
// Distance is the PRIMARY factor (Max 70 pts), supplemented by Rating (Max 15 pts) and Acceptance Rate (Max 15 pts).
func (s *Store) FindAndRankDrivers(pos models.Position, requestedVehicleType string, excludedIDs map[string]bool) []*models.Driver {
	s.mu.Lock()
	defer s.mu.Unlock()

	var candidates []DriverScoreItem
	for _, d := range s.drivers {
		// Strictly exclude drivers already rejected or busy
		if d.Status == models.DriverStatusIdle && !excludedIDs[d.ID] {
			// Vehicle Type Matching Filter (e.g. Xe Máy, Ô tô 4 chỗ, Ô tô 7 chỗ)
			if requestedVehicleType != "" {
				vReq := strings.ToLower(requestedVehicleType)
				vDrv := strings.ToLower(d.VehicleType)
				if !strings.Contains(vDrv, vReq) && !strings.Contains(vReq, vDrv) {
					// Soft match check
					if strings.Contains(vReq, "máy") && !strings.Contains(vDrv, "máy") {
						continue
					}
					if (strings.Contains(vReq, "4 chỗ") || strings.Contains(vReq, "taxi")) && !strings.Contains(vDrv, "4 chỗ") {
						continue
					}
					if (strings.Contains(vReq, "7 chỗ") || strings.Contains(vReq, "luxury")) && !strings.Contains(vDrv, "7 chỗ") {
						continue
					}
				}
			}

			dist := CalculateDistance(pos, d.Position)

			// Multi-factor Score Formula (Distance Dominant)
			// 1. Proximity score (Max 70 pts for < 0.5km, drops sharply with distance squared)
			distScore := 70.0 / (dist*dist*0.8 + 1.0)

			// 2. Rating score (Max 15 pts for 5.0)
			ratingScore := (d.Rating / 5.0) * 15.0

			// 3. Acceptance rate score (Max 15 pts for 100%)
			acceptanceScore := (d.AcceptanceRate / 100.0) * 15.0

			totalScore := math.Round((distScore+ratingScore+acceptanceScore)*10) / 10.0

			d.Score = totalScore
			copied := *d
			candidates = append(candidates, DriverScoreItem{Driver: &copied, Distance: dist, Score: totalScore})
		}
	}

	// Sort candidates by total Score DESCENDING
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Distance < candidates[j].Distance
		}
		return candidates[i].Score > candidates[j].Score
	})

	result := make([]*models.Driver, len(candidates))
	for i, c := range candidates {
		result[i] = c.Driver
	}
	return result
}

// FindAndRankDriversAdvanced incorporates advanced business rules:
// - Fatigue check (DrivingMinutes < 240)
// - Wallet balance check for CASH trips (WalletBalance >= 20,000)
// - VIP Loyalty Boost (+10 pts)
// - Destination Filter (My Route) alignment boost (+10 pts)
func (s *Store) FindAndRankDriversAdvanced(pos models.Position, requestedVehicleType string, paymentMethod string, customerTier string, excludedIDs map[string]bool) []*models.Driver {
	s.mu.Lock()
	defer s.mu.Unlock()

	var candidates []DriverScoreItem
	for _, d := range s.drivers {
		// 1. Hard Filter: IDLE and not Excluded
		if d.Status != models.DriverStatusIdle || excludedIDs[d.ID] {
			continue
		}

		// 2. Hard Filter: Driving Fatigue Limit (Max 240 mins continuous driving)
		if d.DrivingMinutes >= 240 {
			continue
		}

		// 3. Hard Filter: Driver Wallet Balance for CASH trips (Min 20,000 VND deposit)
		if strings.ToUpper(paymentMethod) == "CASH" && d.WalletBalance < 20000 {
			continue
		}

		// 4. Vehicle Type Matching Filter
		if requestedVehicleType != "" {
			vReq := strings.ToLower(requestedVehicleType)
			vDrv := strings.ToLower(d.VehicleType)
			if !strings.Contains(vDrv, vReq) && !strings.Contains(vReq, vDrv) {
				if strings.Contains(vReq, "máy") && !strings.Contains(vDrv, "máy") {
					continue
				}
				if (strings.Contains(vReq, "4 chỗ") || strings.Contains(vReq, "taxi")) && !strings.Contains(vDrv, "4 chỗ") {
					continue
				}
				if (strings.Contains(vReq, "7 chỗ") || strings.Contains(vReq, "luxury")) && !strings.Contains(vDrv, "7 chỗ") {
					continue
				}
			}
		}

		dist := CalculateDistance(pos, d.Position)

		// Multi-factor Score Calculation
		// a. Proximity score (Max 50 pts)
		distScore := 50.0 / (dist*dist*0.8 + 1.0)

		// b. Rating score (Max 15 pts)
		ratingScore := (d.Rating / 5.0) * 15.0

		// c. Acceptance rate score (Max 15 pts)
		acceptanceScore := (d.AcceptanceRate / 100.0) * 15.0

		// d. Customer VIP Boost (+10 pts)
		vipBoost := 0.0
		if strings.ToUpper(customerTier) == "VIP" || strings.ToUpper(customerTier) == "PLATINUM" {
			vipBoost = 10.0
		}

		// e. Destination Filter (My Route) Bonus (+10 pts if trip is towards driver's home)
		myRouteBonus := 0.0
		if d.HomeDestination != nil {
			distToHomeNow := CalculateDistance(d.Position, *d.HomeDestination)
			distToHomeFromPickup := CalculateDistance(pos, *d.HomeDestination)
			if distToHomeFromPickup < distToHomeNow {
				myRouteBonus = 10.0
			}
		}

		totalScore := math.Round((distScore+ratingScore+acceptanceScore+vipBoost+myRouteBonus)*10) / 10.0

		d.Score = totalScore
		copied := *d
		candidates = append(candidates, DriverScoreItem{Driver: &copied, Distance: dist, Score: totalScore})
	}

	// Sort candidates by total Score DESCENDING
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Distance < candidates[j].Distance
		}
		return candidates[i].Score > candidates[j].Score
	})

	result := make([]*models.Driver, len(candidates))
	for i, c := range candidates {
		result[i] = c.Driver
	}
	return result
}

// AtomicReserveDriver attempts to assign a driver to a booking atomically
func (s *Store) AtomicReserveDriver(driverID, bookingID, token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	driver, ok := s.drivers[driverID]
	if !ok || driver.Status != models.DriverStatusIdle {
		return false
	}

	driver.Status = models.DriverStatusAssigning
	driver.CurrentBookingID = bookingID

	if booking, found := s.bookings[bookingID]; found {
		booking.DriverID = driverID
		booking.Status = models.BookingStatusAssigning
		booking.AssignmentToken = token
		booking.UpdatedAt = time.Now()
	}

	return true
}

// AtomicAcceptBooking handles driver accepting a booking
func (s *Store) AtomicAcceptBooking(bookingID, driverID, token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	booking, ok := s.bookings[bookingID]
	if !ok || booking.Status != models.BookingStatusAssigning || booking.DriverID != driverID || booking.AssignmentToken != token {
		return false
	}

	driver, okD := s.drivers[driverID]
	if !okD {
		return false
	}

	booking.Status = models.BookingStatusAccepted
	booking.UpdatedAt = time.Now()

	driver.Status = models.DriverStatusBusy
	driver.CurrentBookingID = bookingID
	driver.TotalTrips++

	return true
}

// AtomicReleaseDriver releases driver back to IDLE
func (s *Store) AtomicReleaseDriver(driverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if driver, ok := s.drivers[driverID]; ok {
		driver.Status = models.DriverStatusIdle
		driver.CurrentBookingID = ""
	}
}

func (s *Store) AddExcludedDriverToBooking(bookingID, driverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if booking, ok := s.bookings[bookingID]; ok {
		for _, id := range booking.ExcludedDriverIDs {
			if id == driverID {
				return
			}
		}
		booking.ExcludedDriverIDs = append(booking.ExcludedDriverIDs, driverID)
	}
}

func (s *Store) GetStats() models.SystemStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := models.SystemStats{}
	stats.TotalDrivers = len(s.drivers)
	for _, d := range s.drivers {
		switch d.Status {
		case models.DriverStatusIdle:
			stats.IdleDrivers++
		case models.DriverStatusAssigning, models.DriverStatusBusy:
			stats.BusyDrivers++
		}
	}

	stats.TotalBookings = len(s.bookings)
	for _, b := range s.bookings {
		switch b.Status {
		case models.BookingStatusPending, models.BookingStatusAssigning:
			stats.PendingBookings++
		case models.BookingStatusAccepted, models.BookingStatusCompleted:
			stats.AcceptedBookings++
		case models.BookingStatusFailed, models.BookingStatusCancelled, models.BookingStatusTimeout, models.BookingStatusRejected:
			stats.FailedBookings++
		}
	}

	return stats
}
