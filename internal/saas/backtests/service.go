package backtests

import (
	"context"
	"encoding/json"
	"fmt"

	"quantsaas/internal/adapters/backtest"
	"quantsaas/internal/quant"
	"quantsaas/internal/saas/store"
	"quantsaas/internal/strategies"
	"quantsaas/internal/strategies/coretemplate"

	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	BacktestStatusRunning   = "running"
	BacktestStatusCompleted = "completed"
	BacktestStatusFailed    = "failed"
)

type CreateRequest struct {
	TemplateID         string          `json:"template_id"`
	GeneID             *uint           `json:"gene_id,omitempty"`
	ParamPack          json.RawMessage `json:"param_pack,omitempty"`
	InitialCapitalUSDT float64         `json:"initial_capital_usdt,omitempty"`
	InitialColdAsset   float64         `json:"initial_cold_asset,omitempty"`
}

type ResultPayload struct {
	FinalEquity   float64   `json:"final_equity"`
	TotalInjected float64   `json:"total_injected"`
	MaxDrawdown   float64   `json:"max_drawdown"`
	ROI           float64   `json:"roi"`
	TradeCount    int       `json:"trade_count"`
	NAV           []float64 `json:"nav"`
}

type Service struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewService(db *gorm.DB, logger *zap.Logger) *Service {
	return &Service{db: db, logger: logger}
}

func (s *Service) CreateAndRun(ctx context.Context, req CreateRequest) (*store.BacktestTask, error) {
	spec, err := strategies.Lookup(req.TemplateID)
	if err != nil {
		return nil, err
	}

	task := &store.BacktestTask{
		StrategyID: req.TemplateID,
		Symbol:     spec.Manifest.Symbol,
		Status:     BacktestStatusRunning,
	}
	requestJSON, _ := json.Marshal(req)
	task.RequestJSON = datatypes.JSON(requestJSON)
	if err := s.db.WithContext(ctx).Create(task).Error; err != nil {
		return nil, err
	}

	go s.run(task.ID, req)
	return task, nil
}

func (s *Service) Get(ctx context.Context, id uint) (*store.BacktestTask, error) {
	var task store.BacktestTask
	if err := s.db.WithContext(ctx).First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *Service) run(taskID uint, req CreateRequest) {
	ctx := context.Background()
	spec, err := strategies.Lookup(req.TemplateID)
	if err != nil {
		s.failTask(ctx, taskID, err)
		return
	}

	params, err := s.resolveParams(ctx, spec, req)
	if err != nil {
		s.failTask(ctx, taskID, err)
		return
	}

	bars, err := s.loadBars(ctx, spec.Manifest.Symbol)
	if err != nil {
		s.failTask(ctx, taskID, err)
		return
	}

	initialCapital := req.InitialCapitalUSDT
	if initialCapital <= 0 {
		initialCapital = 10000
	}

	result, err := backtest.Run(ctx, backtest.RunOptions{
		TemplateID:         spec.Manifest.ID,
		Symbol:             spec.Manifest.Symbol,
		Bars:               bars,
		InitialCapitalUSDT: initialCapital,
		InitialColdAsset:   req.InitialColdAsset,
		SpawnPoint:         params.SpawnPoint,
		Runner:             spec.NewBacktestRunner(params),
	})
	if err != nil {
		s.failTask(ctx, taskID, err)
		return
	}

	payload, _ := json.Marshal(ResultPayload{
		FinalEquity:   result.FinalEquity,
		TotalInjected: result.TotalInjected,
		MaxDrawdown:   result.MaxDrawdown,
		ROI:           result.ROI,
		TradeCount:    result.TradeCount,
		NAV:           result.NAV,
	})
	if err := s.db.WithContext(ctx).Model(&store.BacktestTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status":      BacktestStatusCompleted,
		"result_json": datatypes.JSON(payload),
		"error_text":  "",
	}).Error; err != nil {
		s.logger.Error("update completed backtest task", zap.Uint("task_id", taskID), zap.Error(err))
	}
}

func (s *Service) resolveParams(ctx context.Context, spec strategies.Spec, req CreateRequest) (coretemplate.Params, error) {
	if len(req.ParamPack) > 0 {
		return spec.ParseParamPack(req.ParamPack)
	}
	if req.GeneID != nil {
		var gene store.GeneRecord
		if err := s.db.WithContext(ctx).First(&gene, *req.GeneID).Error; err != nil {
			return coretemplate.Params{}, err
		}
		return spec.ParseParamPack(json.RawMessage(gene.ParamPack))
	}

	var champion store.GeneRecord
	if err := s.db.WithContext(ctx).
		Where("strategy_id = ? AND role = ?", spec.Manifest.ID, store.GeneRoleChampion).
		Order("created_at DESC").
		Take(&champion).Error; err == nil {
		return spec.ParseParamPack(json.RawMessage(champion.ParamPack))
	}
	return spec.DefaultParams(), nil
}

func (s *Service) loadBars(ctx context.Context, symbol string) ([]quant.Bar, error) {
	var rows []store.KLine
	if err := s.db.WithContext(ctx).
		Where("symbol = ? AND interval = ?", symbol, "1h").
		Order("open_time ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no 1h bars found for %s", symbol)
	}
	bars := make([]quant.Bar, 0, len(rows))
	for _, row := range rows {
		bars = append(bars, quant.Bar{
			OpenTime: row.OpenTime,
			Open:     row.Open,
			High:     row.High,
			Low:      row.Low,
			Close:    row.Close,
			Volume:   row.Volume,
		})
	}
	return bars, nil
}

func (s *Service) failTask(ctx context.Context, taskID uint, err error) {
	s.logger.Error("backtest task failed", zap.Uint("task_id", taskID), zap.Error(err))
	_ = s.db.WithContext(ctx).Model(&store.BacktestTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status":     BacktestStatusFailed,
		"error_text": err.Error(),
	}).Error
}
