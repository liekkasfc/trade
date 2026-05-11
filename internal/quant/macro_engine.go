package quant

import (
	"math"
	"time"
)

type MacroDecisionInput struct {
	CurrentPrice     float64
	Timestamp        int64
	Closes           []float64
	TotalEquity      float64
	SpendableUSDT    float64
	MarketState      MarketState
	Chromosome       Chromosome
	SpawnPoint       SpawnPoint
	LastMacroBuyAt   int64
	LastExtraBuyAt   int64
	LastIdleDeployAt int64
}

type MacroDecision struct {
	OrderUSD     float64
	Reason       string
	UsedBaseDCA  bool
	UsedExtraBuy bool
	UsedIdleBuy  bool
	Discount     float64
}

func ComputeMacroDecision(input MacroDecisionInput) MacroDecision {
	if input.SpendableUSDT < MinOrderUSDT || input.CurrentPrice <= 0 || len(input.Closes) == 0 {
		return MacroDecision{}
	}

	var totalOrder float64
	var reason string
	var usedBase, usedExtra, usedIdle bool

	if shouldRunMonthlyDCA(input.Timestamp, input.LastMacroBuyAt) {
		base := minFloat(input.SpendableUSDT, input.SpawnPoint.Policy.MonthlyInjectUSDT)
		totalOrder += base
		usedBase = base >= MinOrderUSDT
		if usedBase {
			reason = "monthly_dca"
		}
	}

	anchor := EMA(input.Closes, MacroAnchorEMABars)
	discount := 0.0
	if anchor > epsilon {
		discount = math.Max(0, (anchor-input.CurrentPrice)/anchor)
	}
	threshold := input.Chromosome.DiscountBuyThreshold
	capPct := input.SpawnPoint.Policy.ExtraBuyCapPct
	if input.MarketState.State == MarketStateBear {
		threshold *= 1.5
		capPct = input.SpawnPoint.Policy.BearExtraBuyCapPct
	}

	if discount >= threshold && hoursSince(input.Timestamp, input.LastExtraBuyAt) >= 72 {
		scale := math.Min(1.25, discount/math.Max(threshold, epsilon))
		extraBudget := input.TotalEquity * capPct * input.Chromosome.DiscountBuyScale * scale
		extra := minFloat(input.SpendableUSDT-totalOrder, extraBudget)
		if extra >= MinOrderUSDT {
			totalOrder += extra
			usedExtra = true
			if reason == "" {
				reason = "discount_buy"
			} else {
				reason += "+discount_buy"
			}
		}
	}

	lastDeployAt := maxInt64(input.LastMacroBuyAt, input.LastExtraBuyAt, input.LastIdleDeployAt)
	if totalOrder < MinOrderUSDT && daysSince(input.Timestamp, lastDeployAt) >= input.SpawnPoint.Policy.IdleDeployDeadlineDays {
		idleBudget := input.TotalEquity * math.Max(0.05, capPct*0.5)
		idleOrder := minFloat(input.SpendableUSDT, idleBudget)
		if idleOrder >= MinOrderUSDT {
			totalOrder = idleOrder
			usedIdle = true
			reason = "idle_deploy"
		}
	}

	totalOrder = minFloat(totalOrder, input.SpendableUSDT)
	totalOrder = RoundToUSDT(totalOrder)
	if totalOrder < MinOrderUSDT {
		return MacroDecision{Discount: discount}
	}

	return MacroDecision{
		OrderUSD:     totalOrder,
		Reason:       reason,
		UsedBaseDCA:  usedBase,
		UsedExtraBuy: usedExtra,
		UsedIdleBuy:  usedIdle,
		Discount:     discount,
	}
}

func shouldRunMonthlyDCA(nowMS, lastMS int64) bool {
	if lastMS == 0 {
		return true
	}
	now := time.UnixMilli(nowMS).UTC()
	last := time.UnixMilli(lastMS).UTC()
	return now.Year() != last.Year() || now.Month() != last.Month()
}

func hoursSince(nowMS, thenMS int64) float64 {
	if thenMS == 0 || nowMS <= thenMS {
		return math.MaxFloat64
	}
	return float64(nowMS-thenMS) / float64(time.Hour/time.Millisecond)
}

func daysSince(nowMS, thenMS int64) int {
	if thenMS == 0 || nowMS <= thenMS {
		return math.MaxInt / 2
	}
	return int((nowMS - thenMS) / int64(24*time.Hour/time.Millisecond))
}

func maxInt64(values ...int64) int64 {
	var out int64
	for _, v := range values {
		if v > out {
			out = v
		}
	}
	return out
}
