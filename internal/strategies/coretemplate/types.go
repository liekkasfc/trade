package coretemplate

import (
	"encoding/json"
	"errors"

	"quantsaas/internal/adapters/backtest"
	"quantsaas/internal/quant"
)

type Manifest struct {
	ID          string
	Name        string
	Version     string
	Symbol      string
	Description string
	IsSpot      bool
}

type Params struct {
	Chromosome quant.Chromosome `json:"chromosome"`
	SpawnPoint quant.SpawnPoint `json:"spawn_point"`
}

type RuntimeState struct {
	LastProcessedBarTime int64   `json:"last_processed_bar_time"`
	LastMacroBuyAt       int64   `json:"last_macro_buy_at"`
	LastExtraBuyAt       int64   `json:"last_extra_buy_at"`
	LastIdleDeployAt     int64   `json:"last_idle_deploy_at"`
	LastMarketState      string  `json:"last_market_state"`
	LastSignal           float64 `json:"last_signal"`
}

type ParamPack struct {
	Chromosome quant.Chromosome `json:"chromosome"`
	SpawnPoint quant.SpawnPoint `json:"spawn_point"`
}

func DefaultParams(seed quant.Chromosome) Params {
	return Params{
		Chromosome: quant.ClampChromosome(seed),
		SpawnPoint: quant.DefaultSpawnPoint,
	}
}

func ParseParamPack(raw json.RawMessage, defaults Params) (Params, error) {
	if len(raw) == 0 {
		return defaults, nil
	}

	var pack ParamPack
	if err := json.Unmarshal(raw, &pack); err != nil {
		return defaults, err
	}

	pack.Chromosome = quant.ClampChromosome(pack.Chromosome)
	if pack.SpawnPoint == (quant.SpawnPoint{}) {
		pack.SpawnPoint = defaults.SpawnPoint
	}

	return Params{
		Chromosome: pack.Chromosome,
		SpawnPoint: pack.SpawnPoint,
	}, nil
}

func EncodeParamPack(params Params) (json.RawMessage, error) {
	raw, err := json.Marshal(ParamPack{
		Chromosome: params.Chromosome,
		SpawnPoint: params.SpawnPoint,
	})
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func DecodeState(raw json.RawMessage) RuntimeState {
	if len(raw) == 0 {
		return RuntimeState{}
	}
	var state RuntimeState
	if err := json.Unmarshal(raw, &state); err != nil {
		return RuntimeState{}
	}
	return state
}

func EncodeState(state RuntimeState) json.RawMessage {
	raw, _ := json.Marshal(state)
	return raw
}

func NewBacktestRunner(manifest Manifest, params Params) backtest.StepRunner {
	return func(input quant.StrategyInput) (quant.StrategyOutput, error) {
		return Step(manifest, input, params), nil
	}
}

func ValidateManifest(manifest Manifest) error {
	if manifest.ID == "" || manifest.Symbol == "" {
		return errors.New("manifest id and symbol are required")
	}
	return nil
}
