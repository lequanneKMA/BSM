package scoring

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestDynamicConfigHotReload(t *testing.T) {
	cm := GetConfigManager()

	// 1. Valid config update
	newCfg := ServiceConfig{
		Alpha:                0.005,
		W1:                   0.50,
		W2:                   0.25,
		W3:                   0.25,
		BarrierPenaltyWeight: 0.20,
		BaseMinScore:         60.0,
		MinScoreFloor:        30.0,
		ScoreDecayRate:       0.80,
		AgingLambda:          0.005,
		IdleBeta:             0.001,
		CVip:                 10.0,
		FareAvgZoneHour:      50000.0,
	}

	err := cm.UpdateConfig(ServiceBike, newCfg)
	if err != nil {
		t.Fatalf("expected valid config update to succeed, got error: %v", err)
	}

	activeCfg := cm.GetConfig(ServiceBike)
	if activeCfg.Alpha != 0.005 || activeCfg.W1 != 0.50 {
		t.Errorf("config update failed to persist in memory")
	}

	// Test fallback to default for unknown service type
	unknownCfg := cm.GetConfig(ServiceType("UNKNOWN"))
	if unknownCfg.W1 == 0 {
		t.Errorf("expected default fallback config for unknown service type")
	}

	// 2. Invalid weight sum check (w1+w2+w3 != 1.0)
	invalidCfg := newCfg
	invalidCfg.W1 = 0.80 // Sum = 1.30
	err = cm.UpdateConfig(ServiceBike, invalidCfg)
	if err == nil {
		t.Errorf("expected error on invalid weight sum, got nil")
	}

	// 3. Invalid min score floor check
	invalidFloorCfg := newCfg
	invalidFloorCfg.BaseMinScore = 20.0
	invalidFloorCfg.MinScoreFloor = 30.0
	err = cm.UpdateConfig(ServiceBike, invalidFloorCfg)
	if err == nil {
		t.Errorf("expected error when BaseMinScore < MinScoreFloor, got nil")
	}

	// 4. Min score decay floor hit check
	decayedFloor := activeCfg.CalculateEffectiveMinScore(10) // 10 attempts
	if decayedFloor != activeCfg.MinScoreFloor {
		t.Errorf("expected MinScore floor %.1f, got %.1f", activeCfg.MinScoreFloor, decayedFloor)
	}

	// 5. Concurrent access test (5 users modifying & reading simultaneously)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = cm.GetConfig(ServiceBike)
				cfg := newCfg
				if id%2 == 0 {
					cfg.Alpha = 0.004
				} else {
					cfg.Alpha = 0.006
				}
				_ = cm.UpdateConfig(ServiceBike, cfg)
			}
		}(i)
	}
	wg.Wait()
}

func TestCompleteScoringFormulas(t *testing.T) {
	now := time.Now()
	ctx := BookingContext{
		BookingID:           "B1001",
		CustomerID:          "C500",
		CreatedAt:           now.Add(-60 * time.Second), // 60s wait
		ServiceType:         ServiceCar4Seat,
		EstimatedFare:       150000.0, // FareRatio = 150k / 50k = 3.0
		PaymentMethod:       PaymentCard,
		IsVIP:               true,
		Attempt:             0,
		InitialRadiusMeters: 2000,
	}

	candidate := Candidate{
		Driver: Driver{
			ID:               "D1",
			VehicleType:      ServiceCar4Seat,
			IsIdle:           true,
			Rating:           5.0,
			AcceptanceRate:   100.0,
			CancellationRate: 10.0, // 90% CoR
			IdleTimeSeconds:  1200.0, // 20 mins idle
			WalletBalance:    50000.0,
		},
		Route: RouteInfo{
			ETASeconds:         180.0, // 3 mins ETA
			RoadDistanceMeters: 1500.0,
			BarrierCount:       10, // Physical barriers capped at 5
		},
	}

	res := CalculateDriverScore(candidate, ctx)

	if res.TotalScore <= 0 || res.TotalScore > 130.0 {
		t.Errorf("TotalScore (%.2f) out of valid bounds [0, 130]", res.TotalScore)
	}
	if res.BarrierFactor != 0.4 {
		t.Errorf("BarrierFactor (%.2f) should be clamped at 0.4", res.BarrierFactor)
	}
	if res.BoostScore > 30.0 {
		t.Errorf("BoostScore (%.2f) exceeded S_boost cap of 30.0", res.BoostScore)
	}

	// Test RankCandidates empty
	emptyResults, emptyTop := RankCandidates(nil, ctx)
	if emptyResults != nil || emptyTop != nil {
		t.Errorf("expected nil for empty candidates")
	}
}

func TestFindAndRankDriversAdvanced(t *testing.T) {
	now := time.Now()
	ctx := BookingContext{
		BookingID:           "B2002",
		ServiceType:         ServiceBike,
		PaymentMethod:       PaymentCash,
		Attempt:             0,
		CurrentRadiusMeters: 2000,
		InitialRadiusMeters: 2000,
		RadiusExpansionRate: 1.5,
		ExcludedDriverIDs:   []string{"D_EXCLUDED"},
	}

	allCandidates := []Candidate{
		{
			Driver: Driver{ID: "D_EXCLUDED", IsIdle: true, VehicleType: ServiceBike, WalletBalance: 50000},
			Route:  RouteInfo{ETASeconds: 100, RoadDistanceMeters: 1000},
		},
		{
			Driver: Driver{ID: "D_BUSY", IsIdle: false, VehicleType: ServiceBike, WalletBalance: 50000},
			Route:  RouteInfo{ETASeconds: 100, RoadDistanceMeters: 1000},
		},
		{
			Driver: Driver{ID: "D_MISMATCH_VEHICLE", IsIdle: true, VehicleType: ServiceCar4Seat, WalletBalance: 50000},
			Route:  RouteInfo{ETASeconds: 100, RoadDistanceMeters: 1000},
		},
		{
			Driver: Driver{ID: "D_LOW_WALLET", IsIdle: true, VehicleType: ServiceBike, WalletBalance: 5000}, // Cash check fails < 20k
			Route:  RouteInfo{ETASeconds: 100, RoadDistanceMeters: 1000},
		},
		{
			Driver: Driver{ID: "D_LOCKED", IsIdle: true, VehicleType: ServiceBike, WalletBalance: 50000, CooldownUntil: now.Add(10 * time.Minute)},
			Route:  RouteInfo{ETASeconds: 100, RoadDistanceMeters: 1000},
		},
		{
			Driver: Driver{ID: "D_VALID_1", IsIdle: true, VehicleType: ServiceBike, Rating: 4.9, AcceptanceRate: 95, CompletionRate: 95, WalletBalance: 50000},
			Route:  RouteInfo{ETASeconds: 120, RoadDistanceMeters: 1200},
		},
		{
			Driver: Driver{ID: "D_VALID_2", IsIdle: true, VehicleType: ServiceBike, Rating: 4.5, AcceptanceRate: 80, CompletionRate: 85, WalletBalance: 30000},
			Route:  RouteInfo{ETASeconds: 240, RoadDistanceMeters: 1500},
		},
	}

	decision := FindAndRankDriversAdvanced(allCandidates, ctx, now)

	if len(decision.AllResults) != 2 {
		t.Fatalf("expected 2 valid candidates passing hard filters, got %d", len(decision.AllResults))
	}
	if decision.TopCandidate == nil || decision.TopCandidate.DriverID != "D_VALID_1" {
		t.Errorf("expected top candidate D_VALID_1, got %v", decision.TopCandidate)
	}
	if decision.ShouldExpandRadius {
		t.Errorf("expected ShouldExpandRadius=false when top candidate is found")
	}
	if decision.SuggestedNextRadius != 3000.0 {
		t.Errorf("expected SuggestedNextRadius=3000.0, got %.1f", decision.SuggestedNextRadius)
	}

	// Test scenario where no candidate passes MinScore at attempt 0 (MinScore = 60.0)
	poorCandidates := []Candidate{
		{
			Driver: Driver{ID: "D_POOR", IsIdle: true, VehicleType: ServiceBike, Rating: 1.0, AcceptanceRate: 10, CompletionRate: 10, WalletBalance: 50000},
			Route:  RouteInfo{ETASeconds: 1200}, // 20 mins ETA -> very low score
		},
	}

	failedDecision := FindAndRankDriversAdvanced(poorCandidates, ctx, now)
	if failedDecision.TopCandidate != nil {
		t.Errorf("expected nil top candidate for poor score")
	}
	if !failedDecision.ShouldExpandRadius {
		t.Errorf("expected ShouldExpandRadius=true when no candidate satisfies MinScore")
	}
}

func TestStrategyRouter(t *testing.T) {
	// Empty matrix check
	resEmpty := StrategyRouter(nil, 10*time.Millisecond)
	if resEmpty.SolverUsed != SolverGreedy || len(resEmpty.Assignments) != 0 {
		t.Errorf("expected empty greedy result")
	}

	// 1. Small V <= 30 -> Hungarian
	matSmall := [][]float64{
		{80.0, 90.0},
		{70.0, 85.0},
	}
	resHungarian := StrategyRouter(matSmall, 10*time.Millisecond)
	if resHungarian.SolverUsed != SolverHungarian {
		t.Errorf("expected Hungarian solver for V=2, got %s", resHungarian.SolverUsed)
	}
	if len(resHungarian.Assignments) != 2 {
		t.Errorf("expected 2 assignments, got %d", len(resHungarian.Assignments))
	}

	// 2. Medium 30 < V <= 200 -> Auction (epsilon = 1.0)
	size := 35
	matMedium := make([][]float64, size)
	for i := 0; i < size; i++ {
		matMedium[i] = make([]float64, size)
		for j := 0; j < size; j++ {
			matMedium[i][j] = float64((i*j)%100) + 1.0
		}
	}
	resAuction := StrategyRouter(matMedium, 10*time.Millisecond)
	if resAuction.SolverUsed != SolverAuction {
		t.Errorf("expected Auction solver for V=35, got %s", resAuction.SolverUsed)
	}

	// 3. Timeout Budget Guard < 1ms -> Fallback Greedy
	resTimeout := StrategyRouter(matSmall, 500*time.Microsecond)
	if resTimeout.SolverUsed != SolverGreedy {
		t.Errorf("expected Greedy fallback for sub-1ms budget, got %s", resTimeout.SolverUsed)
	}
}

func BenchmarkRank1000Drivers(b *testing.B) {
	now := time.Now()
	ctx := BookingContext{
		BookingID:           "BENCH_1000",
		ServiceType:         ServiceCar4Seat,
		PaymentMethod:       PaymentCash,
		Attempt:             0,
		InitialRadiusMeters: 5000,
	}

	candidates := make([]Candidate, 1000)
	for i := 0; i < 1000; i++ {
		candidates[i] = Candidate{
			Driver: Driver{
				ID:              fmt.Sprintf("D_%d", i),
				VehicleType:     ServiceCar4Seat,
				IsIdle:          true,
				Rating:          3.5 + rand.Float64()*1.5,
				AcceptanceRate:  70 + rand.Float64()*30,
				CompletionRate:  80 + rand.Float64()*20,
				IdleTimeSeconds: rand.Float64() * 3600,
				WalletBalance:   50000,
			},
			Route: RouteInfo{
				ETASeconds:         60 + rand.Float64()*600,
				RoadDistanceMeters: 500 + rand.Float64()*4000,
				BarrierCount:       rand.Intn(3),
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FindAndRankDriversAdvanced(candidates, ctx, now)
	}
}

func BenchmarkParallelRank1000Drivers(b *testing.B) {
	now := time.Now()
	ctx := BookingContext{
		BookingID:           "BENCH_PARALLEL_1000",
		ServiceType:         ServiceCar4Seat,
		PaymentMethod:       PaymentCash,
		Attempt:             0,
		InitialRadiusMeters: 5000,
	}

	candidates := make([]Candidate, 1000)
	for i := 0; i < 1000; i++ {
		candidates[i] = Candidate{
			Driver: Driver{
				ID:              fmt.Sprintf("D_%d", i),
				VehicleType:     ServiceCar4Seat,
				IsIdle:          true,
				Rating:          3.5 + rand.Float64()*1.5,
				AcceptanceRate:  70 + rand.Float64()*30,
				CompletionRate:  80 + rand.Float64()*20,
				IdleTimeSeconds: rand.Float64() * 3600,
				WalletBalance:   50000,
			},
			Route: RouteInfo{
				ETASeconds:         60 + rand.Float64()*600,
				RoadDistanceMeters: 500 + rand.Float64()*4000,
				BarrierCount:       rand.Intn(3),
			},
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = FindAndRankDriversAdvanced(candidates, ctx, now)
		}
	})
}

func BenchmarkScore20DriversSingleOrder(b *testing.B) {
	now := time.Now()
	ctx := BookingContext{
		BookingID:           "BENCH_SINGLE_ORDER",
		ServiceType:         ServiceBike,
		PaymentMethod:       PaymentCash,
		Attempt:             0,
		InitialRadiusMeters: 1500,
	}

	// Real-world single order candidate pool: 20 drivers
	candidates := make([]Candidate, 20)
	for i := 0; i < 20; i++ {
		candidates[i] = Candidate{
			Driver: Driver{
				ID:              fmt.Sprintf("D_%d", i),
				VehicleType:     ServiceBike,
				IsIdle:          true,
				Rating:          4.5 + rand.Float64()*0.5,
				AcceptanceRate:  85 + rand.Float64()*15,
				CompletionRate:  90 + rand.Float64()*10,
				IdleTimeSeconds: rand.Float64() * 1800,
				WalletBalance:   150000,
			},
			Route: RouteInfo{
				ETASeconds:         60 + rand.Float64()*240,
				RoadDistanceMeters: 300 + rand.Float64()*1500,
				BarrierCount:       rand.Intn(2),
			},
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = FindAndRankDriversAdvanced(candidates, ctx, now)
		}
	})
}


