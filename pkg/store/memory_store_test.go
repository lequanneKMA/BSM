package store

import (
	"testing"

	"bsm/pkg/models"
)

func TestCalculateDistance(t *testing.T) {
	tests := []struct {
		name     string
		p1       models.Position
		p2       models.Position
		minDist  float64
		maxDist  float64
	}{
		{
			name:    "Same Position (Zero Distance)",
			p1:      models.Position{Lat: 21.0285, Lng: 105.8542},
			p2:      models.Position{Lat: 21.0285, Lng: 105.8542},
			minDist: 0.0,
			maxDist: 0.001,
		},
		{
			name:    "Hanoi Center to Long Bien (~2 km)",
			p1:      models.Position{Lat: 21.0285, Lng: 105.8542},
			p2:      models.Position{Lat: 21.0400, Lng: 105.8650},
			minDist: 1.0,
			maxDist: 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateDistance(tt.p1, tt.p2)
			if got < tt.minDist || got > tt.maxDist {
				t.Errorf("CalculateDistance() = %v, want range [%v, %v]", got, tt.minDist, tt.maxDist)
			}
		})
	}
}

func TestFindAndRankDrivers_DistanceDominance(t *testing.T) {
	s := NewStore()

	// Driver 1: Near (0.2 km), Rating 4.5
	dNear := models.Driver{
		ID:             "drv_near",
		Name:           "Tài xế Gần",
		Position:       models.Position{Lat: 21.0285, Lng: 105.8542},
		Status:         models.DriverStatusIdle,
		Rating:         4.5,
		AcceptanceRate: 90.0,
		VehicleType:    "Ô tô 4 chỗ",
	}

	// Driver 2: Far (8 km), Rating 5.0
	dFar := models.Driver{
		ID:             "drv_far",
		Name:           "Tài xế Xa",
		Position:       models.Position{Lat: 21.1000, Lng: 105.9000},
		Status:         models.DriverStatusIdle,
		Rating:         5.0,
		AcceptanceRate: 100.0,
		VehicleType:    "Ô tô 4 chỗ",
	}

	s.AddDriver(dNear)
	s.AddDriver(dFar)

	customerPos := models.Position{Lat: 21.0285, Lng: 105.8542}
	excluded := make(map[string]bool)

	ranked := s.FindAndRankDrivers(customerPos, "Ô tô 4 chỗ", excluded)

	if len(ranked) == 0 {
		t.Fatalf("FindAndRankDrivers returned 0 drivers")
	}

	if ranked[0].ID != "drv_near" {
		t.Errorf("Expected top ranked driver to be drv_near, got %s (Score: %.1f)", ranked[0].ID, ranked[0].Score)
	}
}

func TestFindAndRankDrivers_VehicleTypeFiltering(t *testing.T) {
	s := NewStore()

	dBike := models.Driver{
		ID:          "drv_bike",
		Name:        "Tài xế Xe Máy",
		Position:    models.Position{Lat: 21.0285, Lng: 105.8542},
		Status:      models.DriverStatusIdle,
		VehicleType: "Xe Máy 🛵",
	}

	dCar := models.Driver{
		ID:          "drv_car",
		Name:        "Tài xế Taxi",
		Position:    models.Position{Lat: 21.0285, Lng: 105.8542},
		Status:      models.DriverStatusIdle,
		VehicleType: "Ô tô 4 chỗ 🚗",
	}

	s.AddDriver(dBike)
	s.AddDriver(dCar)

	customerPos := models.Position{Lat: 21.0285, Lng: 105.8542}
	excluded := make(map[string]bool)

	// Filter for Xe Máy
	rankedBike := s.FindAndRankDrivers(customerPos, "Xe Máy", excluded)
	if len(rankedBike) != 1 || rankedBike[0].ID != "drv_bike" {
		t.Errorf("Expected only drv_bike for Xe Máy filter, got %v", len(rankedBike))
	}

	// Filter for Ô tô 4 chỗ
	rankedCar := s.FindAndRankDrivers(customerPos, "Ô tô 4 chỗ", excluded)
	if len(rankedCar) != 1 || rankedCar[0].ID != "drv_car" {
		t.Errorf("Expected only drv_car for Ô tô 4 chỗ filter, got %v", len(rankedCar))
	}
}

func TestDriverExclusion(t *testing.T) {
	s := NewStore()

	d1 := models.Driver{
		ID:          "drv_1",
		Name:        "Tài xế 1",
		Position:    models.Position{Lat: 21.0285, Lng: 105.8542},
		Status:      models.DriverStatusIdle,
		VehicleType: "Xe Máy",
	}

	s.AddDriver(d1)

	customerPos := models.Position{Lat: 21.0285, Lng: 105.8542}
	excluded := map[string]bool{"drv_1": true}

	ranked := s.FindAndRankDrivers(customerPos, "Xe Máy", excluded)
	if len(ranked) != 0 {
		t.Errorf("Expected excluded driver drv_1 to be filtered out, got %d drivers", len(ranked))
	}
}

func BenchmarkCalculateDistance(b *testing.B) {
	p1 := models.Position{Lat: 21.0285, Lng: 105.8542}
	p2 := models.Position{Lat: 21.0400, Lng: 105.8650}

	for i := 0; i < b.N; i++ {
		_ = CalculateDistance(p1, p2)
	}
}
