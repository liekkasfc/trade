package dashboard

import (
	"context"
	"errors"
	"time"

	"quantsaas/internal/protocol"
	"quantsaas/internal/saas/config"
	"quantsaas/internal/saas/store"

	"gorm.io/gorm"
)

type AgentStatusSource interface {
	StatusForUser(uint) protocol.AgentStatus
}

type Service struct {
	db      *gorm.DB
	hub     AgentStatusSource
	appRole string
}

type Overview struct {
	Summary   Summary            `json:"summary"`
	Instances []InstanceOverview `json:"instances"`
}

type Summary struct {
	InstanceCount           int     `json:"instance_count"`
	RunningInstanceCount    int     `json:"running_instance_count"`
	StoppedInstanceCount    int     `json:"stopped_instance_count"`
	ErrorInstanceCount      int     `json:"error_instance_count"`
	TotalEquity             float64 `json:"total_equity"`
	TotalUSDTBalance        float64 `json:"total_usdt_balance"`
	TotalDeadAssetQty       float64 `json:"total_dead_asset_qty"`
	TotalFloatAssetQty      float64 `json:"total_float_asset_qty"`
	TotalColdSealedAssetQty float64 `json:"total_cold_sealed_asset_qty"`
}

type InstanceOverview struct {
	ID                           uint    `json:"id"`
	TemplateID                   string  `json:"template_id"`
	TemplateName                 string  `json:"template_name"`
	Name                         string  `json:"name"`
	Symbol                       string  `json:"symbol"`
	Status                       string  `json:"status"`
	CapitalQuotaUSDT             float64 `json:"capital_quota_usdt"`
	MonthlyInjectUSDT            float64 `json:"monthly_inject_usdt"`
	ConfiguredColdSealedAssetQty float64 `json:"configured_cold_sealed_asset_qty"`
	MaxDrawdownPct               float64 `json:"max_drawdown_pct"`
	CurrentPrice                 float64 `json:"current_price"`
	USDTBalance                  float64 `json:"usdt_balance"`
	DeadAssetQty                 float64 `json:"dead_asset_qty"`
	FloatAssetQty                float64 `json:"float_asset_qty"`
	ColdSealedAssetQty           float64 `json:"cold_sealed_asset_qty"`
	TotalEquity                  float64 `json:"total_equity"`
	LastProcessedBarTime         int64   `json:"last_processed_bar_time"`
	LastSyncedAt                 int64   `json:"last_synced_at,omitempty"`
	TradeCountTotal              int64   `json:"trade_count_total"`
	TradeCountMonth              int64   `json:"trade_count_month"`
	SnapshotCount                int64   `json:"snapshot_count"`
	DecisionCount                int64   `json:"decision_count"`
	LatestSnapshotTime           int64   `json:"latest_snapshot_time,omitempty"`
	FirstDecisionTime            int64   `json:"first_decision_time,omitempty"`
	CreatedAt                    int64   `json:"created_at"`
	UpdatedAt                    int64   `json:"updated_at"`
}

type EquitySnapshotsResponse struct {
	InstanceID uint              `json:"instance_id"`
	RangeDays  int               `json:"range_days"`
	Snapshots  []SnapshotPayload `json:"snapshots"`
}

type SnapshotPayload struct {
	SnapshotTime       int64   `json:"snapshot_time"`
	Source             string  `json:"source"`
	USDTBalance        float64 `json:"usdt_balance"`
	DeadAssetQty       float64 `json:"dead_asset_qty"`
	FloatAssetQty      float64 `json:"float_asset_qty"`
	ColdSealedAssetQty float64 `json:"cold_sealed_asset_qty"`
	TotalEquity        float64 `json:"total_equity"`
	MarkPrice          float64 `json:"mark_price"`
}

type SystemStatus struct {
	AppRole           string               `json:"app_role"`
	EngineStatus      string               `json:"engine_status"`
	Agent             protocol.AgentStatus `json:"agent"`
	ReconcileRequired bool                 `json:"reconcile_required"`
	ServerTime        int64                `json:"server_time"`
}

func NewService(db *gorm.DB, hub AgentStatusSource, appRole string) *Service {
	return &Service{
		db:      db,
		hub:     hub,
		appRole: appRole,
	}
}

func (s *Service) LoadDashboard(ctx context.Context, userID uint) (Overview, error) {
	var instances []store.StrategyInstance
	if err := s.db.WithContext(ctx).
		Preload("Template").
		Where("user_id = ? AND status <> ?", userID, store.InstanceStatusDeleted).
		Order("created_at DESC").
		Find(&instances).Error; err != nil {
		return Overview{}, err
	}

	monthStart := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	overview := Overview{
		Instances: make([]InstanceOverview, 0, len(instances)),
		Summary: Summary{
			InstanceCount: len(instances),
		},
	}

	for _, inst := range instances {
		portfolio, err := s.loadPortfolio(ctx, inst.ID)
		if err != nil {
			return Overview{}, err
		}

		tradeCountTotal, err := s.countTrades(ctx, inst.ID, time.Time{})
		if err != nil {
			return Overview{}, err
		}
		tradeCountMonth, err := s.countTrades(ctx, inst.ID, monthStart)
		if err != nil {
			return Overview{}, err
		}
		snapshotCount, err := s.countSnapshots(ctx, inst.ID, "")
		if err != nil {
			return Overview{}, err
		}
		decisionCount, err := s.countSnapshots(ctx, inst.ID, SnapshotSourceTick)
		if err != nil {
			return Overview{}, err
		}
		firstDecisionTime, err := s.extremeSnapshotTime(ctx, inst.ID, SnapshotSourceTick, true)
		if err != nil {
			return Overview{}, err
		}
		latestSnapshot, err := s.latestSnapshot(ctx, inst.ID)
		if err != nil {
			return Overview{}, err
		}

		currentPrice := s.latestStoredClose(ctx, inst.Symbol)
		if currentPrice <= 0 {
			currentPrice = latestSnapshot.MarkPrice
		}

		next := InstanceOverview{
			ID:                           inst.ID,
			TemplateID:                   inst.Template.TemplateKey,
			TemplateName:                 inst.Template.Name,
			Name:                         inst.Name,
			Symbol:                       inst.Symbol,
			Status:                       inst.Status,
			CapitalQuotaUSDT:             inst.CapitalQuotaUSDT,
			MonthlyInjectUSDT:            inst.MonthlyInjectUSDT,
			ConfiguredColdSealedAssetQty: inst.ColdSealedAssetQty,
			MaxDrawdownPct:               inst.MaxDrawdownPct,
			CurrentPrice:                 currentPrice,
			USDTBalance:                  portfolio.USDTBalance,
			DeadAssetQty:                 portfolio.DeadBTC,
			FloatAssetQty:                portfolio.FloatBTC,
			ColdSealedAssetQty:           portfolio.ColdSealedBTC,
			TotalEquity:                  portfolio.TotalEquity,
			LastProcessedBarTime:         portfolio.LastProcessedBarTime,
			TradeCountTotal:              tradeCountTotal,
			TradeCountMonth:              tradeCountMonth,
			SnapshotCount:                snapshotCount,
			DecisionCount:                decisionCount,
			LatestSnapshotTime:           latestSnapshot.SnapshotTime,
			FirstDecisionTime:            firstDecisionTime,
			CreatedAt:                    inst.CreatedAt.UTC().UnixMilli(),
			UpdatedAt:                    inst.UpdatedAt.UTC().UnixMilli(),
		}
		if portfolio.LastSyncedAt != nil {
			next.LastSyncedAt = portfolio.LastSyncedAt.UTC().UnixMilli()
		}

		switch inst.Status {
		case store.InstanceStatusRunning:
			overview.Summary.RunningInstanceCount++
		case store.InstanceStatusStopped:
			overview.Summary.StoppedInstanceCount++
		case store.InstanceStatusError:
			overview.Summary.ErrorInstanceCount++
		}

		overview.Summary.TotalEquity += portfolio.TotalEquity
		overview.Summary.TotalUSDTBalance += portfolio.USDTBalance
		overview.Summary.TotalDeadAssetQty += portfolio.DeadBTC
		overview.Summary.TotalFloatAssetQty += portfolio.FloatBTC
		overview.Summary.TotalColdSealedAssetQty += portfolio.ColdSealedBTC
		overview.Instances = append(overview.Instances, next)
	}

	return overview, nil
}

func (s *Service) LoadEquitySnapshots(ctx context.Context, userID, instanceID uint, rangeDays int) (EquitySnapshotsResponse, error) {
	if rangeDays <= 0 {
		rangeDays = 30
	}

	if _, err := s.loadOwnedInstance(ctx, userID, instanceID); err != nil {
		return EquitySnapshotsResponse{}, err
	}

	since := time.Now().UTC().AddDate(0, 0, -rangeDays).UnixMilli()
	var rows []store.EquitySnapshot
	if err := s.db.WithContext(ctx).
		Where("strategy_instance_id = ? AND snapshot_time >= ?", instanceID, since).
		Order("snapshot_time ASC").
		Find(&rows).Error; err != nil {
		return EquitySnapshotsResponse{}, err
	}

	resp := EquitySnapshotsResponse{
		InstanceID: instanceID,
		RangeDays:  rangeDays,
		Snapshots:  make([]SnapshotPayload, 0, len(rows)),
	}
	for _, row := range rows {
		resp.Snapshots = append(resp.Snapshots, SnapshotPayload{
			SnapshotTime:       row.SnapshotTime,
			Source:             row.Source,
			USDTBalance:        row.USDTBalance,
			DeadAssetQty:       row.DeadAssetQty,
			FloatAssetQty:      row.FloatAssetQty,
			ColdSealedAssetQty: row.ColdSealedAssetQty,
			TotalEquity:        row.TotalEquity,
			MarkPrice:          row.MarkPrice,
		})
	}
	return resp, nil
}

func (s *Service) LoadSystemStatus(_ context.Context, userID uint) (SystemStatus, error) {
	status := SystemStatus{
		AppRole:           s.appRole,
		EngineStatus:      engineStatusForRole(s.appRole),
		ReconcileRequired: false,
		ServerTime:        time.Now().UTC().UnixMilli(),
	}
	if s.hub != nil {
		status.Agent = s.hub.StatusForUser(userID)
	}
	return status, nil
}

func engineStatusForRole(appRole string) string {
	switch appRole {
	case config.RoleSaaS, config.RoleDev:
		return "running"
	case config.RoleLab:
		return "paused"
	default:
		return "halted"
	}
}

func (s *Service) loadOwnedInstance(ctx context.Context, userID, instanceID uint) (*store.StrategyInstance, error) {
	var instance store.StrategyInstance
	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND status <> ?", instanceID, userID, store.InstanceStatusDeleted).
		Take(&instance).Error; err != nil {
		return nil, err
	}
	return &instance, nil
}

func (s *Service) loadPortfolio(ctx context.Context, instanceID uint) (store.PortfolioState, error) {
	var portfolio store.PortfolioState
	err := s.db.WithContext(ctx).
		Where("strategy_instance_id = ?", instanceID).
		Take(&portfolio).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.PortfolioState{}, nil
	}
	return portfolio, err
}

func (s *Service) countTrades(ctx context.Context, instanceID uint, since time.Time) (int64, error) {
	query := s.db.WithContext(ctx).Model(&store.TradeRecord{}).Where("strategy_instance_id = ?", instanceID)
	if !since.IsZero() {
		query = query.Where("created_at >= ?", since)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Service) countSnapshots(ctx context.Context, instanceID uint, source string) (int64, error) {
	query := s.db.WithContext(ctx).Model(&store.EquitySnapshot{}).Where("strategy_instance_id = ?", instanceID)
	if source != "" {
		query = query.Where("source = ?", source)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Service) extremeSnapshotTime(ctx context.Context, instanceID uint, source string, ascending bool) (int64, error) {
	query := s.db.WithContext(ctx).
		Model(&store.EquitySnapshot{}).
		Select("snapshot_time").
		Where("strategy_instance_id = ?", instanceID)
	if source != "" {
		query = query.Where("source = ?", source)
	}
	if ascending {
		query = query.Order("snapshot_time ASC")
	} else {
		query = query.Order("snapshot_time DESC")
	}

	var row struct {
		SnapshotTime int64
	}
	err := query.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return row.SnapshotTime, err
}

func (s *Service) latestSnapshot(ctx context.Context, instanceID uint) (store.EquitySnapshot, error) {
	var row store.EquitySnapshot
	err := s.db.WithContext(ctx).
		Where("strategy_instance_id = ?", instanceID).
		Order("snapshot_time DESC").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.EquitySnapshot{}, nil
	}
	return row, err
}

func (s *Service) latestStoredClose(ctx context.Context, symbol string) float64 {
	var bar store.KLine
	if err := s.db.WithContext(ctx).
		Where("symbol = ? AND interval = ?", symbol, "1h").
		Order("open_time DESC").
		Take(&bar).Error; err != nil {
		return 0
	}
	return bar.Close
}
