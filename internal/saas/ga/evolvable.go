package ga

import (
	"context"
	"encoding/json"
	"math/rand"

	"quantsaas/internal/quant"
)

type Gene = any

type DCABaseline struct {
	FinalEquity   float64 `json:"final_equity"`
	TotalInjected float64 `json:"total_injected"`
	MaxDrawdown   float64 `json:"max_drawdown"`
	ROI           float64 `json:"roi"`
}

type EvaluablePlan struct {
	TemplateID         string
	Symbol             string
	SpawnPoint         quant.SpawnPoint
	Windows            []quant.CrucibleWindow
	DCABaselines       []DCABaseline
	LotStep            float64
	LotMin             float64
	InitialCapitalUSDT float64
}

type FitnessResult struct {
	ScoreTotal   float64
	MaxDrawdown  float64
	TradeCount   int
	WindowScores []quant.CrucibleResult
}

type EvolvableStrategy interface {
	StrategyID() string
	Sample(rng *rand.Rand) Gene
	Mutate(g Gene, prob, scale float64, rng *rand.Rand) Gene
	Crossover(p1, p2 Gene, rng *rand.Rand) Gene
	Fingerprint(g Gene) string
	Evaluate(ctx context.Context, g Gene, plan EvaluablePlan) (FitnessResult, error)
	DecodeElite(raw json.RawMessage) Gene
	EncodeResult(g Gene, spawn quant.SpawnPoint) (json.RawMessage, error)
}
