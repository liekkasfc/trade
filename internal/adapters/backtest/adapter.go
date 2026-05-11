package backtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"quantsaas/internal/quant"
)

type StepRunner func(input quant.StrategyInput) (quant.StrategyOutput, error)

type RunOptions struct {
	TemplateID         string
	Symbol             string
	Bars               []quant.Bar
	InitialCapitalUSDT float64
	InitialColdAsset   float64
	EvalStartMS        int64
	SpawnPoint         quant.SpawnPoint
	Runner             StepRunner
}

type Result struct {
	FinalPortfolio quant.PortfolioSnapshot
	RuntimeState   json.RawMessage
	Lots           []quant.SpotLot
	NAV            []float64
	MaxDrawdown    float64
	ROI            float64
	TotalInjected  float64
	FinalEquity    float64
	TradeCount     int
}

func Run(ctx context.Context, opts RunOptions) (Result, error) {
	if len(opts.Bars) == 0 {
		return Result{}, errors.New("bars are required")
	}
	if opts.Runner == nil {
		return Result{}, errors.New("runner is required")
	}
	if opts.InitialCapitalUSDT <= 0 {
		return Result{}, errors.New("initial capital must be positive")
	}

	portfolio := quant.PortfolioSnapshot{
		USDTBalance:     opts.InitialCapitalUSDT,
		ColdSealedAsset: opts.InitialColdAsset,
	}
	lots := make([]quant.SpotLot, 0, 8)
	if opts.InitialColdAsset > 0 {
		lots = quant.ApplyBuyLot(lots, quant.LotTypeColdSealed, opts.InitialColdAsset, opts.Bars[0].Close, time.UnixMilli(opts.Bars[0].OpenTime).UTC(), true)
	}

	var runtimeState json.RawMessage
	nav := make([]float64, 0, len(opts.Bars))
	flows := make([]quant.Cashflow, 0, 24)
	totalInjected := opts.InitialCapitalUSDT
	tradeCount := 0
	startMS := opts.Bars[0].OpenTime

	lastMonth := time.UnixMilli(startMS).UTC().Month()
	lastYear := time.UnixMilli(startMS).UTC().Year()

	closes := make([]float64, 0, len(opts.Bars))
	timestamps := make([]int64, 0, len(opts.Bars))

	for _, bar := range opts.Bars {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}

		ts := time.UnixMilli(bar.OpenTime).UTC()
		if len(closes) > 0 && opts.SpawnPoint.Policy.MonthlyInjectUSDT > 0 && (ts.Month() != lastMonth || ts.Year() != lastYear) {
			portfolio.USDTBalance += opts.SpawnPoint.Policy.MonthlyInjectUSDT
			totalInjected += opts.SpawnPoint.Policy.MonthlyInjectUSDT
			flows = append(flows, quant.Cashflow{
				Timestamp: bar.OpenTime,
				Amount:    opts.SpawnPoint.Policy.MonthlyInjectUSDT,
			})
		}
		lastMonth = ts.Month()
		lastYear = ts.Year()

		closes = append(closes, bar.Close)
		timestamps = append(timestamps, bar.OpenTime)
		if opts.EvalStartMS > 0 && bar.OpenTime < opts.EvalStartMS {
			continue
		}

		input := quant.StrategyInput{
			TemplateID:   opts.TemplateID,
			Symbol:       opts.Symbol,
			Timestamp:    bar.OpenTime,
			Closes:       append([]float64(nil), closes...),
			Timestamps:   append([]int64(nil), timestamps...),
			Portfolio:    portfolio,
			Lots:         quant.DeepCopyLots(lots),
			RuntimeState: runtimeState,
		}

		output, err := opts.Runner(input)
		if err != nil {
			return Result{}, fmt.Errorf("run strategy step: %w", err)
		}

		runtimeState = output.UpdatedRuntimeState
		lots = applyReleaseIntents(lots, output.ReleaseIntents, bar.OpenTime)

		if output.MacroIntent != nil {
			portfolio, lots, err = applyOrderIntent(portfolio, lots, *output.MacroIntent, bar.Close, bar.OpenTime, opts.SpawnPoint)
			if err != nil {
				return Result{}, fmt.Errorf("apply macro intent: %w", err)
			}
			tradeCount++
		}
		if output.MicroIntent != nil {
			portfolio, lots, err = applyOrderIntent(portfolio, lots, *output.MicroIntent, bar.Close, bar.OpenTime, opts.SpawnPoint)
			if err != nil {
				return Result{}, fmt.Errorf("apply micro intent: %w", err)
			}
			tradeCount++
		}

		portfolio = rebuildPortfolio(portfolio, lots, bar.Close)
		nav = append(nav, portfolio.TotalEquity(bar.Close))
	}

	if len(nav) == 0 {
		return Result{}, errors.New("no evaluation bars available")
	}

	finalEquity := nav[len(nav)-1]
	if opts.EvalStartMS > 0 {
		startMS = opts.EvalStartMS
	}
	roi := quant.ModifiedDietzROI(opts.InitialCapitalUSDT, finalEquity, flows, startMS, opts.Bars[len(opts.Bars)-1].OpenTime)

	return Result{
		FinalPortfolio: portfolio,
		RuntimeState:   runtimeState,
		Lots:           lots,
		NAV:            nav,
		MaxDrawdown:    quant.MaxDrawdown(nav),
		ROI:            roi,
		TotalInjected:  totalInjected,
		FinalEquity:    finalEquity,
		TradeCount:     tradeCount,
	}, nil
}

func applyReleaseIntents(lots []quant.SpotLot, intents []quant.ReleaseIntent, openTime int64) []quant.SpotLot {
	now := time.UnixMilli(openTime).UTC()
	for _, intent := range intents {
		switch intent.Mode {
		case "soft":
			lots, _ = quant.SoftReleaseLots(lots, now, 0, 1.0, intent.AmountAsset)
		case "hard":
			lots, _ = quant.HardReleaseLots(lots, now, 1.0, intent.AmountAsset)
		}
	}
	return lots
}

func applyOrderIntent(portfolio quant.PortfolioSnapshot, lots []quant.SpotLot, intent quant.OrderIntent, closePrice float64, openTime int64, spawn quant.SpawnPoint) (quant.PortfolioSnapshot, []quant.SpotLot, error) {
	now := time.UnixMilli(openTime).UTC()
	feeRate := spawn.Risk.TakerFeeBps / 10000.0
	slippage := spawn.Risk.SlippageBps / 10000.0

	switch intent.Action {
	case quant.ActionBuy:
		orderUSD := math.Min(portfolio.USDTBalance, intent.AmountUSDT)
		if orderUSD <= 0 {
			return portfolio, lots, nil
		}
		effectivePrice := closePrice * (1 + slippage)
		qty := (orderUSD / effectivePrice) * (1 - feeRate)
		portfolio.USDTBalance -= orderUSD
		lots = quant.ApplyBuyLot(lots, intent.LotType, qty, effectivePrice, now, intent.LotType == quant.LotTypeColdSealed)
		return rebuildPortfolio(portfolio, lots, closePrice), lots, nil

	case quant.ActionSell:
		if intent.QtyAsset <= 0 {
			return portfolio, lots, nil
		}
		effectivePrice := closePrice * (1 - slippage)
		var err error
		lots, err = quant.SellFromFloatingLots(lots, intent.QtyAsset)
		if err != nil {
			return portfolio, lots, err
		}
		received := intent.QtyAsset * effectivePrice * (1 - feeRate)
		portfolio.USDTBalance += received
		return rebuildPortfolio(portfolio, lots, closePrice), lots, nil
	default:
		return portfolio, lots, fmt.Errorf("unsupported action %q", intent.Action)
	}
}

func rebuildPortfolio(portfolio quant.PortfolioSnapshot, lots []quant.SpotLot, price float64) quant.PortfolioSnapshot {
	var dead, floating, cold float64
	for _, lot := range lots {
		switch lot.LotType {
		case quant.LotTypeDead:
			if lot.IsColdSealed {
				cold += lot.Amount
			} else {
				dead += lot.Amount
			}
		case quant.LotTypeFloating:
			floating += lot.Amount
		case quant.LotTypeColdSealed:
			cold += lot.Amount
		}
	}

	portfolio.DeadAsset = dead
	portfolio.FloatAsset = floating
	portfolio.ColdSealedAsset = cold
	portfolio.LastProcessedBarTime = 0
	return portfolio
}
