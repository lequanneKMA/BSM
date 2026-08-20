package scoring

import (
	"time"
)

// SolverType identifies which batch matching solver was selected by the Strategy Router
type SolverType string

const (
	SolverHungarian SolverType = "HUNGARIAN"
	SolverAuction   SolverType = "AUCTION"
	SolverGreedy    SolverType = "GREEDY_FALLBACK"
)

// MatchAssignment represents a solved pairing of order index (M) to driver index (N)
type MatchAssignment struct {
	OrderIndex  int     `json:"order_index"`
	DriverIndex int     `json:"driver_index"`
	Weight      float64 `json:"weight"`
}

// BatchSolveResult holds the matching assignments and metadata
type BatchSolveResult struct {
	Assignments []MatchAssignment `json:"assignments"`
	SolverUsed  SolverType        `json:"solver_used"`
	SolveTimeMs float64           `json:"solve_time_ms"`
}

// StrategyRouter routes batch matching requests to the appropriate solver based on matrix size V
func StrategyRouter(weightMatrix [][]float64, timeoutBudget time.Duration) BatchSolveResult {
	start := time.Now()
	numOrders := len(weightMatrix)
	if numOrders == 0 {
		return BatchSolveResult{SolverUsed: SolverGreedy, SolveTimeMs: 0}
	}
	numDrivers := len(weightMatrix[0])

	V := numOrders
	if numDrivers > V {
		V = numDrivers
	}

	// Rule 1: Fallback if V > 200 or timeout budget < 1ms
	if V > 200 || timeoutBudget < time.Millisecond {
		return solveGreedy(weightMatrix, SolverGreedy, start)
	}

	// Rule 2: Hungarian algorithm for V <= 30
	if V <= 30 {
		return solveHungarian(weightMatrix, start)
	}

	// Rule 3: Bertsekas' Auction algorithm (epsilon = 1.0) for 30 < V <= 200
	return solveAuction(weightMatrix, 1.0, start)
}

// solveGreedy provides O(V) fast greedy single-assignment
func solveGreedy(weights [][]float64, solverType SolverType, start time.Time) BatchSolveResult {
	numOrders := len(weights)
	numDrivers := len(weights[0])

	assignedDrivers := make([]bool, numDrivers)
	assignments := make([]MatchAssignment, 0, numOrders)

	for i := 0; i < numOrders; i++ {
		bestJ := -1
		bestW := -1.0
		for j := 0; j < numDrivers; j++ {
			if !assignedDrivers[j] && weights[i][j] > bestW {
				bestW = weights[i][j]
				bestJ = j
			}
		}
		if bestJ != -1 && bestW > 0 {
			assignedDrivers[bestJ] = true
			assignments = append(assignments, MatchAssignment{
				OrderIndex:  i,
				DriverIndex: bestJ,
				Weight:      bestW,
			})
		}
	}

	return BatchSolveResult{
		Assignments: assignments,
		SolverUsed:  solverType,
		SolveTimeMs: float64(time.Since(start).Microseconds()) / 1000.0,
	}
}

// solveHungarian implements O(V^3) Kuhn-Munkres Maximization algorithm for V <= 30
func solveHungarian(weights [][]float64, start time.Time) BatchSolveResult {
	n := len(weights)
	m := len(weights[0])
	maxDim := n
	if m > maxDim {
		maxDim = m
	}

	// Build square cost matrix for minimization (C_max - weight)
	cMax := 130.0
	cost := make([][]float64, maxDim)
	for i := 0; i < maxDim; i++ {
		cost[i] = make([]float64, maxDim)
		for j := 0; j < maxDim; j++ {
			if i < n && j < m {
				cost[i][j] = cMax - weights[i][j]
			} else {
				cost[i][j] = cMax
			}
		}
	}

	// Hungarian Algorithm (Kuhn-Munkres)
	u := make([]float64, maxDim+1)
	v := make([]float64, maxDim+1)
	p := make([]int, maxDim+1)
	way := make([]int, maxDim+1)

	for i := 1; i <= maxDim; i++ {
		p[0] = i
		j0 := 0
		minv := make([]float64, maxDim+1)
		for k := 0; k <= maxDim; k++ {
			minv[k] = 1e9
		}
		used := make([]bool, maxDim+1)
		for {
			used[j0] = true
			i0 := p[j0]
			delta := 1e9
			j1 := 0
			for j := 1; j <= maxDim; j++ {
				if !used[j] {
					cur := cost[i0-1][j-1] - u[i0] - v[j]
					if cur < minv[j] {
						minv[j] = cur
						way[j] = j0
					}
					if minv[j] < delta {
						delta = minv[j]
						j1 = j
					}
				}
			}
			for j := 0; j <= maxDim; j++ {
				if used[j] {
					u[p[j]] += delta
					v[j] -= delta
				} else {
					minv[j] -= delta
				}
			}
			j0 = j1
			if p[j0] == 0 {
				break
			}
		}
		for {
			j1 := way[j0]
			p[j0] = p[j1]
			j0 = j1
			if j0 == 0 {
				break
			}
		}
	}

	assignments := make([]MatchAssignment, 0, n)
	for j := 1; j <= maxDim; j++ {
		i := p[j] - 1
		jIdx := j - 1
		if i >= 0 && i < n && jIdx < m {
			w := weights[i][jIdx]
			if w > 0 {
				assignments = append(assignments, MatchAssignment{
					OrderIndex:  i,
					DriverIndex: jIdx,
					Weight:      w,
				})
			}
		}
	}

	return BatchSolveResult{
		Assignments: assignments,
		SolverUsed:  SolverHungarian,
		SolveTimeMs: float64(time.Since(start).Microseconds()) / 1000.0,
	}
}

// solveAuction implements Bertsekas' Auction Algorithm (with step epsilon = 1.0) for 30 < V <= 200
func solveAuction(weights [][]float64, epsilon float64, start time.Time) BatchSolveResult {
	n := len(weights)
	m := len(weights[0])

	prices := make([]float64, m)
	personAssignment := make([]int, n) // order -> driver (-1 if unassigned)
	itemAssignment := make([]int, m)   // driver -> order (-1 if unassigned)
	for i := 0; i < n; i++ {
		personAssignment[i] = -1
	}
	for j := 0; j < m; j++ {
		itemAssignment[j] = -1
	}

	unassigned := make([]int, n)
	for i := 0; i < n; i++ {
		unassigned[i] = i
	}

	maxIter := n * 50
	iter := 0

	for len(unassigned) > 0 && iter < maxIter {
		iter++
		i := unassigned[0]
		unassigned = unassigned[1:]

		// Find best and second best value (weight[i][j] - price[j])
		bestJ := -1
		bestVal := -1e9
		secondBestVal := -1e9

		for j := 0; j < m; j++ {
			val := weights[i][j] - prices[j]
			if val > bestVal {
				secondBestVal = bestVal
				bestVal = val
				bestJ = j
			} else if val > secondBestVal {
				secondBestVal = val
			}
		}

		if bestJ != -1 {
			bid := bestVal - secondBestVal + epsilon
			prices[bestJ] += bid

			// Evict previous owner if exists
			if itemAssignment[bestJ] != -1 {
				prevOwner := itemAssignment[bestJ]
				personAssignment[prevOwner] = -1
				unassigned = append(unassigned, prevOwner)
			}

			itemAssignment[bestJ] = i
			personAssignment[i] = bestJ
		}
	}

	assignments := make([]MatchAssignment, 0, n)
	for i := 0; i < n; i++ {
		j := personAssignment[i]
		if j != -1 && j < m {
			assignments = append(assignments, MatchAssignment{
				OrderIndex:  i,
				DriverIndex: j,
				Weight:      weights[i][j],
			})
		}
	}

	return BatchSolveResult{
		Assignments: assignments,
		SolverUsed:  SolverAuction,
		SolveTimeMs: float64(time.Since(start).Microseconds()) / 1000.0,
	}
}
