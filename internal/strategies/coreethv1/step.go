package coreethv1

import (
	"quantsaas/internal/adapters/backtest"
	"quantsaas/internal/quant"
	"quantsaas/internal/strategies/coretemplate"
)

func Step(input quant.StrategyInput, params Params) quant.StrategyOutput {
	return coretemplate.Step(Manifest, input, params)
}

func NewBacktestRunner(params Params) backtest.StepRunner {
	return coretemplate.NewBacktestRunner(Manifest, params)
}
