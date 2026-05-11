package store

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	UserRoleUser  = "user"
	UserRoleLab   = "lab"
	UserRoleAdmin = "admin"

	InstanceStatusStopped = "STOPPED"
	InstanceStatusRunning = "RUNNING"
	InstanceStatusError   = "ERROR"
	InstanceStatusDeleted = "DELETED"

	LotTypeDead       = "DEAD_STACK"
	LotTypeFloating   = "FLOATING"
	LotTypeColdSealed = "COLD_SEALED"

	GeneRoleChallenger = "challenger"
	GeneRoleChampion   = "champion"
	GeneRoleRetired    = "retired"
)

type User struct {
	gorm.Model
	Email        string `gorm:"size:255;uniqueIndex;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	Role         string `gorm:"size:32;not null;default:user"`
	Plan         string `gorm:"size:64;not null;default:core"`
}

type StrategyTemplate struct {
	gorm.Model
	TemplateKey string         `gorm:"size:128;uniqueIndex;not null"`
	Name        string         `gorm:"size:255;not null"`
	Version     string         `gorm:"size:64;not null"`
	IsSpot      bool           `gorm:"not null;default:true"`
	Manifest    datatypes.JSON `gorm:"type:jsonb"`
}

type StrategyInstance struct {
	gorm.Model
	UserID             uint    `gorm:"not null;index"`
	TemplateID         uint    `gorm:"not null;index"`
	Name               string  `gorm:"size:255;not null"`
	Symbol             string  `gorm:"size:64;not null;index"`
	Status             string  `gorm:"size:32;not null;default:STOPPED"`
	CapitalQuotaUSDT   float64 `gorm:"not null;default:0"`
	MonthlyInjectUSDT  float64 `gorm:"not null;default:0"`
	ColdSealedAssetQty float64 `gorm:"not null;default:0"`
	MaxDrawdownPct     float64 `gorm:"not null;default:0"`

	User     User             `gorm:"constraint:OnDelete:CASCADE"`
	Template StrategyTemplate `gorm:"constraint:OnDelete:RESTRICT"`
}

type PortfolioState struct {
	gorm.Model
	StrategyInstanceID   uint    `gorm:"uniqueIndex;not null"`
	USDTBalance          float64 `gorm:"not null;default:0"`
	DeadBTC              float64 `gorm:"not null;default:0"`
	FloatBTC             float64 `gorm:"not null;default:0"`
	ColdSealedBTC        float64 `gorm:"not null;default:0"`
	TotalEquity          float64 `gorm:"not null;default:0"`
	LastProcessedBarTime int64   `gorm:"not null;default:0"`
	LastSyncedAt         *time.Time
}

type RuntimeState struct {
	gorm.Model
	StrategyInstanceID uint           `gorm:"uniqueIndex;not null"`
	StateBlob          datatypes.JSON `gorm:"type:jsonb"`
}

type SpotLot struct {
	gorm.Model
	StrategyInstanceID uint    `gorm:"not null;index"`
	LotType            string  `gorm:"size:32;not null"`
	Amount             float64 `gorm:"not null;default:0"`
	CostPrice          float64 `gorm:"not null;default:0"`
	IsColdSealed       bool    `gorm:"not null;default:false"`
}

type TradeRecord struct {
	gorm.Model
	StrategyInstanceID uint    `gorm:"not null;index"`
	ClientOrderID      string  `gorm:"size:128;not null;index"`
	Action             string  `gorm:"size:16;not null"`
	Engine             string  `gorm:"size:16;not null"`
	Symbol             string  `gorm:"size:64;not null"`
	FilledQty          float64 `gorm:"not null;default:0"`
	FilledPrice        float64 `gorm:"not null;default:0"`
	Fee                float64 `gorm:"not null;default:0"`
}

type SpotExecution struct {
	gorm.Model
	StrategyInstanceID uint           `gorm:"not null;index"`
	ClientOrderID      string         `gorm:"size:128;uniqueIndex;not null"`
	Action             string         `gorm:"size:16;not null"`
	Engine             string         `gorm:"size:16;not null"`
	Symbol             string         `gorm:"size:64;not null"`
	AmountUSDT         float64        `gorm:"not null;default:0"`
	QtyAsset           float64        `gorm:"not null;default:0"`
	LotType            string         `gorm:"size:32;not null"`
	Status             string         `gorm:"size:32;not null;default:pending"`
	RawExecution       datatypes.JSON `gorm:"type:jsonb"`
}

type AuditLog struct {
	gorm.Model
	UserID             *uint          `gorm:"index"`
	StrategyInstanceID *uint          `gorm:"index"`
	EventType          string         `gorm:"size:128;not null;index"`
	Payload            datatypes.JSON `gorm:"type:jsonb"`
}

type GeneRecord struct {
	gorm.Model
	StrategyID   string         `gorm:"size:128;not null;index"`
	Symbol       string         `gorm:"size:64;not null;index"`
	Role         string         `gorm:"size:32;not null;index"`
	ParamPack    datatypes.JSON `gorm:"type:jsonb;not null"`
	ScoreTotal   float64        `gorm:"not null;default:0"`
	MaxDrawdown  float64        `gorm:"not null;default:0"`
	WindowScores datatypes.JSON `gorm:"type:jsonb"`
}

type EvolutionTask struct {
	gorm.Model
	StrategyID   string         `gorm:"size:128;not null;index"`
	Symbol       string         `gorm:"size:64;not null;index"`
	Status       string         `gorm:"size:32;not null;index"`
	ProgressJSON datatypes.JSON `gorm:"type:jsonb"`
	ConfigJSON   datatypes.JSON `gorm:"type:jsonb"`
	ResultGeneID *uint          `gorm:"index"`
	ErrorText    string         `gorm:"type:text"`
}

type BacktestTask struct {
	gorm.Model
	StrategyID  string         `gorm:"size:128;not null;index"`
	Symbol      string         `gorm:"size:64;not null;index"`
	Status      string         `gorm:"size:32;not null;index"`
	RequestJSON datatypes.JSON `gorm:"type:jsonb"`
	ResultJSON  datatypes.JSON `gorm:"type:jsonb"`
	ErrorText   string         `gorm:"type:text"`
}

type EquitySnapshot struct {
	gorm.Model
	StrategyInstanceID uint    `gorm:"not null;uniqueIndex:idx_equity_snapshot_instance_time"`
	SnapshotTime       int64   `gorm:"not null;uniqueIndex:idx_equity_snapshot_instance_time;index"`
	Source             string  `gorm:"size:32;not null;default:tick"`
	USDTBalance        float64 `gorm:"not null;default:0"`
	DeadAssetQty       float64 `gorm:"not null;default:0"`
	FloatAssetQty      float64 `gorm:"not null;default:0"`
	ColdSealedAssetQty float64 `gorm:"not null;default:0"`
	TotalEquity        float64 `gorm:"not null;default:0"`
	MarkPrice          float64 `gorm:"not null;default:0"`
}

type KLine struct {
	gorm.Model
	Symbol   string  `gorm:"size:64;not null;uniqueIndex:idx_kline_symbol_interval_open_time"`
	Interval string  `gorm:"size:16;not null;uniqueIndex:idx_kline_symbol_interval_open_time"`
	OpenTime int64   `gorm:"not null;uniqueIndex:idx_kline_symbol_interval_open_time"`
	Open     float64 `gorm:"not null"`
	High     float64 `gorm:"not null"`
	Low      float64 `gorm:"not null"`
	Close    float64 `gorm:"not null"`
	Volume   float64 `gorm:"not null"`
}

func AllModels() []any {
	return []any{
		&User{},
		&StrategyTemplate{},
		&StrategyInstance{},
		&PortfolioState{},
		&RuntimeState{},
		&SpotLot{},
		&TradeRecord{},
		&SpotExecution{},
		&AuditLog{},
		&GeneRecord{},
		&EvolutionTask{},
		&BacktestTask{},
		&EquitySnapshot{},
		&KLine{},
	}
}
