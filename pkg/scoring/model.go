package scoring

import "time"

// ServiceType defines the vehicle service category
type ServiceType string

const (
	ServiceBike     ServiceType = "BIKE"
	ServiceCar4Seat ServiceType = "CAR_4SEAT"
	ServiceCar7Seat ServiceType = "CAR_7SEAT"
)

// PaymentMethod defines how the customer pays for the trip
type PaymentMethod string

const (
	PaymentCash   PaymentMethod = "CASH"
	PaymentWallet PaymentMethod = "WALLET"
	PaymentCard   PaymentMethod = "CARD"
)

// Driver represents driver profile and real-time state metrics
type Driver struct {
	ID               string      `json:"driver_id"`
	VehicleType      ServiceType `json:"vehicle_type"`      // Matching vehicle service type
	IsIdle           bool        `json:"is_idle"`           // Must be true (Status == IDLE)
	Rating           float64     `json:"rating"`            // 1.0 - 5.0 (R_star)
	AcceptanceRate   float64     `json:"acceptance_rate"`  // 0.0 - 100.0 (AR)
	CancellationRate float64     `json:"cancellation_rate"`// 0.0 - 100.0 (CoR)
	CompletionRate   float64     `json:"completion_rate"`  // 0.0 - 100.0 (CoR)
	IdleTimeSeconds  float64     `json:"idle_time_seconds"`// Idle duration in seconds (t_idle)
	WalletBalance    float64     `json:"wallet_balance"`   // Driver wallet balance in VND (L_cash)
	CooldownUntil    time.Time   `json:"cooldown_until"`   // Lock cooldown expiration timestamp
	Lat              float64     `json:"lat"`
	Lng              float64     `json:"lng"`
}

// RouteInfo represents spatial & routing metadata from OSRM
type RouteInfo struct {
	ETASeconds         float64 `json:"eta_seconds"`          // Pickup ETA in seconds (t_ETA)
	RoadDistanceMeters float64 `json:"road_distance_meters"` // Pickup road distance in meters
	AngleDeviationDeg  float64 `json:"angle_deviation_deg"`  // Heading angle deviation in degrees
	BarrierCount       int     `json:"barrier_count"`        // Physical barrier count (B_barrier 0..5)
}

// BookingContext represents the comprehensive customer order context
type BookingContext struct {
	BookingID  string    `json:"booking_id"`
	CustomerID string    `json:"customer_id"`
	CreatedAt  time.Time `json:"created_at"`

	// Locations
	PickupLat      float64 `json:"pickup_lat"`
	PickupLng      float64 `json:"pickup_lng"`
	PickupAddress  string  `json:"pickup_address"`
	DropoffLat     float64 `json:"dropoff_lat"`
	DropoffLng     float64 `json:"dropoff_lng"`
	DropoffAddress string  `json:"dropoff_address"`

	// Trip Metadata
	TripDistanceMeters       float64 `json:"trip_distance_meters"`
	EstimatedTripDurationSec float64 `json:"estimated_trip_duration_sec"`

	// Pricing & Financials
	EstimatedFare   float64       `json:"estimated_fare"`   // Total fare in VND (V_fare)
	PaymentMethod   PaymentMethod `json:"payment_method"`   // CASH, WALLET, CARD
	SurgeMultiplier float64       `json:"surge_multiplier"` // Price multiplier (e.g. 1.2x, 1.5x)

	// Customer Profile & Requirements
	CustomerRating float64     `json:"customer_rating"`
	ServiceType    ServiceType `json:"service_type"`
	IsVIP          bool        `json:"is_vip"`

	// Radius expansion parameters (used to compute SuggestedRadius)
	CurrentRadiusMeters float64 `json:"current_radius_meters"`
	InitialRadiusMeters float64 `json:"initial_radius_meters"`
	RadiusExpansionRate float64 `json:"radius_expansion_rate"`
	MaxRadiusMeters     float64 `json:"max_radius_meters"`

	// Dispatch Lifecycle & Retry Tracking
	Attempt           int      `json:"attempt"`             // Search attempt count (0, 1, 2...)
	ExcludedDriverIDs []string `json:"excluded_driver_ids"` // Drivers who rejected/timed out on previous attempts
}

// Candidate combines a Driver with their pre-calculated RouteInfo
type Candidate struct {
	Driver Driver    `json:"driver"`
	Route  RouteInfo `json:"route"`
}

// ScoringResult stores the final evaluated score and breakdown
type ScoringResult struct {
	DriverID       string  `json:"driver_id"`
	TotalScore     float64 `json:"total_score"`
	CoreScore      float64 `json:"core_score"`
	ETAMultiplier  float64 `json:"eta_multiplier"`
	ProfileScore   float64 `json:"profile_score"`
	BarrierFactor  float64 `json:"barrier_factor"`
	BoostScore     float64 `json:"boost_score"`
	AgingBoost     float64 `json:"aging_boost"`
	IdleFifoBoost  float64 `json:"idle_fifo_boost"`
	RevenueBoost   float64 `json:"revenue_boost"`
	VIPBoost       float64 `json:"vip_boost"`
	MinScorePassed bool    `json:"min_score_passed"`
	CandidateIndex int     // Index lookup
}

// DispatchDecision is the explicit decision contract returned by the Scoring Engine to Orchestrator/Location-Svc
type DispatchDecision struct {
	TopCandidate        *ScoringResult  `json:"top_candidate"`
	AllResults          []ScoringResult `json:"all_results"`
	ShouldExpandRadius  bool            `json:"should_expand_radius"`  // True if candidate pool yielded no winner, signaling location-svc to expand
	SuggestedNextRadius float64         `json:"suggested_next_radius"` // Radius in meters suggested for location-svc next attempt query
	EffectiveMinScore   float64         `json:"effective_min_score"`   // The active MinScore threshold used for this attempt
}
