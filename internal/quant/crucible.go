package quant

import (
	"errors"
	"sort"
	"time"
)

type CrucibleWindow struct {
	Label       string
	Weight      float64
	Bars        []Bar
	EvalStartMs int64
}

type CrucibleResult struct {
	Window string
	Score  float64
	ROI    float64
	MaxDD  float64
	Alpha  float64
}

func BuildCrucibleWindows(bars []Bar, warmupDays int) ([]CrucibleWindow, error) {
	if len(bars) == 0 {
		return nil, errors.New("bars are required")
	}
	if warmupDays <= 0 {
		warmupDays = defaultEvalWarmupDays
	}

	latest := bars[len(bars)-1].OpenTime
	warmupDuration := int64(warmupDays) * int64(24*time.Hour/time.Millisecond)

	build := func(label string, days int, weight float64) CrucibleWindow {
		if label == "full" {
			return CrucibleWindow{
				Label:       label,
				Weight:      weight,
				Bars:        append([]Bar(nil), bars...),
				EvalStartMs: bars[0].OpenTime,
			}
		}

		evalStartCutoff := latest - int64(days)*int64(24*time.Hour/time.Millisecond)
		evalStartIdx := firstBarAtOrAfter(bars, evalStartCutoff)
		if evalStartIdx < 0 {
			evalStartIdx = 0
		}
		evalStartMS := bars[evalStartIdx].OpenTime
		warmupCutoff := evalStartMS - warmupDuration
		startIdx := firstBarAtOrAfter(bars, warmupCutoff)
		if startIdx < 0 || startIdx > evalStartIdx {
			startIdx = 0
		}
		return CrucibleWindow{
			Label:       label,
			Weight:      weight,
			Bars:        append([]Bar(nil), bars[startIdx:]...),
			EvalStartMs: evalStartMS,
		}
	}

	windows := []CrucibleWindow{
		build("6m", 183, 0.10),
		build("2y", 730, 0.20),
		build("5y", 1825, 0.30),
		build("full", 0, 0.40),
	}

	sort.SliceStable(windows, func(i, j int) bool {
		return len(windows[i].Bars) < len(windows[j].Bars)
	})

	return windows, nil
}

func FilterEvaluationBars(window CrucibleWindow) []Bar {
	idx := firstBarAtOrAfter(window.Bars, window.EvalStartMs)
	if idx < 0 {
		return nil
	}
	return window.Bars[idx:]
}

func firstBarAtOrAfter(bars []Bar, cutoff int64) int {
	for i, bar := range bars {
		if bar.OpenTime >= cutoff {
			return i
		}
	}
	return -1
}
