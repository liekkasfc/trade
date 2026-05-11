package dashboard

import (
	"context"
	"testing"
	"time"

	"quantsaas/internal/protocol"
	"quantsaas/internal/saas/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type stubHub struct {
	status protocol.AgentStatus
}

func (s stubHub) StatusForUser(uint) protocol.AgentStatus {
	return s.status
}

func TestLoadDashboardAndSnapshots(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(store.AllModels()...); err != nil {
		t.Fatalf("migrate models: %v", err)
	}

	now := time.Now().UTC()
	user := store.User{Email: "user@example.com", PasswordHash: "x", Role: store.UserRoleUser, Plan: "core"}
	template := store.StrategyTemplate{TemplateKey: "core-btc-v1", Name: "BTC Core", Version: "v1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&template).Error; err != nil {
		t.Fatalf("create template: %v", err)
	}

	instance := store.StrategyInstance{
		UserID:             user.ID,
		TemplateID:         template.ID,
		Name:               "BTC Sleeve",
		Symbol:             "BTCUSDT",
		Status:             store.InstanceStatusRunning,
		CapitalQuotaUSDT:   10000,
		MonthlyInjectUSDT:  500,
		ColdSealedAssetQty: 0.2,
		MaxDrawdownPct:     0.35,
	}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}

	lastSync := now
	portfolio := store.PortfolioState{
		StrategyInstanceID:   instance.ID,
		USDTBalance:          4200,
		DeadBTC:              0.15,
		FloatBTC:             0.05,
		ColdSealedBTC:        0.2,
		TotalEquity:          12600,
		LastProcessedBarTime: now.Add(-2 * time.Hour).UnixMilli(),
		LastSyncedAt:         &lastSync,
	}
	if err := db.Create(&portfolio).Error; err != nil {
		t.Fatalf("create portfolio: %v", err)
	}

	if err := db.Create(&store.KLine{
		Symbol:   "BTCUSDT",
		Interval: "1h",
		OpenTime: now.Add(-1 * time.Hour).UnixMilli(),
		Open:     62000,
		High:     63000,
		Low:      61000,
		Close:    62500,
		Volume:   123,
	}).Error; err != nil {
		t.Fatalf("create kline: %v", err)
	}

	if err := db.Create(&store.TradeRecord{
		Model:              gorm.Model{CreatedAt: now},
		StrategyInstanceID: instance.ID,
		ClientOrderID:      "now",
		Action:             "BUY",
		Engine:             "MICRO",
		Symbol:             "BTCUSDT",
		FilledQty:          0.01,
		FilledPrice:        62000,
	}).Error; err != nil {
		t.Fatalf("create recent trade: %v", err)
	}
	if err := db.Create(&store.TradeRecord{
		Model:              gorm.Model{CreatedAt: now.AddDate(0, -1, 0)},
		StrategyInstanceID: instance.ID,
		ClientOrderID:      "old",
		Action:             "SELL",
		Engine:             "MACRO",
		Symbol:             "BTCUSDT",
		FilledQty:          0.01,
		FilledPrice:        60000,
	}).Error; err != nil {
		t.Fatalf("create old trade: %v", err)
	}

	oldSnapshotTime := now.AddDate(0, 0, -45).UnixMilli()
	recentSnapshotTime := now.AddDate(0, 0, -5).UnixMilli()
	for _, snapshot := range []store.EquitySnapshot{
		{
			StrategyInstanceID: instance.ID,
			SnapshotTime:       oldSnapshotTime,
			Source:             SnapshotSourceTick,
			USDTBalance:        4000,
			DeadAssetQty:       0.1,
			FloatAssetQty:      0.05,
			ColdSealedAssetQty: 0.2,
			TotalEquity:        11000,
			MarkPrice:          58000,
		},
		{
			StrategyInstanceID: instance.ID,
			SnapshotTime:       recentSnapshotTime,
			Source:             SnapshotSourceTick,
			USDTBalance:        4200,
			DeadAssetQty:       0.15,
			FloatAssetQty:      0.05,
			ColdSealedAssetQty: 0.2,
			TotalEquity:        12600,
			MarkPrice:          62500,
		},
		{
			StrategyInstanceID: instance.ID,
			SnapshotTime:       now.AddDate(0, 0, -1).UnixMilli(),
			Source:             SnapshotSourceDeltaReport,
			USDTBalance:        4210,
			DeadAssetQty:       0.15,
			FloatAssetQty:      0.05,
			ColdSealedAssetQty: 0.2,
			TotalEquity:        12650,
			MarkPrice:          62600,
		},
	} {
		if err := db.Create(&snapshot).Error; err != nil {
			t.Fatalf("create snapshot: %v", err)
		}
	}

	svc := NewService(db, stubHub{status: protocol.AgentStatus{Connected: true, Version: "test-agent"}}, "dev")

	overview, err := svc.LoadDashboard(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("load dashboard: %v", err)
	}
	if overview.Summary.InstanceCount != 1 {
		t.Fatalf("instance count = %d, want 1", overview.Summary.InstanceCount)
	}
	if overview.Summary.TotalEquity != 12600 {
		t.Fatalf("total equity = %v, want 12600", overview.Summary.TotalEquity)
	}
	if len(overview.Instances) != 1 {
		t.Fatalf("instances len = %d, want 1", len(overview.Instances))
	}
	if overview.Instances[0].TradeCountTotal != 2 {
		t.Fatalf("trade total = %d, want 2", overview.Instances[0].TradeCountTotal)
	}
	if overview.Instances[0].TradeCountMonth != 1 {
		t.Fatalf("trade month = %d, want 1", overview.Instances[0].TradeCountMonth)
	}
	if overview.Instances[0].DecisionCount != 2 {
		t.Fatalf("decision count = %d, want 2", overview.Instances[0].DecisionCount)
	}
	if overview.Instances[0].CurrentPrice != 62500 {
		t.Fatalf("current price = %v, want 62500", overview.Instances[0].CurrentPrice)
	}

	snapshots, err := svc.LoadEquitySnapshots(context.Background(), user.ID, instance.ID, 30)
	if err != nil {
		t.Fatalf("load snapshots: %v", err)
	}
	if len(snapshots.Snapshots) != 2 {
		t.Fatalf("snapshot len = %d, want 2", len(snapshots.Snapshots))
	}
	if snapshots.Snapshots[0].SnapshotTime != recentSnapshotTime {
		t.Fatalf("first returned snapshot = %d, want %d", snapshots.Snapshots[0].SnapshotTime, recentSnapshotTime)
	}

	status, err := svc.LoadSystemStatus(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("load system status: %v", err)
	}
	if status.EngineStatus != "running" {
		t.Fatalf("engine status = %s, want running", status.EngineStatus)
	}
	if !status.Agent.Connected {
		t.Fatalf("agent should be connected")
	}
}
