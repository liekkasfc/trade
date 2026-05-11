package corebtcv1

import (
	"encoding/json"

	"quantsaas/internal/quant"
	"quantsaas/internal/strategies/coretemplate"
)

type Params = coretemplate.Params

func DefaultParams() Params {
	seed := quant.DefaultSeedChromosome
	seed.SignalWeightMomentum = -0.20
	seed.Beta = 1.10
	return coretemplate.DefaultParams(seed)
}

func ParseParamPack(raw json.RawMessage) (Params, error) {
	return coretemplate.ParseParamPack(raw, DefaultParams())
}

func EncodeParamPack(params Params) (json.RawMessage, error) {
	return coretemplate.EncodeParamPack(params)
}
