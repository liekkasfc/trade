package strategies

import (
	"encoding/json"
	"fmt"

	"quantsaas/internal/adapters/backtest"
	"quantsaas/internal/quant"
	"quantsaas/internal/strategies/corebtcv1"
	"quantsaas/internal/strategies/coreethv1"
	"quantsaas/internal/strategies/coretemplate"
)

type Spec struct {
	Manifest          coretemplate.Manifest
	DefaultParams     func() coretemplate.Params
	ParseParamPack    func(json.RawMessage) (coretemplate.Params, error)
	EncodeParamPack   func(coretemplate.Params) (json.RawMessage, error)
	NewBacktestRunner func(coretemplate.Params) backtest.StepRunner
}

func Catalog() []Spec {
	return []Spec{
		{
			Manifest:          corebtcv1.Manifest,
			DefaultParams:     corebtcv1.DefaultParams,
			ParseParamPack:    corebtcv1.ParseParamPack,
			EncodeParamPack:   corebtcv1.EncodeParamPack,
			NewBacktestRunner: corebtcv1.NewBacktestRunner,
		},
		{
			Manifest:          coreethv1.Manifest,
			DefaultParams:     coreethv1.DefaultParams,
			ParseParamPack:    coreethv1.ParseParamPack,
			EncodeParamPack:   coreethv1.EncodeParamPack,
			NewBacktestRunner: coreethv1.NewBacktestRunner,
		},
	}
}

func Lookup(templateID string) (Spec, error) {
	for _, spec := range Catalog() {
		if spec.Manifest.ID == templateID {
			return spec, nil
		}
	}
	return Spec{}, fmt.Errorf("unknown template_id %q", templateID)
}

func (s Spec) DefaultChromosome() quant.Chromosome {
	return s.DefaultParams().Chromosome
}
