package instance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"quantsaas/internal/protocol"
	"quantsaas/internal/quant"
	"quantsaas/internal/saas/dashboard"
	"quantsaas/internal/saas/ledger"
	"quantsaas/internal/saas/marketdata"
	"quantsaas/internal/saas/store"
	"quantsaas/internal/strategies"
	"quantsaas/internal/strategies/coretemplate"

	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const championCacheTTL = 24 * time.Hour

type CommandHub interface {
	SendToAgent(userID uint, cmd protocol.TradeCommand) error
	IsConnected(userID uint) bool
	StatusForUser(userID uint) protocol.AgentStatus
}

type CreateRequest struct {
	TemplateID         string  `json:"template_id"`
	Name               string  `json:"name"`
	CapitalQuotaUSDT   float64 `json:"capital_quota_usdt"`
	MonthlyInjectUSDT  float64 `json:"monthly_inject_usdt"`
	ColdSealedAssetQty float64 `json:"cold_sealed_asset_qty"`
	MaxDrawdownPct     float64 `json:"max_drawdown_pct"`
}

type Manager struct {
	db         *gorm.DB
	cache      *store.Cache
	hub        CommandHub
	marketData *marketdata.Service
	logger     *zap.Logger

	active sync.Map
}

func NewManager(db *gorm.DB, cache *store.Cache, hub CommandHub, marketData *marketdata.Service, logger *zap.Logger) *Manager {
	return &Manager{
		db:         db,
		cache:      cache,
		hub:        hub,
		marketData: marketData,
		logger:     logger,
	}
}

func (m *Manager) ListStrategies(ctx context.Context) ([]store.StrategyTemplate, error) {
	var templates []store.StrategyTemplate
	if err := m.db.WithContext(ctx).Order("template_key ASC").Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

func (m *Manager) GetStrategy(ctx context.Context, templateKey string) (*store.StrategyTemplate, error) {
	var template store.StrategyTemplate
	if err := m.db.WithContext(ctx).Where("template_key = ?", templateKey).Take(&template).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

func (m *Manager) ListInstances(ctx context.Context, userID uint) ([]store.StrategyInstance, error) {
	var instances []store.StrategyInstance
	if err := m.db.WithContext(ctx).
		Preload("Template").
		Where("user_id = ? AND status <> ?", userID, store.InstanceStatusDeleted).
		Order("created_at DESC").
		Find(&instances).Error; err != nil {
		return nil, err
	}
	return instances, nil
}

func (m *Manager) Create(ctx context.Context, userID uint, req CreateRequest) (*store.StrategyInstance, error) {
	spec, err := strategies.Lookup(req.TemplateID)
	if err != nil {
		return nil, err
	}
	if req.CapitalQuotaUSDT < 0 || req.MonthlyInjectUSDT < 0 || req.ColdSealedAssetQty < 0 {
		return nil, errors.New("instance amounts must be non-negative")
	}

	var template store.StrategyTemplate
	if err := m.db.WithContext(ctx).Where("template_key = ?", spec.Manifest.ID).Take(&template).Error; err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = spec.Manifest.Name
	}

	now := time.Now().UTC()
	latestClose := m.marketData.LatestClose(ctx, spec.Manifest.Symbol)

	instance := &store.StrategyInstance{
		UserID:             userID,
		TemplateID:         template.ID,
		Name:               name,
		Symbol:             spec.Manifest.Symbol,
		Status:             store.InstanceStatusStopped,
		CapitalQuotaUSDT:   req.CapitalQuotaUSDT,
		MonthlyInjectUSDT:  req.MonthlyInjectUSDT,
		ColdSealedAssetQty: req.ColdSealedAssetQty,
		MaxDrawdownPct:     req.MaxDrawdownPct,
	}

	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(instance).Error; err != nil {
			return err
		}

		portfolio := store.PortfolioState{
			StrategyInstanceID: instance.ID,
			USDTBalance:        req.CapitalQuotaUSDT,
			ColdSealedBTC:      req.ColdSealedAssetQty,
			TotalEquity:        req.CapitalQuotaUSDT + req.ColdSealedAssetQty*latestClose,
		}
		if err := tx.Create(&portfolio).Error; err != nil {
			return err
		}

		runtime := store.RuntimeState{
			StrategyInstanceID: instance.ID,
			StateBlob:          datatypes.JSON(coretemplate.EncodeState(coretemplate.RuntimeState{})),
		}
		if err := tx.Create(&runtime).Error; err != nil {
			return err
		}

		if req.ColdSealedAssetQty > 0 {
			lot := store.SpotLot{
				StrategyInstanceID: instance.ID,
				LotType:            store.LotTypeColdSealed,
				Amount:             req.ColdSealedAssetQty,
				CostPrice:          latestClose,
				IsColdSealed:       true,
				Model: gorm.Model{
					CreatedAt: now,
					UpdatedAt: now,
				},
			}
			if err := tx.Create(&lot).Error; err != nil {
				return err
			}
		}

		if err := dashboard.RecordEquitySnapshot(ctx, tx, dashboard.SnapshotRecord{
			StrategyInstanceID: instance.ID,
			SnapshotTime:       now.UnixMilli(),
			Source:             dashboard.SnapshotSourceCreate,
			USDTBalance:        portfolio.USDTBalance,
			DeadAssetQty:       portfolio.DeadBTC,
			FloatAssetQty:      portfolio.FloatBTC,
			ColdSealedAssetQty: portfolio.ColdSealedBTC,
			TotalEquity:        portfolio.TotalEquity,
			MarkPrice:          latestClose,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := m.db.WithContext(ctx).Preload("Template").First(instance, instance.ID).Error; err != nil {
		return nil, err
	}
	return instance, nil
}

func (m *Manager) Start(ctx context.Context, userID, instanceID uint) error {
	return m.transitionStatus(ctx, userID, instanceID, store.InstanceStatusRunning, store.InstanceStatusStopped, store.InstanceStatusError)
}

func (m *Manager) Stop(ctx context.Context, userID, instanceID uint) error {
	return m.transitionStatus(ctx, userID, instanceID, store.InstanceStatusStopped, store.InstanceStatusRunning)
}

func (m *Manager) Delete(ctx context.Context, userID, instanceID uint) error {
	return m.transitionStatus(ctx, userID, instanceID, store.InstanceStatusDeleted, store.InstanceStatusStopped, store.InstanceStatusRunning, store.InstanceStatusError)
}

func (m *Manager) ListLots(ctx context.Context, userID, instanceID uint) ([]store.SpotLot, error) {
	if _, err := m.loadUserInstance(ctx, userID, instanceID); err != nil {
		return nil, err
	}
	var lots []store.SpotLot
	if err := m.db.WithContext(ctx).
		Where("strategy_instance_id = ?", instanceID).
		Order("created_at ASC").
		Find(&lots).Error; err != nil {
		return nil, err
	}
	return lots, nil
}

func (m *Manager) ListTrades(ctx context.Context, userID, instanceID uint) ([]store.TradeRecord, error) {
	if _, err := m.loadUserInstance(ctx, userID, instanceID); err != nil {
		return nil, err
	}
	var trades []store.TradeRecord
	if err := m.db.WithContext(ctx).
		Where("strategy_instance_id = ?", instanceID).
		Order("created_at DESC").
		Find(&trades).Error; err != nil {
		return nil, err
	}
	return trades, nil
}

func (m *Manager) ListRunningInstances(ctx context.Context) ([]store.StrategyInstance, error) {
	var instances []store.StrategyInstance
	if err := m.db.WithContext(ctx).
		Preload("Template").
		Where("status = ?", store.InstanceStatusRunning).
		Find(&instances).Error; err != nil {
		return nil, err
	}
	return instances, nil
}

func (m *Manager) Tick(ctx context.Context, instanceID uint) error {
	if _, loaded := m.active.LoadOrStore(instanceID, struct{}{}); loaded {
		return nil
	}
	defer m.active.Delete(instanceID)

	var instance store.StrategyInstance
	if err := m.db.WithContext(ctx).
		Preload("Template").
		First(&instance, instanceID).Error; err != nil {
		return err
	}
	if instance.Status != store.InstanceStatusRunning {
		return nil
	}

	bars, err := m.marketData.LoadRecent(ctx, instance.Symbol, 600)
	if err != nil {
		return err
	}
	if len(bars) < quant.MinimumStrategyBars {
		return fmt.Errorf("insufficient 1h bars for %s: need %d got %d", instance.Symbol, quant.MinimumStrategyBars, len(bars))
	}
	latest := bars[len(bars)-1]

	portfolio, err := m.loadOrCreatePortfolio(ctx, instance.ID)
	if err != nil {
		return err
	}
	if latest.OpenTime <= portfolio.LastProcessedBarTime {
		return nil
	}

	runtime, err := m.loadOrCreateRuntime(ctx, instance.ID)
	if err != nil {
		return err
	}

	var lotRows []store.SpotLot
	if err := m.db.WithContext(ctx).
		Where("strategy_instance_id = ?", instance.ID).
		Order("created_at ASC").
		Find(&lotRows).Error; err != nil {
		return err
	}

	spec, err := strategies.Lookup(instance.Template.TemplateKey)
	if err != nil {
		return err
	}
	params, err := m.resolveParams(ctx, spec, instance)
	if err != nil {
		return err
	}

	snapshot := quant.PortfolioSnapshot{
		USDTBalance:          portfolio.USDTBalance,
		DeadAsset:            portfolio.DeadBTC,
		FloatAsset:           portfolio.FloatBTC,
		ColdSealedAsset:      portfolio.ColdSealedBTC,
		LastProcessedBarTime: portfolio.LastProcessedBarTime,
	}
	lots := ledger.NormalizeLotsForSnapshot(ledger.QuantLots(lotRows), snapshot, time.UnixMilli(latest.OpenTime).UTC(), latest.Close)

	closes := make([]float64, 0, len(bars))
	timestamps := make([]int64, 0, len(bars))
	for _, bar := range bars {
		closes = append(closes, bar.Close)
		timestamps = append(timestamps, bar.OpenTime)
	}

	output, err := spec.NewBacktestRunner(params)(quant.StrategyInput{
		TemplateID:   spec.Manifest.ID,
		Symbol:       instance.Symbol,
		Timestamp:    latest.OpenTime,
		Closes:       closes,
		Timestamps:   timestamps,
		Portfolio:    snapshot,
		Lots:         lots,
		RuntimeState: json.RawMessage(runtime.StateBlob),
	})
	if err != nil {
		return err
	}

	persistedLots := ledger.ApplyReleaseIntents(ledger.QuantLots(lotRows), output.ReleaseIntents, time.UnixMilli(latest.OpenTime).UTC())
	commands := buildCommands(instance.ID, output, latest.OpenTime)
	connected := m.hub != nil && m.hub.IsConnected(instance.UserID)

	runtime.StateBlob = datatypes.JSON(output.UpdatedRuntimeState)
	portfolio.LastProcessedBarTime = latest.OpenTime
	ledger.UpdatePortfolioFromLots(portfolio, persistedLots, latest.Close)

	type pendingDispatch struct {
		record store.SpotExecution
		cmd    protocol.TradeCommand
	}
	pending := make([]pendingDispatch, 0, len(commands))

	if err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(runtime).Error; err != nil {
			return err
		}
		if err := ledger.SaveLots(ctx, tx, instance.ID, persistedLots); err != nil {
			return err
		}
		if err := tx.Save(portfolio).Error; err != nil {
			return err
		}
		if err := dashboard.RecordEquitySnapshot(ctx, tx, dashboard.SnapshotRecord{
			StrategyInstanceID: instance.ID,
			SnapshotTime:       latest.OpenTime,
			Source:             dashboard.SnapshotSourceTick,
			USDTBalance:        portfolio.USDTBalance,
			DeadAssetQty:       portfolio.DeadBTC,
			FloatAssetQty:      portfolio.FloatBTC,
			ColdSealedAssetQty: portfolio.ColdSealedBTC,
			TotalEquity:        portfolio.TotalEquity,
			MarkPrice:          latest.Close,
		}); err != nil {
			return err
		}

		for _, release := range output.ReleaseIntents {
			payload, _ := json.Marshal(release)
			if err := tx.Create(&store.AuditLog{
				UserID:             &instance.UserID,
				StrategyInstanceID: &instance.ID,
				EventType:          "release_intent_applied",
				Payload:            datatypes.JSON(payload),
			}).Error; err != nil {
				return err
			}
		}

		if len(commands) == 0 {
			return nil
		}
		if !connected {
			for _, cmd := range commands {
				payload, _ := json.Marshal(cmd)
				if err := tx.Create(&store.AuditLog{
					UserID:             &instance.UserID,
					StrategyInstanceID: &instance.ID,
					EventType:          "agent_offline_command_skipped",
					Payload:            datatypes.JSON(payload),
				}).Error; err != nil {
					return err
				}
			}
			return nil
		}

		for _, cmd := range commands {
			record := store.SpotExecution{
				StrategyInstanceID: instance.ID,
				ClientOrderID:      cmd.ClientOrderID,
				Action:             cmd.Action,
				Engine:             cmd.Engine,
				Symbol:             cmd.Symbol,
				AmountUSDT:         cmd.AmountUSDT,
				QtyAsset:           cmd.QtyAsset,
				LotType:            cmd.LotType,
				Status:             "pending",
			}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
			pending = append(pending, pendingDispatch{record: record, cmd: cmd})
		}
		return nil
	}); err != nil {
		return err
	}

	for _, item := range pending {
		if err := m.hub.SendToAgent(instance.UserID, item.cmd); err != nil {
			m.logger.Warn("dispatch trade command", zap.Uint("instance_id", instance.ID), zap.String("client_order_id", item.cmd.ClientOrderID), zap.Error(err))
			_ = m.db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&store.SpotExecution{}).
					Where("client_order_id = ?", item.cmd.ClientOrderID).
					Update("status", "dispatch_failed").Error; err != nil {
					return err
				}
				payload, _ := json.Marshal(map[string]any{
					"command": item.cmd,
					"error":   err.Error(),
				})
				return tx.Create(&store.AuditLog{
					UserID:             &instance.UserID,
					StrategyInstanceID: &instance.ID,
					EventType:          "command_dispatch_failed",
					Payload:            datatypes.JSON(payload),
				}).Error
			})
		}
	}

	return nil
}

func (m *Manager) transitionStatus(ctx context.Context, userID, instanceID uint, target string, allowed ...string) error {
	instance, err := m.loadUserInstance(ctx, userID, instanceID)
	if err != nil {
		return err
	}

	current := strings.ToUpper(instance.Status)
	ok := false
	for _, status := range allowed {
		if current == status {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("cannot transition instance from %s to %s", current, target)
	}

	return m.db.WithContext(ctx).
		Model(&store.StrategyInstance{}).
		Where("id = ?", instanceID).
		Update("status", target).Error
}

func (m *Manager) loadUserInstance(ctx context.Context, userID, instanceID uint) (*store.StrategyInstance, error) {
	var instance store.StrategyInstance
	if err := m.db.WithContext(ctx).
		Preload("Template").
		Where("id = ? AND user_id = ?", instanceID, userID).
		First(&instance).Error; err != nil {
		return nil, err
	}
	return &instance, nil
}

func (m *Manager) loadOrCreatePortfolio(ctx context.Context, instanceID uint) (*store.PortfolioState, error) {
	var portfolio store.PortfolioState
	err := m.db.WithContext(ctx).Where("strategy_instance_id = ?", instanceID).Take(&portfolio).Error
	if err == nil {
		return &portfolio, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	portfolio = store.PortfolioState{StrategyInstanceID: instanceID}
	if err := m.db.WithContext(ctx).Create(&portfolio).Error; err != nil {
		return nil, err
	}
	return &portfolio, nil
}

func (m *Manager) loadOrCreateRuntime(ctx context.Context, instanceID uint) (*store.RuntimeState, error) {
	var runtime store.RuntimeState
	err := m.db.WithContext(ctx).Where("strategy_instance_id = ?", instanceID).Take(&runtime).Error
	if err == nil {
		return &runtime, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	runtime = store.RuntimeState{
		StrategyInstanceID: instanceID,
		StateBlob:          datatypes.JSON(coretemplate.EncodeState(coretemplate.RuntimeState{})),
	}
	if err := m.db.WithContext(ctx).Create(&runtime).Error; err != nil {
		return nil, err
	}
	return &runtime, nil
}

func (m *Manager) resolveParams(ctx context.Context, spec strategies.Spec, instance store.StrategyInstance) (coretemplate.Params, error) {
	cacheKey := "champion:param_pack:" + spec.Manifest.ID
	if m.cache != nil {
		if cached, err := m.cache.Get(ctx, cacheKey); err == nil && strings.TrimSpace(cached) != "" {
			params, parseErr := spec.ParseParamPack(json.RawMessage(cached))
			if parseErr == nil {
				return overrideInstanceSpawn(instance, params), nil
			}
		}
	}

	var champion store.GeneRecord
	err := m.db.WithContext(ctx).
		Where("strategy_id = ? AND role = ?", spec.Manifest.ID, store.GeneRoleChampion).
		Order("created_at DESC").
		Take(&champion).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return overrideInstanceSpawn(instance, spec.DefaultParams()), nil
		}
		return coretemplate.Params{}, err
	}

	if m.cache != nil {
		_ = m.cache.Set(ctx, cacheKey, string(champion.ParamPack), championCacheTTL)
	}

	params, err := spec.ParseParamPack(json.RawMessage(champion.ParamPack))
	if err != nil {
		return coretemplate.Params{}, err
	}
	return overrideInstanceSpawn(instance, params), nil
}

func overrideInstanceSpawn(instance store.StrategyInstance, params coretemplate.Params) coretemplate.Params {
	if instance.MonthlyInjectUSDT > 0 {
		params.SpawnPoint.Policy.MonthlyInjectUSDT = instance.MonthlyInjectUSDT
	}
	return params
}

func buildCommands(instanceID uint, output quant.StrategyOutput, timestamp int64) []protocol.TradeCommand {
	commands := make([]protocol.TradeCommand, 0, 2)
	appendIntent := func(intent *quant.OrderIntent) {
		if intent == nil {
			return
		}
		commands = append(commands, protocol.TradeCommand{
			ClientOrderID:      fmt.Sprintf("inst%d-%s-%d", instanceID, strings.ToLower(intent.Engine), timestamp),
			StrategyInstanceID: instanceID,
			Action:             intent.Action,
			Engine:             intent.Engine,
			Symbol:             intent.Symbol,
			AmountUSDT:         intent.AmountUSDT,
			QtyAsset:           intent.QtyAsset,
			LotType:            intent.LotType,
			Reason:             intent.Reason,
			SentAt:             time.Now().UTC().UnixMilli(),
		})
	}
	appendIntent(output.MacroIntent)
	appendIntent(output.MicroIntent)
	return commands
}
