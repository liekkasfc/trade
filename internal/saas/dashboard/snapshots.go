package dashboard

import (
	"context"
	"errors"
	"strings"
	"time"

	"quantsaas/internal/saas/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SnapshotSourceCreate         = "create"
	SnapshotSourceTick           = "tick"
	SnapshotSourceDeltaReport    = "delta_report"
	SnapshotSourceAccountBalance = "account_snapshot"
)

type SnapshotRecord struct {
	StrategyInstanceID uint
	SnapshotTime       int64
	Source             string
	USDTBalance        float64
	DeadAssetQty       float64
	FloatAssetQty      float64
	ColdSealedAssetQty float64
	TotalEquity        float64
	MarkPrice          float64
}

func RecordEquitySnapshot(ctx context.Context, tx *gorm.DB, record SnapshotRecord) error {
	if tx == nil {
		return errors.New("snapshot db handle is nil")
	}
	if record.StrategyInstanceID == 0 {
		return errors.New("strategy_instance_id is required")
	}
	if record.SnapshotTime <= 0 {
		record.SnapshotTime = time.Now().UTC().UnixMilli()
	}
	if strings.TrimSpace(record.Source) == "" {
		record.Source = SnapshotSourceTick
	}

	row := store.EquitySnapshot{
		StrategyInstanceID: record.StrategyInstanceID,
		SnapshotTime:       record.SnapshotTime,
		Source:             record.Source,
		USDTBalance:        record.USDTBalance,
		DeadAssetQty:       record.DeadAssetQty,
		FloatAssetQty:      record.FloatAssetQty,
		ColdSealedAssetQty: record.ColdSealedAssetQty,
		TotalEquity:        record.TotalEquity,
		MarkPrice:          record.MarkPrice,
	}

	now := time.Now().UTC()
	return tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "strategy_instance_id"},
				{Name: "snapshot_time"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"source":                row.Source,
				"usdt_balance":          row.USDTBalance,
				"dead_asset_qty":        row.DeadAssetQty,
				"float_asset_qty":       row.FloatAssetQty,
				"cold_sealed_asset_qty": row.ColdSealedAssetQty,
				"total_equity":          row.TotalEquity,
				"mark_price":            row.MarkPrice,
				"updated_at":            now,
			}),
		}).
		Create(&row).Error
}
