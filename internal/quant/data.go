package quant

import (
	"encoding/json"
	"math"
	"time"
)

const (
	ActionBuy  = "BUY"
	ActionSell = "SELL"

	EngineMacro = "MACRO"
	EngineMicro = "MICRO"

	LotTypeDead       = "DEAD_STACK"
	LotTypeFloating   = "FLOATING"
	LotTypeColdSealed = "COLD_SEALED"
)

type Bar struct {
	OpenTime int64
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   float64
}

type PortfolioSnapshot struct {
	USDTBalance          float64
	DeadAsset            float64
	FloatAsset           float64
	ColdSealedAsset      float64
	LastProcessedBarTime int64
}

func (p PortfolioSnapshot) TotalAsset() float64 {
	return p.DeadAsset + p.FloatAsset + p.ColdSealedAsset
}

func (p PortfolioSnapshot) TotalEquity(price float64) float64 {
	return p.USDTBalance + p.TotalAsset()*price
}

func (p PortfolioSnapshot) FloatWeight(price float64) float64 {
	totalEquity := p.TotalEquity(price)
	if totalEquity <= 0 {
		return 0
	}
	return (p.FloatAsset * price) / totalEquity
}

type StrategyInput struct {
	TemplateID   string
	Symbol       string
	Timestamp    int64
	Closes       []float64
	Timestamps   []int64
	Portfolio    PortfolioSnapshot
	Lots         []SpotLot
	RuntimeState json.RawMessage
}

type OrderIntent struct {
	Action     string  `json:"action"`
	Engine     string  `json:"engine"`
	Symbol     string  `json:"symbol"`
	LotType    string  `json:"lot_type"`
	AmountUSDT float64 `json:"amount_usdt,omitempty"`
	QtyAsset   float64 `json:"qty_asset,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

type ReleaseIntent struct {
	Mode        string  `json:"mode"`
	AmountAsset float64 `json:"amount_asset"`
	Reason      string  `json:"reason"`
}

type StrategyDiagnostics struct {
	MarketState     string   `json:"market_state"`
	IsQuiet         bool     `json:"is_quiet"`
	Signal          float64  `json:"signal"`
	TargetWeight    float64  `json:"target_weight"`
	TheoreticalUSD  float64  `json:"theoretical_usd"`
	VolatilityRatio float64  `json:"volatility_ratio"`
	MacroDeployment float64  `json:"macro_deployment"`
	Notes           []string `json:"notes,omitempty"`
}

type StrategyOutput struct {
	UpdatedRuntimeState json.RawMessage     `json:"updated_runtime_state"`
	MacroIntent         *OrderIntent        `json:"macro_intent,omitempty"`
	MicroIntent         *OrderIntent        `json:"micro_intent,omitempty"`
	ReleaseIntents      []ReleaseIntent     `json:"release_intents,omitempty"`
	Diagnostics         StrategyDiagnostics `json:"diagnostics"`
}

func (o StrategyOutput) Empty() bool {
	return o.MacroIntent == nil && o.MicroIntent == nil && len(o.ReleaseIntents) == 0
}

type SpotLot struct {
	LotType      string    `json:"lot_type"`
	Amount       float64   `json:"amount"`
	CostPrice    float64   `json:"cost_price"`
	CreatedAt    time.Time `json:"created_at"`
	IsColdSealed bool      `json:"is_cold_sealed"`
}

type BacktestMetrics struct {
	FinalEquity   float64
	TotalInjected float64
	MaxDrawdown   float64
	ROI           float64
	TradeCount    int
	NAV           []float64
}

func SafeLogReturn(current, previous float64) float64 {
	if current <= 0 || previous <= 0 {
		return 0
	}
	return math.Log(current / previous)
}
