package ga

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"

	"quantsaas/internal/adapters/backtest"
	"quantsaas/internal/quant"
	"quantsaas/internal/strategies"
	"quantsaas/internal/strategies/coretemplate"
)

type TemplateEvolvable struct {
	spec strategies.Spec
}

func NewTemplateEvolvable(spec strategies.Spec) *TemplateEvolvable {
	return &TemplateEvolvable{spec: spec}
}

func (t *TemplateEvolvable) StrategyID() string {
	return t.spec.Manifest.ID
}

func (t *TemplateEvolvable) Sample(rng *rand.Rand) Gene {
	return quant.SampleChromosome(rng)
}

func (t *TemplateEvolvable) Mutate(g Gene, prob, scale float64, rng *rand.Rand) Gene {
	chromosome := geneAsChromosome(g, t.spec.DefaultChromosome())
	mutateField := func(value float64, bound quant.Bound) float64 {
		if rng.Float64() >= prob {
			return value
		}
		next := value + rng.NormFloat64()*bound.Step*scale
		return quant.ClipFloat64(next, bound.Min, bound.Max)
	}

	chromosome.SignalWeightMeanRev = mutateField(chromosome.SignalWeightMeanRev, quant.HardBounds["signal_weight_mean_rev"])
	chromosome.SignalWeightMomentum = mutateField(chromosome.SignalWeightMomentum, quant.HardBounds["signal_weight_momentum"])
	chromosome.SigmaFloor = mutateField(chromosome.SigmaFloor, quant.HardBounds["sigma_floor"])
	chromosome.Beta = mutateField(chromosome.Beta, quant.HardBounds["beta"])
	chromosome.Gamma = mutateField(chromosome.Gamma, quant.HardBounds["gamma"])
	chromosome.DiscountBuyThreshold = mutateField(chromosome.DiscountBuyThreshold, quant.HardBounds["discount_buy_threshold"])
	chromosome.DiscountBuyScale = mutateField(chromosome.DiscountBuyScale, quant.HardBounds["discount_buy_scale"])
	chromosome.QuietVolRatioThreshold = mutateField(chromosome.QuietVolRatioThreshold, quant.HardBounds["quiet_vol_ratio_threshold"])
	chromosome.MicroDeltaWeightThreshold = mutateField(chromosome.MicroDeltaWeightThreshold, quant.HardBounds["micro_delta_weight_threshold"])
	chromosome.MicroReservePct = mutateField(chromosome.MicroReservePct, quant.HardBounds["micro_reserve_pct"])
	return quant.ClampChromosome(chromosome)
}

func (t *TemplateEvolvable) Crossover(p1, p2 Gene, rng *rand.Rand) Gene {
	left := geneAsChromosome(p1, t.spec.DefaultChromosome())
	right := geneAsChromosome(p2, t.spec.DefaultChromosome())
	pick := func(a, b float64) float64 {
		if rng.Float64() < 0.5 {
			return a
		}
		return b
	}
	return quant.ClampChromosome(quant.Chromosome{
		SignalWeightMeanRev:       pick(left.SignalWeightMeanRev, right.SignalWeightMeanRev),
		SignalWeightMomentum:      pick(left.SignalWeightMomentum, right.SignalWeightMomentum),
		SigmaFloor:                pick(left.SigmaFloor, right.SigmaFloor),
		Beta:                      pick(left.Beta, right.Beta),
		Gamma:                     pick(left.Gamma, right.Gamma),
		DiscountBuyThreshold:      pick(left.DiscountBuyThreshold, right.DiscountBuyThreshold),
		DiscountBuyScale:          pick(left.DiscountBuyScale, right.DiscountBuyScale),
		QuietVolRatioThreshold:    pick(left.QuietVolRatioThreshold, right.QuietVolRatioThreshold),
		MicroDeltaWeightThreshold: pick(left.MicroDeltaWeightThreshold, right.MicroDeltaWeightThreshold),
		MicroReservePct:           pick(left.MicroReservePct, right.MicroReservePct),
	})
}

func (t *TemplateEvolvable) Fingerprint(g Gene) string {
	chromosome := geneAsChromosome(g, t.spec.DefaultChromosome())
	round := func(v float64) int64 {
		return int64(math.Round(v * 1_000_000))
	}
	hasher := fnv.New64a()
	fmt.Fprintf(hasher, "%d|%d|%d|%d|%d|%d|%d|%d|%d|%d",
		round(chromosome.SignalWeightMeanRev),
		round(chromosome.SignalWeightMomentum),
		round(chromosome.SigmaFloor),
		round(chromosome.Beta),
		round(chromosome.Gamma),
		round(chromosome.DiscountBuyThreshold),
		round(chromosome.DiscountBuyScale),
		round(chromosome.QuietVolRatioThreshold),
		round(chromosome.MicroDeltaWeightThreshold),
		round(chromosome.MicroReservePct),
	)
	return fmt.Sprintf("%x", hasher.Sum64())
}

func (t *TemplateEvolvable) Evaluate(ctx context.Context, g Gene, plan EvaluablePlan) (FitnessResult, error) {
	chromosome := geneAsChromosome(g, t.spec.DefaultChromosome())
	params := coretemplate.Params{
		Chromosome: chromosome,
		SpawnPoint: plan.SpawnPoint,
	}
	runner := t.spec.NewBacktestRunner(params)
	windowScores := make([]quant.CrucibleResult, 0, len(plan.Windows))
	scoreTotal := 0.0
	maxDrawdown := 0.0
	tradeCount := 0

	for i, window := range plan.Windows {
		baseline := plan.DCABaselines[i]
		result, err := backtest.Run(ctx, backtest.RunOptions{
			TemplateID:         plan.TemplateID,
			Symbol:             plan.Symbol,
			Bars:               window.Bars,
			EvalStartMS:        window.EvalStartMs,
			InitialCapitalUSDT: plan.InitialCapitalUSDT,
			SpawnPoint:         plan.SpawnPoint,
			Runner:             runner,
		})
		if err != nil {
			return FitnessResult{}, err
		}

		alpha := result.ROI - baseline.ROI
		sliceScore := scoreFitnessWindow(alpha, result.MaxDrawdown, baseline.MaxDrawdown, result.TradeCount, plan.Fitness)
		if result.MaxDrawdown >= plan.Fitness.FatalMaxDrawdown {
			return FitnessResult{
				ScoreTotal:  -99999,
				MaxDrawdown: result.MaxDrawdown,
				TradeCount:  result.TradeCount,
				WindowScores: []quant.CrucibleResult{
					{
						Window: window.Label,
						Score:  -99999,
						ROI:    result.ROI,
						MaxDD:  result.MaxDrawdown,
						Alpha:  alpha,
					},
				},
			}, nil
		}

		scoreTotal += window.Weight * sliceScore
		if result.MaxDrawdown > maxDrawdown {
			maxDrawdown = result.MaxDrawdown
		}
		tradeCount += result.TradeCount
		windowScores = append(windowScores, quant.CrucibleResult{
			Window: window.Label,
			Score:  sliceScore,
			ROI:    result.ROI,
			MaxDD:  result.MaxDrawdown,
			Alpha:  alpha,
		})
	}

	return FitnessResult{
		ScoreTotal:   scoreTotal,
		MaxDrawdown:  maxDrawdown,
		TradeCount:   tradeCount,
		WindowScores: windowScores,
	}, nil
}

func scoreFitnessWindow(alpha, maxDrawdown, baselineMaxDrawdown float64, tradeCount int, cfg FitnessConfig) float64 {
	baselineDDPenalty := cfg.BaselineDrawdownPenalty * math.Max(0, maxDrawdown-baselineMaxDrawdown)
	targetDDPenalty := cfg.DrawdownPenaltyFactor * math.Max(0, maxDrawdown-cfg.TargetMaxDrawdown)
	tradePenalty := cfg.TradeCountPenaltyFactor * float64(tradeCount)
	return alpha - baselineDDPenalty - targetDDPenalty - tradePenalty
}

func (t *TemplateEvolvable) DecodeElite(raw json.RawMessage) Gene {
	params, err := t.spec.ParseParamPack(raw)
	if err != nil {
		return t.spec.DefaultChromosome()
	}
	return params.Chromosome
}

func (t *TemplateEvolvable) EncodeResult(g Gene, spawn quant.SpawnPoint) (json.RawMessage, error) {
	chromosome := geneAsChromosome(g, t.spec.DefaultChromosome())
	return t.spec.EncodeParamPack(coretemplate.Params{
		Chromosome: chromosome,
		SpawnPoint: spawn,
	})
}

func geneAsChromosome(g Gene, fallback quant.Chromosome) quant.Chromosome {
	chromosome, ok := g.(quant.Chromosome)
	if !ok {
		return fallback
	}
	return quant.ClampChromosome(chromosome)
}
