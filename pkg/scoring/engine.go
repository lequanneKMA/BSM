package scoring

import (
	"math"
	"slices"
	"time"
)

// CalculateDriverScore computes the exact non-linear score for a candidate driver according to algo.md
func CalculateDriverScore(candidate Candidate, ctx BookingContext) ScoringResult {
	cm := GetConfigManager()
	cfg := cm.GetConfig(ctx.ServiceType)

	d := candidate.Driver
	r := candidate.Route

	// 1. Physical Barrier Factor G_barrier = max(0.4, 1.0 - w_barrier * B_barrier)
	barrierCount := float64(r.BarrierCount)
	if barrierCount > 5.0 {
		barrierCount = 5.0
	}
	barrierFactor := 1.0 - (cfg.BarrierPenaltyWeight * barrierCount)
	if barrierFactor < 0.4 {
		barrierFactor = 0.4
	}

	// 2. Non-linear reciprocal ETA decay multiplier: 100.0 / (1.0 + alpha * t_ETA)
	etaMultiplier := 100.0 / (1.0 + cfg.Alpha*r.ETASeconds)

	// Handle CompletionRate (CoR) and CancellationRate (CR) penalty
	corValue := d.CompletionRate
	if corValue == 0 && d.CancellationRate > 0 {
		corValue = 100.0 - d.CancellationRate
	} else if corValue == 0 && d.CancellationRate == 0 {
		corValue = 100.0 // Default 100% CoR
	}
	corRatio := corValue / 100.0

	// Explicit Cancellation Risk Penalty: Deduct up to 0.15 from profile ratio if CR > 10%
	cancelPenalty := 0.0
	if d.CancellationRate > 10.0 {
		cancelPenalty = (d.CancellationRate - 10.0) / 100.0 * 0.5 // Phạt tăng dần khi CR > 10%
	}

	ratingRatio := d.Rating / 5.0
	arRatio := d.AcceptanceRate / 100.0

	profileScore := (cfg.W1 * (ratingRatio * ratingRatio)) +
		(cfg.W2 * arRatio) +
		(cfg.W3 * corRatio) - cancelPenalty
	if profileScore < 0.0 {
		profileScore = 0.0
	}

	// Core Score in [0, 100]
	coreScore := etaMultiplier * profileScore * barrierFactor
	if coreScore > 100.0 {
		coreScore = 100.0
	}

	// 4. Boost Components calculation (capped at S_boost <= 30.0)
	// 4a. Aging Boost: S_aging = min(10.0, 10.0 * (1 - e^(-lambda * t_wait)))
	tWaitSeconds := time.Since(ctx.CreatedAt).Seconds()
	if tWaitSeconds < 0 {
		tWaitSeconds = float64(ctx.Attempt) * 30.0 // Fallback estimate per attempt
	}
	agingBoost := 10.0 * (1.0 - math.Exp(-cfg.AgingLambda*tWaitSeconds))
	if agingBoost > 10.0 {
		agingBoost = 10.0
	}

	// 4b. Idle FIFO Boost: S_idle_fifo = min(7.0, 7.0 * (1 - e^(-beta * t_idle)))
	idleFifoBoost := 7.0 * (1.0 - math.Exp(-cfg.IdleBeta*d.IdleTimeSeconds))
	if idleFifoBoost > 7.0 {
		idleFifoBoost = 7.0
	}

	// 4c. Revenue Boost: P_revenue = min(3.0, FareRatio * CoR / 100)
	fareAvg := cfg.FareAvgZoneHour
	if fareAvg <= 0 {
		fareAvg = 50000.0
	}
	fareRatio := ctx.EstimatedFare / fareAvg
	if fareRatio > 3.0 {
		fareRatio = 3.0
	}
	revenueBoost := fareRatio * corRatio
	if revenueBoost > 3.0 {
		revenueBoost = 3.0
	}

	// 4d. VIP Boost: S_VIP(attempt) = C_vip * 0.8^attempt
	vipBoost := 0.0
	if ctx.IsVIP {
		vipBoost = cfg.CVip * math.Pow(0.8, float64(ctx.Attempt))
		if vipBoost > 10.0 {
			vipBoost = 10.0
		}
	}

	// Total S_boost capped at 30.0
	boostScore := agingBoost + idleFifoBoost + revenueBoost + vipBoost
	if boostScore > 30.0 {
		boostScore = 30.0
	}

	// 5. Total Score capped at 130.0
	totalScore := coreScore + boostScore
	if totalScore > 130.0 {
		totalScore = 130.0
	}
	totalScore = math.Round(totalScore*100) / 100

	// 6. Effective MinScore threshold check (decayed from 60.0 to 30.0 floor)
	effectiveMinScore := cfg.CalculateEffectiveMinScore(ctx.Attempt)
	passed := totalScore >= effectiveMinScore

	return ScoringResult{
		DriverID:       d.ID,
		TotalScore:     totalScore,
		CoreScore:      coreScore,
		ETAMultiplier:  etaMultiplier,
		ProfileScore:   profileScore,
		BarrierFactor:  barrierFactor,
		BoostScore:     boostScore,
		AgingBoost:     agingBoost,
		IdleFifoBoost:  idleFifoBoost,
		RevenueBoost:   revenueBoost,
		VIPBoost:       vipBoost,
		MinScorePassed: passed,
	}
}

// RankCandidates scores candidates and ranks them with strict tie-breaking rules
func RankCandidates(candidates []Candidate, ctx BookingContext) ([]ScoringResult, *ScoringResult) {
	if len(candidates) == 0 {
		return nil, nil
	}

	results := make([]ScoringResult, len(candidates))
	for i, c := range candidates {
		results[i] = CalculateDriverScore(c, ctx)
		results[i].CandidateIndex = i
	}

	// Sort descending by TotalScore with strict tie-breaking (ETA -> Idle FIFO -> Rating)
	slices.SortStableFunc(results, func(a, b ScoringResult) int {
		diff := a.TotalScore - b.TotalScore
		if math.Abs(diff) > 0.01 {
			if diff > 0 {
				return -1
			}
			return 1
		}

		// Tie-breaker 1: Shorter ETA
		candI := candidates[a.CandidateIndex]
		candJ := candidates[b.CandidateIndex]
		if candI.Route.ETASeconds != candJ.Route.ETASeconds {
			if candI.Route.ETASeconds < candJ.Route.ETASeconds {
				return -1
			}
			return 1
		}

		// Tie-breaker 2: Longer idle time (income fairness FIFO)
		if candI.Driver.IdleTimeSeconds != candJ.Driver.IdleTimeSeconds {
			if candI.Driver.IdleTimeSeconds > candJ.Driver.IdleTimeSeconds {
				return -1
			}
			return 1
		}

		// Tie-breaker 3: Higher Rating (R_star)
		if candI.Driver.Rating != candJ.Driver.Rating {
			if candI.Driver.Rating > candJ.Driver.Rating {
				return -1
			}
			return 1
		}

		return 0
	})

	var topCandidate *ScoringResult
	for i := range results {
		if results[i].MinScorePassed {
			topCandidate = &results[i]
			break
		}
	}

	return results, topCandidate
}

// FindAndRankDriversAdvanced applies 5 Driver Hard Filters, ranks candidates, and returns a DispatchDecision contract.
// If no candidate satisfies the active MinScore, it sets ShouldExpandRadius = true and suggests the next radius for location-svc.
func FindAndRankDriversAdvanced(candidates []Candidate, ctx BookingContext, now time.Time) DispatchDecision {
	cm := GetConfigManager()
	cfg := cm.GetConfig(ctx.ServiceType)
	effectiveMinScore := cfg.CalculateEffectiveMinScore(ctx.Attempt)

	// 1. Calculate suggested next radius for location-svc if current attempt fails
	currentRadius := ctx.CurrentRadiusMeters
	if currentRadius <= 0 {
		currentRadius = 2000.0
	}
	initialRadius := ctx.InitialRadiusMeters
	if initialRadius <= 0 {
		initialRadius = 2000.0
	}
	expansionRate := ctx.RadiusExpansionRate
	if expansionRate <= 0 {
		expansionRate = 1.5
	}
	maxRadius := ctx.MaxRadiusMeters
	if maxRadius <= 0 {
		maxRadius = 10000.0
	}

	suggestedNextRadius := currentRadius * expansionRate
	if suggestedNextRadius > maxRadius {
		suggestedNextRadius = maxRadius
	}

	// 2. Apply 5 Hard Driver Profile & State Filters
	validCandidates := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		d := c.Driver

		// Filter 1: Must be active IDLE
		if !d.IsIdle {
			continue
		}

		// Filter 2: Vehicle Type matching
		if ctx.ServiceType != "" && d.VehicleType != "" && d.VehicleType != ctx.ServiceType {
			continue
		}

		// Filter 3: Excluded Driver Check (previously rejected / timed out on this booking)
		isExcluded := false
		for _, exID := range ctx.ExcludedDriverIDs {
			if d.ID == exID {
				isExcluded = true
				break
			}
		}
		if isExcluded {
			continue
		}

		// Filter 4: Cooldown Lock Check
		if !d.CooldownUntil.IsZero() && d.CooldownUntil.After(now) {
			continue
		}

		// Filter 5: CASH Wallet Balance Filter (>= 20,000 VND)
		if ctx.PaymentMethod == PaymentCash && d.WalletBalance < 20000.0 {
			continue
		}

		validCandidates = append(validCandidates, c)
	}

	// 3. Rank filtered candidates
	results, topCandidate := RankCandidates(validCandidates, ctx)

	// 4. Formulate explicit DispatchDecision contract
	shouldExpand := (topCandidate == nil)

	return DispatchDecision{
		TopCandidate:        topCandidate,
		AllResults:          results,
		ShouldExpandRadius:  shouldExpand,
		SuggestedNextRadius: suggestedNextRadius,
		EffectiveMinScore:   effectiveMinScore,
	}
}
