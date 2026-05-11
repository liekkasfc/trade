package quant

import (
	"errors"
	"time"
)

type GhostDCAConfig struct {
	InitialCapital float64
	MonthlyInject  float64
}

type GhostDCAResult struct {
	FinalEquity   float64
	TotalInjected float64
	MaxDrawdown   float64
	ROI           float64
	NAV           []float64
}

type Cashflow struct {
	Timestamp int64
	Amount    float64
}

func SimulateGhostDCA(bars []Bar, cfg GhostDCAConfig) (GhostDCAResult, error) {
	if len(bars) == 0 {
		return GhostDCAResult{}, errors.New("bars are required")
	}
	if cfg.InitialCapital <= 0 {
		return GhostDCAResult{}, errors.New("initial capital must be positive")
	}

	firstPrice := bars[0].Close
	if firstPrice <= 0 {
		return GhostDCAResult{}, errors.New("first bar close must be positive")
	}

	qty := cfg.InitialCapital / firstPrice
	totalInjected := cfg.InitialCapital
	flows := make([]Cashflow, 0, 16)
	nav := make([]float64, 0, len(bars))

	lastMonth := time.UnixMilli(bars[0].OpenTime).UTC().Month()
	lastYear := time.UnixMilli(bars[0].OpenTime).UTC().Year()

	for i, bar := range bars {
		ts := time.UnixMilli(bar.OpenTime).UTC()
		if i > 0 && cfg.MonthlyInject > 0 && (ts.Month() != lastMonth || ts.Year() != lastYear) {
			qty += cfg.MonthlyInject / bar.Close
			totalInjected += cfg.MonthlyInject
			flows = append(flows, Cashflow{
				Timestamp: bar.OpenTime,
				Amount:    cfg.MonthlyInject,
			})
		}
		lastMonth = ts.Month()
		lastYear = ts.Year()
		nav = append(nav, qty*bar.Close)
	}

	finalEquity := nav[len(nav)-1]
	roi := ModifiedDietzROI(cfg.InitialCapital, finalEquity, flows, bars[0].OpenTime, bars[len(bars)-1].OpenTime)

	return GhostDCAResult{
		FinalEquity:   finalEquity,
		TotalInjected: totalInjected,
		MaxDrawdown:   MaxDrawdown(nav),
		ROI:           roi,
		NAV:           nav,
	}, nil
}

func ModifiedDietzROI(initialCapital, finalEquity float64, flows []Cashflow, startMS, endMS int64) float64 {
	if initialCapital <= 0 || endMS <= startMS {
		return 0
	}

	totalDays := float64(endMS-startMS) / float64(24*time.Hour/time.Millisecond)
	if totalDays <= 0 {
		return 0
	}

	var flowSum float64
	denominator := initialCapital
	for _, flow := range flows {
		if flow.Timestamp <= startMS || flow.Timestamp > endMS {
			continue
		}
		flowSum += flow.Amount
		remainingDays := float64(endMS-flow.Timestamp) / float64(24*time.Hour/time.Millisecond)
		weight := remainingDays / totalDays
		denominator += flow.Amount * weight
	}
	if denominator <= 0 {
		return 0
	}

	return (finalEquity - initialCapital - flowSum) / denominator
}

func MaxDrawdown(nav []float64) float64 {
	if len(nav) == 0 {
		return 0
	}
	peak := nav[0]
	maxDD := 0.0
	for _, v := range nav {
		if v > peak {
			peak = v
			continue
		}
		if peak <= 0 {
			continue
		}
		dd := (peak - v) / peak
		if dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}
