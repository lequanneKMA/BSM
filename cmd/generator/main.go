package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"

	"bsm/pkg/scoring"
)

func main() {
	r := rand.New(rand.NewSource(42))

	centerLat := 21.0285
	centerLng := 105.8542

	drivers := make([]scoring.Driver, 1000)

	for i := 0; i < 1000; i++ {
		radiusKm := r.Float64() * 10.0
		angleRad := r.Float64() * 2 * math.Pi

		latOffset := (radiusKm / 111.0) * math.Cos(angleRad)
		lngOffset := (radiusKm / (111.0 * math.Cos(centerLat*math.Pi/180.0))) * math.Sin(angleRad)

		rating := 3.8 + (r.Float64() * 1.2)
		ar := 60.0 + (r.Float64() * 40.0)
		cor := 70.0 + (r.Float64() * 30.0)
		idleSec := r.Float64() * 1800.0
		wallet := 100000.0 + (r.Float64() * 900000.0)

		drivers[i] = scoring.Driver{
			ID:               fmt.Sprintf("DRV-HN-%04d", i+1),
			Rating:           math.Round(rating*100) / 100,
			AcceptanceRate:   math.Round(ar*10) / 10,
			CancellationRate: math.Round(cor*10) / 10,
			IdleTimeSeconds:  math.Round(idleSec),
			WalletBalance:    math.Round(wallet),
			Lat:              centerLat + latOffset,
			Lng:              centerLng + lngOffset,
		}
	}

	_ = os.MkdirAll("data", 0755)
	file, err := os.Create("data/hanoi_drivers.json")
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(drivers); err != nil {
		fmt.Printf("Error encoding JSON: %v\n", err)
		return
	}

	fmt.Println("✅ Generated 1,000 Hanoi drivers dataset at data/hanoi_drivers.json!")
}
