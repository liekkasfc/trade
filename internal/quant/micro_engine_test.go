package quant

import (
	"math"
	"testing"
)

func TestComputeMicroDecisionPositiveSignalLowersTargetWeight(t *testing.T) {
	input := baseMicroInput()
	input.Chromosome.SignalWeightMeanRev = 0
	input.Chromosome.SignalWeightMomentum = 1.0

	got := ComputeMicroDecision(input)
	if got.Signal <= 0 {
		t.Fatalf("expected positive signal, got %f", got.Signal)
	}
	if got.TargetWeight >= 0.5 {
		t.Fatalf("expected target weight < 0.5, got %f", got.TargetWeight)
	}
}

func TestComputeMicroDecisionNegativeSignalRaisesTargetWeight(t *testing.T) {
	input := baseMicroInput()
	input.Chromosome.SignalWeightMeanRev = 0
	input.Chromosome.SignalWeightMomentum = -1.0

	got := ComputeMicroDecision(input)
	if got.Signal >= 0 {
		t.Fatalf("expected negative signal, got %f", got.Signal)
	}
	if got.TargetWeight <= 0.5 {
		t.Fatalf("expected target weight > 0.5, got %f", got.TargetWeight)
	}
}

func TestComputeMicroDecisionNeutralInventoryBias(t *testing.T) {
	input := baseMicroInput()
	input.CurrentWeight = 0.5
	input.Chromosome.SignalWeightMeanRev = 0
	input.Chromosome.SignalWeightMomentum = 0
	input.Chromosome.Gamma = 2.0

	got := ComputeMicroDecision(input)
	if math.Abs(got.TargetWeight-0.5) > 1e-9 {
		t.Fatalf("expected target weight 0.5, got %.12f", got.TargetWeight)
	}
}

func TestComputeMicroDecisionIsDeterministic(t *testing.T) {
	input := baseMicroInput()
	first := ComputeMicroDecision(input)
	second := ComputeMicroDecision(input)
	if first != second {
		t.Fatalf("expected deterministic output, got %+v and %+v", first, second)
	}
}

func TestComputeMicroDecisionQuietSuppressesDustOrders(t *testing.T) {
	input := baseMicroInput()
	input.TotalEquity = 50
	input.CurrentWeight = 0.49
	input.MarketState.IsQuiet = true
	input.Chromosome.SignalWeightMeanRev = 0
	input.Chromosome.SignalWeightMomentum = 0.3

	got := ComputeMicroDecision(input)
	if math.Abs(got.TheoreticalUSD) >= MinOrderUSDT {
		t.Fatalf("expected dust order, got theoretical %f", got.TheoreticalUSD)
	}
	if got.OrderUSD != 0 {
		t.Fatalf("expected quiet dust order to be suppressed, got %f", got.OrderUSD)
	}
}

func TestComputeMicroDecisionWedgeForcesMinimumOrder(t *testing.T) {
	input := baseMicroInput()
	input.TotalEquity = 50
	input.CurrentWeight = 0.49
	input.MarketState.IsQuiet = false
	input.Chromosome.SignalWeightMeanRev = 0
	input.Chromosome.SignalWeightMomentum = 0.3
	input.Chromosome.MicroDeltaWeightThreshold = 0.001
	input.Chromosome.QuietVolRatioThreshold = 10

	got := ComputeMicroDecision(input)
	if math.Abs(got.TheoreticalUSD) >= MinOrderUSDT {
		t.Fatalf("expected dust order, got theoretical %f", got.TheoreticalUSD)
	}
	if math.Abs(math.Abs(got.OrderUSD)-MinOrderUSDT) > 1e-9 {
		t.Fatalf("expected wedge filter to force min order magnitude %.2f, got %f", MinOrderUSDT, got.OrderUSD)
	}
}

func baseMicroInput() MicroDecisionInput {
	closes := make([]float64, 0, 160)
	price := 100.0
	for i := 0; i < 160; i++ {
		price += 0.6
		closes = append(closes, price)
	}
	return MicroDecisionInput{
		Closes:        closes,
		CurrentPrice:  closes[len(closes)-1],
		CurrentWeight: 0.5,
		TotalEquity:   1000,
		Chromosome:    DefaultSeedChromosome,
		MarketState: MarketState{
			State:                  MarketStateRange,
			TimeDilationMultiplier: 1,
			BetaMultiplier:         1,
			IsQuiet:                false,
		},
	}
}
