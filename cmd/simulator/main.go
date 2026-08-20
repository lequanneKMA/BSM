package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"bsm/pkg/scoring"
)

type LocationPreset struct {
	Name string
	Lat  float64
	Lng  float64
}

var hanoiPresets = []LocationPreset{
	{Name: "Hồ Hoàn Kiếm (Quận Hoàn Kiếm)", Lat: 21.0285, Lng: 105.8542},
	{Name: "Sân vận động Mỹ Đình (Quận Nam Từ Liêm)", Lat: 21.0205, Lng: 105.7640},
	{Name: "Keangnam Landmark 72 (Quận Cầu Giấy)", Lat: 21.0168, Lng: 105.7838},
	{Name: "Ga Hà Nội (Quận Đống Đa)", Lat: 21.0245, Lng: 105.8412},
	{Name: "Sân bay Nội Bài (Huyện Sóc Sơn)", Lat: 21.2212, Lng: 105.8072},
}

func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

func LocationServiceMock(allDrivers []scoring.Driver, booking scoring.BookingContext, topK int) []scoring.Candidate {
	type candidateWithDist struct {
		driver    scoring.Driver
		distMeter float64
	}

	filtered := make([]candidateWithDist, 0)
	exclMap := make(map[string]bool)
	for _, id := range booking.ExcludedDriverIDs {
		exclMap[id] = true
	}

	for _, d := range allDrivers {
		if exclMap[d.ID] {
			continue
		}
		dist := haversineDistance(booking.PickupLat, booking.PickupLng, d.Lat, d.Lng)
		filtered = append(filtered, candidateWithDist{driver: d, distMeter: dist})
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].distMeter < filtered[j].distMeter
	})

	if len(filtered) > topK {
		filtered = filtered[:topK]
	}

	speedMps := 6.9
	if booking.ServiceType == scoring.ServiceCar4Seat || booking.ServiceType == scoring.ServiceCar7Seat {
		speedMps = 5.0
	}

	candidates := make([]scoring.Candidate, len(filtered))
	for i, f := range filtered {
		etaSec := f.distMeter / speedMps
		barrierCount := 0
		if f.distMeter > 1000 {
			barrierCount = 1
		}
		if f.distMeter > 3000 {
			barrierCount = 2
		}

		candidates[i] = scoring.Candidate{
			Driver: f.driver,
			Route: scoring.RouteInfo{
				ETASeconds:         math.Round(etaSec),
				RoadDistanceMeters: math.Round(f.distMeter * 1.15),
				AngleDeviationDeg:  15.0,
				BarrierCount:       barrierCount,
			},
		}
	}

	return candidates
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println(strings.Repeat("=", 85))
	fmt.Println("   BSM ARCHITECTURE SIMULATOR - 2-STAGE DISPATCH PIPELINE (1000 POOL -> 20 CANDIDATES)  ")
	fmt.Println(strings.Repeat("=", 85))

	file, err := os.Open("data/hanoi_drivers.json")
	if err != nil {
		fmt.Printf("❌ Failed to open data/hanoi_drivers.json: %v. Please run: go run cmd/generator/main.go\n", err)
		return
	}
	defer file.Close()

	var allDrivers []scoring.Driver
	if err := json.NewDecoder(file).Decode(&allDrivers); err != nil {
		fmt.Printf("❌ Failed to parse data/hanoi_drivers.json: %v\n", err)
		return
	}

	fmt.Printf("📂 Successfully loaded %d drivers dataset from data/hanoi_drivers.json\n\n", len(allDrivers))

	fmt.Println("📍 SELECT PICKUP LOCATION IN HANOI:")
	for i, p := range hanoiPresets {
		fmt.Printf("   [%d] %-45s (%.4f, %.4f)\n", i+1, p.Name, p.Lat, p.Lng)
	}
	fmt.Print("👉 Enter selection (1-5, Default = 1): ")

	inputChoice, _ := reader.ReadString('\n')
	inputChoice = strings.TrimSpace(inputChoice)

	pickupLat := hanoiPresets[0].Lat
	pickupLng := hanoiPresets[0].Lng
	pickupName := hanoiPresets[0].Name

	if val, err := strconv.Atoi(inputChoice); err == nil && val >= 1 && val <= len(hanoiPresets) {
		pickupLat = hanoiPresets[val-1].Lat
		pickupLng = hanoiPresets[val-1].Lng
		pickupName = hanoiPresets[val-1].Name
	}

	booking := scoring.BookingContext{
		BookingID:          "BK-HN-PROD-001",
		CustomerID:         "CUST-HN-888",
		CreatedAt:          time.Now(),
		PickupLat:          pickupLat,
		PickupLng:          pickupLng,
		PickupAddress:      pickupName,
		TripDistanceMeters: 4500,
		EstimatedFare:      65000,
		PaymentMethod:      scoring.PaymentCash,
		ServiceType:        scoring.ServiceBike,
		Attempt:            0,
	}

	// STAGE 1: location-svc
	locStartTime := time.Now()
	top20Candidates := LocationServiceMock(allDrivers, booking, 20)
	locElapsed := time.Since(locStartTime)

	fmt.Printf("\n🛰️  [STAGE 1: location-svc Spatial Filtering]\n")
	fmt.Printf("   - Scan %d active drivers from H3 Spatial Index around %s\n", len(allDrivers), pickupName)
	fmt.Printf("   - Haversine Distance Filtering & select top 20 closest candidates (%v)\n", locElapsed)
	fmt.Printf("   - Output: Forward 20 candidate drivers payload to dispatch-svc\n")

	// STAGE 2: dispatch-svc Engine
	dispStartTime := time.Now()
	results, topCandidate := scoring.RankCandidates(top20Candidates, booking)
	dispElapsed := time.Since(dispStartTime)

	fmt.Printf("\n⚙️  [STAGE 2: dispatch-svc Scoring Engine]\n")
	fmt.Printf("   - Evaluate 20 candidates ➔ Execute Non-linear Reciprocal Decay & Profile Scoring (%v)\n", dispElapsed)

	fmt.Println("\n" + strings.Repeat("-", 90))
	fmt.Printf("%-4s | %-13s | %-6s | %-8s | %-8s | %-8s | %-11s | %-8s\n",
		"RANK", "DRIVER ID", "RATING", "AR/CoR", "RAW ETA", "DISTANCE", "TOTAL SCORE", "STATUS")
	fmt.Println(strings.Repeat("-", 90))

	candMap := make(map[string]scoring.Candidate)
	for _, c := range top20Candidates {
		candMap[c.Driver.ID] = c
	}

	for i, res := range results {
		cand := candMap[res.DriverID]
		status := "PASSED"
		if i == 0 && topCandidate != nil && topCandidate.DriverID == res.DriverID {
			status = "🏆 RANK 1"
		}

		fmt.Printf("#%-3d | %-13s | %-6.2f | %-2.0f%%/%-2.0f%% | %-5.0fs (%-2.1fm) | %-6.0fm | %-11.2f | %-8s\n",
			i+1,
			res.DriverID,
			cand.Driver.Rating,
			cand.Driver.AcceptanceRate,
			cand.Driver.CancellationRate,
			cand.Route.ETASeconds,
			cand.Route.ETASeconds/60.0,
			cand.Route.RoadDistanceMeters,
			res.TotalScore,
			status,
		)
	}
	fmt.Println(strings.Repeat("-", 90))

	if topCandidate != nil {
		fmt.Printf("\n✅ [DISPATCH COMPLETE]\n")
		fmt.Printf("   SELECTED WINNING DRIVER : %s (Total Score: %.2f)\n", topCandidate.DriverID, topCandidate.TotalScore)
		fmt.Printf("   TOTAL PIPELINE LATENCY  : %v (location-svc: %v + dispatch-svc: %v)\n",
			locElapsed+dispElapsed, locElapsed, dispElapsed)
	}
	fmt.Println(strings.Repeat("=", 85))
}
