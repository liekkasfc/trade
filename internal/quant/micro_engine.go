package quant

import (
	"math"
)

type MicroDecisionInput struct {
	Closes        []float64
	CurrentPrice  float64
	CurrentWeight float64
	TotalEquity   float64
	Chromosome    Chromosome
	MarketState   MarketState
}

type MicroDecision struct {
	TargetWeight    float64
	Signal          float64
	TheoreticalUSD  float64
	OrderUSD        float64
	VolatilityRatio float64
	DeltaWeight     float64
}

// ComputeMicroDecision implements the Sigmoid dynamic balance:
// signal is the external market force, inventory bias is the restoring spring,
// beta controls spring stiffness, gamma enables the restoring force, and the
// volatility ratio filter suppresses dust orders during quiet periods.
func ComputeMicroDecision(input MicroDecisionInput) MicroDecision {
	if len(input.Closes) < 2 || input.TotalEquity <= 0 || input.CurrentPrice <= 0 {
		return MicroDecision{}
	}

	ema := EMA(input.Closes, MicroSignalEMABars)
	rawStdDev := StdDev(input.Closes, MicroSignalStdDevBars)
	relativeSigma := math.Max(rawStdDev/math.Max(input.CurrentPrice, epsilon), input.Chromosome.SigmaFloor)
	if relativeSigma <= 0 {
		return MicroDecision{}
	}

	deviation := (input.CurrentPrice - ema) / math.Max(rawStdDev, input.CurrentPrice*input.Chromosome.SigmaFloor)
	momentumBase := input.Closes[max(0, len(input.Closes)-1-MicroMomentumBars)]
	momentum := SafeLogReturn(input.CurrentPrice, momentumBase) / math.Max(relativeSigma, epsilon)
	signal := input.Chromosome.SignalWeightMeanRev*deviation + input.Chromosome.SignalWeightMomentum*momentum

	effectiveBeta := math.Max(0.01, input.Chromosome.Beta*input.MarketState.BetaMultiplier)
	inventoryBias := ClipFloat64(input.CurrentWeight, 0, 1) - 0.5
	exponent := effectiveBeta*signal + input.Chromosome.Gamma*inventoryBias
	targetWeight := ClipFloat64(1/(1+math.Exp(exponent)), 0, 1)
	deltaWeight := targetWeight - input.CurrentWeight
	theoreticalUSD := deltaWeight * input.TotalEquity

	shortVol := MAVAbsChange(input.Closes, MicroVolRatioShortBars)
	longVol := MAVAbsChange(input.Closes, MicroVolRatioLongBars)
	volRatio := 1.0
	if longVol > epsilon {
		volRatio = ClipFloat64(shortVol/longVol, 0.1, 3.0)
	}

	orderUSD := 0.0
	if math.Abs(theoreticalUSD) >= MinOrderUSDT {
		orderUSD = RoundToUSDT(theoreticalUSD)
	} else if !input.MarketState.IsQuiet && (math.Abs(deltaWeight) >= input.Chromosome.MicroDeltaWeightThreshold || volRatio >= input.Chromosome.QuietVolRatioThreshold) {
		orderUSD = math.Copysign(MinOrderUSDT, theoreticalUSD)
	}

	return MicroDecision{
		TargetWeight:    targetWeight,
		Signal:          signal,
		TheoreticalUSD:  theoreticalUSD,
		OrderUSD:        orderUSD,
		VolatilityRatio: volRatio,
		DeltaWeight:     deltaWeight,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
