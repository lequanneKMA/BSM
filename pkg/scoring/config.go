package scoring

import (
	"fmt"
	"math"
	"sync"
)

// ServiceConfig holds all dynamic scoring parameters per service type
type ServiceConfig struct {
	Alpha                float64 `json:"alpha"`                 // Reciprocal ETA decay rate (0.008 Bike, 0.003 Car)
	W1                   float64 `json:"w1"`                    // Rating weight (w1 + w2 + w3 == 1.0)
	W2                   float64 `json:"w2"`                    // Acceptance rate (AR) weight
	W3                   float64 `json:"w3"`                    // Completion rate (CoR) weight
	BarrierPenaltyWeight float64 `json:"barrier_penalty_weight"`// Barrier factor w_barrier (0.0 Bike, 0.20 Car)
	BaseMinScore         float64 `json:"base_min_score"`        // Initial MinScore at attempt 0 (60.0)
	MinScoreFloor        float64 `json:"min_score_floor"`       // Minimum floor score (30.0)
	ScoreDecayRate       float64 `json:"score_decay_rate"`      // Decay multiplier per attempt (0.8)
	AgingLambda          float64 `json:"aging_lambda"`          // Aging exponential rate (0.005)
	IdleBeta             float64 `json:"idle_beta"`             // Idle FIFO exponential rate (0.001)
	CVip                 float64 `json:"c_vip"`                 // Max VIP boost (10.0)
	FareAvgZoneHour      float64 `json:"fare_avg_zone_hour"`    // Benchmark avg fare for FareRatio (e.g. 50000.0)
}

// ConfigManager handles thread-safe, dynamic hot-reloading of scoring parameters
type ConfigManager struct {
	mu      sync.RWMutex
	configs map[ServiceType]ServiceConfig
}

var globalConfigManager *ConfigManager
var once sync.Once

// GetConfigManager returns the thread-safe singleton ConfigManager
func GetConfigManager() *ConfigManager {
	once.Do(func() {
		globalConfigManager = &ConfigManager{
			configs: make(map[ServiceType]ServiceConfig),
		}
		globalConfigManager.initDefaults()
	})
	return globalConfigManager
}

// initDefaults sets up verified production default configurations
func (cm *ConfigManager) initDefaults() {
	cm.configs[ServiceBike] = ServiceConfig{
		Alpha:                0.0025,
		W1:                   0.40,
		W2:                   0.30,
		W3:                   0.30,
		BarrierPenaltyWeight: 0.0,
		BaseMinScore:         45.0,
		MinScoreFloor:        25.0,
		ScoreDecayRate:       0.80,
		AgingLambda:          0.005,
		IdleBeta:             0.001,
		CVip:                 10.0,
		FareAvgZoneHour:      30000.0,
	}

	carConfig := ServiceConfig{
		Alpha:                0.003,
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

	cm.configs[ServiceCar4Seat] = carConfig
	cm.configs[ServiceCar7Seat] = carConfig
}

// GetConfig returns a copy of the active configuration for a service type safely
func (cm *ConfigManager) GetConfig(serviceType ServiceType) ServiceConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	cfg, exists := cm.configs[serviceType]
	if !exists {
		return cm.configs[ServiceBike]
	}
	return cfg
}

// UpdateConfig updates scoring parameters dynamically at runtime without restarting
func (cm *ConfigManager) UpdateConfig(serviceType ServiceType, newCfg ServiceConfig) error {
	// 1. Validate invariant: Weights sum must equal 1.0 (with floating-point tolerance)
	weightSum := newCfg.W1 + newCfg.W2 + newCfg.W3
	if math.Abs(weightSum-1.0) > 0.001 {
		return fmt.Errorf("invalid config: weights sum (w1+w2+w3 = %.4f) must equal 1.0", weightSum)
	}

	// 2. Validate floor and base min score
	if newCfg.MinScoreFloor < 0 || newCfg.BaseMinScore < newCfg.MinScoreFloor {
		return fmt.Errorf("invalid config: BaseMinScore (%.2f) must be >= MinScoreFloor (%.2f)", newCfg.BaseMinScore, newCfg.MinScoreFloor)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.configs[serviceType] = newCfg
	return nil
}

// CalculateEffectiveMinScore calculates closed-form MinScore decay: max(30.0, 60.0 * 0.8^attempt)
func (cfg ServiceConfig) CalculateEffectiveMinScore(attempt int) float64 {
	decayed := cfg.BaseMinScore * math.Pow(cfg.ScoreDecayRate, float64(attempt))
	if decayed < cfg.MinScoreFloor {
		return cfg.MinScoreFloor
	}
	return decayed
}
