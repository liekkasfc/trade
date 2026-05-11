package quant

import "math"

const (
	MarketStateBull  = "bull"
	MarketStateBear  = "bear"
	MarketStateRange = "range"
)

type MarketState struct {
	State                  string
	TimeDilationMultiplier float64
	BetaMultiplier         float64
	IsQuiet                bool
}

func ComputeMarketState(closes []float64) MarketState {
	if len(closes) < MarketFastEMABars+2 {
		return MarketState{
			State:                  MarketStateRange,
			TimeDilationMultiplier: 1.0,
			BetaMultiplier:         1.0,
			IsQuiet:                true,
		}
	}

	price := closes[len(closes)-1]
	fastEMA := EMA(closes, MarketFastEMABars)
	slowEMA := EMA(closes, MarketSlowEMABars)
	shortVol := MAVAbsChange(closes, MarketQuietShortBars)
	longVol := MAVAbsChange(closes, MarketQuietLongBars)
	volRatio := 1.0
	if longVol > epsilon {
		volRatio = ClipFloat64(shortVol/longVol, 0.1, 3.0)
	}
	quiet := volRatio < 0.75

	trendSpread := (fastEMA - slowEMA) / math.Max(slowEMA, epsilon)
	priceSpread := (price - slowEMA) / math.Max(slowEMA, epsilon)

	switch {
	case trendSpread > 0.01 && priceSpread > 0.015:
		return MarketState{
			State:                  MarketStateBull,
			TimeDilationMultiplier: 0.9,
			BetaMultiplier:         1.1,
			IsQuiet:                quiet,
		}
	case trendSpread < -0.01 && priceSpread < -0.015:
		return MarketState{
			State:                  MarketStateBear,
			TimeDilationMultiplier: 1.5,
			BetaMultiplier:         0.6,
			IsQuiet:                quiet,
		}
	default:
		return MarketState{
			State:                  MarketStateRange,
			TimeDilationMultiplier: 1.0,
			BetaMultiplier:         1.0,
			IsQuiet:                quiet,
		}
	}
}
