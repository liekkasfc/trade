package quant

import (
	"math/rand"
)

type Bound struct {
	Min  float64
	Max  float64
	Step float64
}

type Chromosome struct {
	SignalWeightMeanRev       float64 `json:"signal_weight_mean_rev"`
	SignalWeightMomentum      float64 `json:"signal_weight_momentum"`
	SigmaFloor                float64 `json:"sigma_floor"`
	Beta                      float64 `json:"beta"`
	Gamma                     float64 `json:"gamma"`
	DiscountBuyThreshold      float64 `json:"discount_buy_threshold"`
	DiscountBuyScale          float64 `json:"discount_buy_scale"`
	QuietVolRatioThreshold    float64 `json:"quiet_vol_ratio_threshold"`
	MicroDeltaWeightThreshold float64 `json:"micro_delta_weight_threshold"`
	MicroReservePct           float64 `json:"micro_reserve_pct"`
}

type SpawnPolicy struct {
	MonthlyInjectUSDT      float64 `json:"monthly_inject_usdt"`
	IdleDeployDeadlineDays int     `json:"idle_deploy_deadline_days"`
	ExtraBuyCapPct         float64 `json:"extra_buy_cap_pct"`
	BearExtraBuyCapPct     float64 `json:"bear_extra_buy_cap_pct"`
	MaxReleasePct          float64 `json:"max_release_pct"`
}

type SpawnRisk struct {
	TakerFeeBps         float64 `json:"taker_fee_bps"`
	SlippageBps         float64 `json:"slippage_bps"`
	GlobalDrawdownGuard float64 `json:"global_drawdown_guard"`
}

type SpawnPoint struct {
	Policy SpawnPolicy `json:"policy"`
	Risk   SpawnRisk   `json:"risk"`
}

var HardBounds = map[string]Bound{
	"signal_weight_mean_rev":       {Min: -3.0, Max: 3.0, Step: 0.05},
	"signal_weight_momentum":       {Min: -3.0, Max: 3.0, Step: 0.05},
	"sigma_floor":                  {Min: 0.001, Max: 0.08, Step: 0.001},
	"beta":                         {Min: 0.1, Max: 4.0, Step: 0.05},
	"gamma":                        {Min: 0.0, Max: 4.0, Step: 0.05},
	"discount_buy_threshold":       {Min: 0.005, Max: 0.20, Step: 0.0025},
	"discount_buy_scale":           {Min: 0.1, Max: 2.0, Step: 0.05},
	"quiet_vol_ratio_threshold":    {Min: 0.5, Max: 2.5, Step: 0.05},
	"micro_delta_weight_threshold": {Min: 0.0025, Max: 0.15, Step: 0.0025},
	"micro_reserve_pct":            {Min: 0.05, Max: 0.60, Step: 0.01},
}

var DefaultSeedChromosome = Chromosome{
	SignalWeightMeanRev:       0.85,
	SignalWeightMomentum:      -0.25,
	SigmaFloor:                0.006,
	Beta:                      1.15,
	Gamma:                     0.70,
	DiscountBuyThreshold:      0.035,
	DiscountBuyScale:          0.80,
	QuietVolRatioThreshold:    0.95,
	MicroDeltaWeightThreshold: 0.015,
	MicroReservePct:           0.25,
}

var DefaultSpawnPoint = SpawnPoint{
	Policy: SpawnPolicy{
		MonthlyInjectUSDT:      500,
		IdleDeployDeadlineDays: 21,
		ExtraBuyCapPct:         0.20,
		BearExtraBuyCapPct:     0.10,
		MaxReleasePct:          0.40,
	},
	Risk: SpawnRisk{
		TakerFeeBps:         10,
		SlippageBps:         8,
		GlobalDrawdownGuard: 0.65,
	},
}

func ClampChromosome(c Chromosome) Chromosome {
	c.SignalWeightMeanRev = ClipFloat64(c.SignalWeightMeanRev, HardBounds["signal_weight_mean_rev"].Min, HardBounds["signal_weight_mean_rev"].Max)
	c.SignalWeightMomentum = ClipFloat64(c.SignalWeightMomentum, HardBounds["signal_weight_momentum"].Min, HardBounds["signal_weight_momentum"].Max)
	c.SigmaFloor = ClipFloat64(c.SigmaFloor, HardBounds["sigma_floor"].Min, HardBounds["sigma_floor"].Max)
	c.Beta = ClipFloat64(c.Beta, HardBounds["beta"].Min, HardBounds["beta"].Max)
	c.Gamma = ClipFloat64(c.Gamma, HardBounds["gamma"].Min, HardBounds["gamma"].Max)
	c.DiscountBuyThreshold = ClipFloat64(c.DiscountBuyThreshold, HardBounds["discount_buy_threshold"].Min, HardBounds["discount_buy_threshold"].Max)
	c.DiscountBuyScale = ClipFloat64(c.DiscountBuyScale, HardBounds["discount_buy_scale"].Min, HardBounds["discount_buy_scale"].Max)
	c.QuietVolRatioThreshold = ClipFloat64(c.QuietVolRatioThreshold, HardBounds["quiet_vol_ratio_threshold"].Min, HardBounds["quiet_vol_ratio_threshold"].Max)
	c.MicroDeltaWeightThreshold = ClipFloat64(c.MicroDeltaWeightThreshold, HardBounds["micro_delta_weight_threshold"].Min, HardBounds["micro_delta_weight_threshold"].Max)
	c.MicroReservePct = ClipFloat64(c.MicroReservePct, HardBounds["micro_reserve_pct"].Min, HardBounds["micro_reserve_pct"].Max)
	return c
}

func SampleChromosome(rng *rand.Rand) Chromosome {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	pick := func(key string) float64 {
		b := HardBounds[key]
		return b.Min + rng.Float64()*(b.Max-b.Min)
	}
	return ClampChromosome(Chromosome{
		SignalWeightMeanRev:       pick("signal_weight_mean_rev"),
		SignalWeightMomentum:      pick("signal_weight_momentum"),
		SigmaFloor:                pick("sigma_floor"),
		Beta:                      pick("beta"),
		Gamma:                     pick("gamma"),
		DiscountBuyThreshold:      pick("discount_buy_threshold"),
		DiscountBuyScale:          pick("discount_buy_scale"),
		QuietVolRatioThreshold:    pick("quiet_vol_ratio_threshold"),
		MicroDeltaWeightThreshold: pick("micro_delta_weight_threshold"),
		MicroReservePct:           pick("micro_reserve_pct"),
	})
}

func RandomSpawnPoint(rng *rand.Rand) SpawnPoint {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	return SpawnPoint{
		Policy: SpawnPolicy{
			MonthlyInjectUSDT:      200 + rng.Float64()*1800,
			IdleDeployDeadlineDays: 7 + rng.Intn(30),
			ExtraBuyCapPct:         0.10 + rng.Float64()*0.25,
			BearExtraBuyCapPct:     0.05 + rng.Float64()*0.15,
			MaxReleasePct:          0.15 + rng.Float64()*0.45,
		},
		Risk: SpawnRisk{
			TakerFeeBps:         8 + rng.Float64()*8,
			SlippageBps:         4 + rng.Float64()*10,
			GlobalDrawdownGuard: 0.50 + rng.Float64()*0.20,
		},
	}
}
