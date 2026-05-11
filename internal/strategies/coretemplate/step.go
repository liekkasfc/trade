package coretemplate

import (
	"math"
	"time"

	"quantsaas/internal/quant"
)

func Step(manifest Manifest, input quant.StrategyInput, params Params) quant.StrategyOutput {
	state := DecodeState(input.RuntimeState)
	state.LastProcessedBarTime = input.Timestamp

	if len(input.Closes) < quant.MinimumStrategyBars || len(input.Closes) != len(input.Timestamps) {
		return quant.StrategyOutput{
			UpdatedRuntimeState: EncodeState(state),
			Diagnostics: quant.StrategyDiagnostics{
				Notes: []string{"insufficient_history"},
			},
		}
	}

	price := input.Closes[len(input.Closes)-1]
	marketState := quant.ComputeMarketState(input.Closes)
	totalEquity := input.Portfolio.TotalEquity(price)
	if totalEquity <= 0 {
		return quant.StrategyOutput{
			UpdatedRuntimeState: EncodeState(state),
			Diagnostics: quant.StrategyDiagnostics{
				MarketState: marketState.State,
				IsQuiet:     marketState.IsQuiet,
				Notes:       []string{"non_positive_equity"},
			},
		}
	}

	effectiveReservePct := params.Chromosome.MicroReservePct
	if marketState.State == quant.MarketStateBear && effectiveReservePct < 0.40 {
		effectiveReservePct = 0.40
	}
	reserveFloor := totalEquity * effectiveReservePct
	spendableUSDT := math.Max(0, input.Portfolio.USDTBalance-reserveFloor)
	currentWeight := input.Portfolio.FloatWeight(price)

	macroDecision := quant.ComputeMacroDecision(quant.MacroDecisionInput{
		CurrentPrice:     price,
		Timestamp:        input.Timestamp,
		Closes:           input.Closes,
		TotalEquity:      totalEquity,
		SpendableUSDT:    spendableUSDT,
		MarketState:      marketState,
		Chromosome:       params.Chromosome,
		SpawnPoint:       params.SpawnPoint,
		LastMacroBuyAt:   state.LastMacroBuyAt,
		LastExtraBuyAt:   state.LastExtraBuyAt,
		LastIdleDeployAt: state.LastIdleDeployAt,
	})

	microDecision := quant.ComputeMicroDecision(quant.MicroDecisionInput{
		Closes:        input.Closes,
		CurrentPrice:  price,
		CurrentWeight: currentWeight,
		TotalEquity:   totalEquity,
		Chromosome:    params.Chromosome,
		MarketState:   marketState,
	})

	releaseIntents := buildReleaseIntents(input.Lots, input.Timestamp, price, microDecision, params.SpawnPoint)
	microIntent := buildMicroIntent(manifest, price, microDecision, input.Lots, releaseIntents)
	macroIntent := buildMacroIntent(manifest, macroDecision)

	if macroDecision.UsedBaseDCA {
		state.LastMacroBuyAt = input.Timestamp
	}
	if macroDecision.UsedExtraBuy {
		state.LastExtraBuyAt = input.Timestamp
	}
	if macroDecision.UsedIdleBuy {
		state.LastIdleDeployAt = input.Timestamp
	}
	state.LastMarketState = marketState.State
	state.LastSignal = microDecision.Signal

	return quant.StrategyOutput{
		UpdatedRuntimeState: EncodeState(state),
		MacroIntent:         macroIntent,
		MicroIntent:         microIntent,
		ReleaseIntents:      releaseIntents,
		Diagnostics: quant.StrategyDiagnostics{
			MarketState:     marketState.State,
			IsQuiet:         marketState.IsQuiet,
			Signal:          microDecision.Signal,
			TargetWeight:    microDecision.TargetWeight,
			TheoreticalUSD:  microDecision.TheoreticalUSD,
			VolatilityRatio: microDecision.VolatilityRatio,
			MacroDeployment: macroDecision.OrderUSD,
		},
	}
}

func buildMacroIntent(manifest Manifest, decision quant.MacroDecision) *quant.OrderIntent {
	if decision.OrderUSD < quant.MinOrderUSDT {
		return nil
	}
	return &quant.OrderIntent{
		Action:     quant.ActionBuy,
		Engine:     quant.EngineMacro,
		Symbol:     manifest.Symbol,
		LotType:    quant.LotTypeDead,
		AmountUSDT: decision.OrderUSD,
		Reason:     decision.Reason,
	}
}

func buildMicroIntent(manifest Manifest, price float64, decision quant.MicroDecision, lots []quant.SpotLot, releases []quant.ReleaseIntent) *quant.OrderIntent {
	if math.Abs(decision.OrderUSD) < quant.MinOrderUSDT {
		return nil
	}

	if decision.OrderUSD > 0 {
		return &quant.OrderIntent{
			Action:     quant.ActionBuy,
			Engine:     quant.EngineMicro,
			Symbol:     manifest.Symbol,
			LotType:    quant.LotTypeFloating,
			AmountUSDT: quant.RoundToUSDT(decision.OrderUSD),
			Reason:     "sigmoid_rebalance_buy",
		}
	}

	availableFloat := quant.FloatAssetAmount(lots)
	for _, intent := range releases {
		availableFloat += intent.AmountAsset
	}
	qty := math.Abs(decision.OrderUSD) / math.Max(price, 1e-9)
	qty = math.Min(qty, availableFloat)
	if qty*price < quant.MinOrderUSDT {
		return nil
	}

	return &quant.OrderIntent{
		Action:   quant.ActionSell,
		Engine:   quant.EngineMicro,
		Symbol:   manifest.Symbol,
		LotType:  quant.LotTypeFloating,
		QtyAsset: qty,
		Reason:   "sigmoid_rebalance_sell",
	}
}

func buildReleaseIntents(lots []quant.SpotLot, timestamp int64, price float64, decision quant.MicroDecision, spawn quant.SpawnPoint) []quant.ReleaseIntent {
	if decision.OrderUSD >= 0 || price <= 0 {
		return nil
	}
	requiredQty := math.Abs(decision.OrderUSD) / price
	floatQty := quant.FloatAssetAmount(lots)
	gap := requiredQty - floatQty
	if gap <= 0 {
		return nil
	}

	now := time.UnixMilli(timestamp).UTC()
	intents := make([]quant.ReleaseIntent, 0, 2)
	_, softReleased := quant.SoftReleaseLots(lots, now, 2, spawn.Policy.MaxReleasePct, gap)
	if softReleased > 0 {
		intents = append(intents, quant.ReleaseIntent{
			Mode:        "soft",
			AmountAsset: softReleased,
			Reason:      "micro_sell_needs_more_float_soft_release",
		})
		gap -= softReleased
	}
	if gap > 0 {
		_, hardReleased := quant.HardReleaseLots(lots, now, spawn.Policy.MaxReleasePct, gap)
		if hardReleased > 0 {
			intents = append(intents, quant.ReleaseIntent{
				Mode:        "hard",
				AmountAsset: hardReleased,
				Reason:      "micro_sell_needs_more_float_hard_release",
			})
		}
	}
	return intents
}
