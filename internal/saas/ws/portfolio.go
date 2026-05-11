package ws

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"quantsaas/internal/protocol"
	"quantsaas/internal/quant"
	"quantsaas/internal/saas/dashboard"
	"quantsaas/internal/saas/ledger"
	"quantsaas/internal/saas/marketdata"
	"quantsaas/internal/saas/store"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func (h *Hub) processDeltaReport(ctx context.Context, userID uint, report protocol.DeltaReport) error {
	if report.ReportedAt == 0 {
		report.ReportedAt = time.Now().UTC().UnixMilli()
	}

	if strings.TrimSpace(report.ClientOrderID) == "" {
		return h.processAccountSnapshot(ctx, userID, report)
	}

	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pending store.SpotExecution
		if err := tx.Where("client_order_id = ?", report.ClientOrderID).Take(&pending).Error; err != nil {
			return err
		}

		var instance store.StrategyInstance
		if err := tx.First(&instance, pending.StrategyInstanceID).Error; err != nil {
			return err
		}
		if instance.UserID != userID {
			return errors.New("delta report user mismatch")
		}

		portfolio, err := loadOrCreatePortfolioTx(ctx, tx, instance.ID)
		if err != nil {
			return err
		}

		var lotRows []store.SpotLot
		if err := tx.WithContext(ctx).
			Where("strategy_instance_id = ?", instance.ID).
			Order("created_at ASC").
			Find(&lotRows).Error; err != nil {
			return err
		}
		markPrice := latestPriceOrZero(h.marketData, ctx, instance.Symbol)

		lots := ledger.NormalizeLotsForSnapshot(
			ledger.QuantLots(lotRows),
			quant.PortfolioSnapshot{
				USDTBalance:     portfolio.USDTBalance,
				DeadAsset:       portfolio.DeadBTC,
				FloatAsset:      portfolio.FloatBTC,
				ColdSealedAsset: portfolio.ColdSealedBTC,
			},
			time.UnixMilli(report.ReportedAt).UTC(),
			markPrice,
		)

		if report.Execution != nil {
			price := report.Execution.FilledPrice
			if price <= 0 {
				price = markPrice
			}

			var applyErr error
			lots, applyErr = ledger.ApplyExecution(lots, pending.Action, pending.LotType, report.Execution.FilledQty, price, time.UnixMilli(report.ReportedAt).UTC())
			if applyErr != nil {
				return applyErr
			}

			adjustVirtualUSDT(portfolio, pending.Action, pending.AmountUSDT, pending.QtyAsset, report.Execution)

			rawExec, _ := json.Marshal(report.Execution)
			updates := map[string]any{
				"status":        normalizeExecutionStatus(report.Execution.Status),
				"raw_execution": datatypes.JSON(rawExec),
			}
			if err := tx.Model(&store.SpotExecution{}).
				Where("id = ?", pending.ID).
				Updates(updates).Error; err != nil {
				return err
			}

			trade := store.TradeRecord{
				StrategyInstanceID: instance.ID,
				ClientOrderID:      pending.ClientOrderID,
				Action:             pending.Action,
				Engine:             pending.Engine,
				Symbol:             pending.Symbol,
				FilledQty:          report.Execution.FilledQty,
				FilledPrice:        report.Execution.FilledPrice,
				Fee:                report.Execution.Fee,
			}
			if err := tx.Create(&trade).Error; err != nil {
				return err
			}
		}

		ledger.UpdatePortfolioFromLots(portfolio, lots, markPrice)
		if len(report.Balances) > 0 && countUserInstances(tx, ctx, userID) == 1 {
			baseQty, usdtQty := extractBalances(instance.Symbol, report.Balances)
			ledger.UpdatePortfolioFromBalances(portfolio, lots, baseQty, usdtQty, markPrice)
		} else if len(report.Balances) > 0 {
			now := time.Now().UTC()
			portfolio.LastSyncedAt = &now
		}

		if err := ledger.SaveLots(ctx, tx, instance.ID, lots); err != nil {
			return err
		}
		if err := tx.Save(portfolio).Error; err != nil {
			return err
		}
		if err := dashboard.RecordEquitySnapshot(ctx, tx, dashboard.SnapshotRecord{
			StrategyInstanceID: instance.ID,
			SnapshotTime:       report.ReportedAt,
			Source:             dashboard.SnapshotSourceDeltaReport,
			USDTBalance:        portfolio.USDTBalance,
			DeadAssetQty:       portfolio.DeadBTC,
			FloatAssetQty:      portfolio.FloatBTC,
			ColdSealedAssetQty: portfolio.ColdSealedBTC,
			TotalEquity:        portfolio.TotalEquity,
			MarkPrice:          markPrice,
		}); err != nil {
			return err
		}

		payload, _ := json.Marshal(report)
		return tx.Create(&store.AuditLog{
			UserID:             &userID,
			StrategyInstanceID: &instance.ID,
			EventType:          "delta_report_applied",
			Payload:            datatypes.JSON(payload),
		}).Error
	})
}

func (h *Hub) processAccountSnapshot(ctx context.Context, userID uint, report protocol.DeltaReport) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var instances []store.StrategyInstance
		if err := tx.WithContext(ctx).
			Where("user_id = ? AND status <> ?", userID, store.InstanceStatusDeleted).
			Order("created_at DESC").
			Find(&instances).Error; err != nil {
			return err
		}

		if len(instances) == 1 {
			instance := instances[0]
			portfolio, err := loadOrCreatePortfolioTx(ctx, tx, instance.ID)
			if err != nil {
				return err
			}

			var lotRows []store.SpotLot
			if err := tx.WithContext(ctx).
				Where("strategy_instance_id = ?", instance.ID).
				Order("created_at ASC").
				Find(&lotRows).Error; err != nil {
				return err
			}
			baseQty, usdtQty := extractBalances(instance.Symbol, report.Balances)
			markPrice := latestPriceOrZero(h.marketData, ctx, instance.Symbol)
			ledger.UpdatePortfolioFromBalances(portfolio, ledger.QuantLots(lotRows), baseQty, usdtQty, markPrice)
			if err := tx.Save(portfolio).Error; err != nil {
				return err
			}
			if err := dashboard.RecordEquitySnapshot(ctx, tx, dashboard.SnapshotRecord{
				StrategyInstanceID: instance.ID,
				SnapshotTime:       report.ReportedAt,
				Source:             dashboard.SnapshotSourceAccountBalance,
				USDTBalance:        portfolio.USDTBalance,
				DeadAssetQty:       portfolio.DeadBTC,
				FloatAssetQty:      portfolio.FloatBTC,
				ColdSealedAssetQty: portfolio.ColdSealedBTC,
				TotalEquity:        portfolio.TotalEquity,
				MarkPrice:          markPrice,
			}); err != nil {
				return err
			}
		}

		payload, _ := json.Marshal(report)
		return tx.Create(&store.AuditLog{
			UserID:    &userID,
			EventType: "agent_balance_snapshot",
			Payload:   datatypes.JSON(payload),
		}).Error
	})
}

func loadOrCreatePortfolioTx(ctx context.Context, tx *gorm.DB, instanceID uint) (*store.PortfolioState, error) {
	var portfolio store.PortfolioState
	err := tx.WithContext(ctx).Where("strategy_instance_id = ?", instanceID).Take(&portfolio).Error
	if err == nil {
		return &portfolio, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	portfolio = store.PortfolioState{StrategyInstanceID: instanceID}
	if err := tx.WithContext(ctx).Create(&portfolio).Error; err != nil {
		return nil, err
	}
	return &portfolio, nil
}

func latestPriceOrZero(service *marketdata.Service, ctx context.Context, symbol string) float64 {
	if service == nil {
		return 0
	}
	return service.LatestClose(ctx, symbol)
}

func extractBalances(symbol string, balances []protocol.Balance) (baseQty, usdtQty float64) {
	baseAsset := strings.TrimSuffix(symbol, "USDT")
	for _, balance := range balances {
		switch strings.ToUpper(balance.Asset) {
		case strings.ToUpper(baseAsset):
			baseQty = balance.Total()
		case "USDT":
			usdtQty = balance.Total()
		}
	}
	return math.Max(baseQty, 0), math.Max(usdtQty, 0)
}

func adjustVirtualUSDT(portfolio *store.PortfolioState, action string, pendingAmountUSDT, pendingQtyAsset float64, execution *protocol.Execution) {
	if portfolio == nil || execution == nil {
		return
	}

	quoteAmount := execution.QuoteAmount
	if quoteAmount <= 0 {
		quoteAmount = pendingAmountUSDT
		if action == quant.ActionSell && execution.FilledPrice > 0 {
			quoteAmount = pendingQtyAsset * execution.FilledPrice
		}
	}
	if execution.Fee > 0 && strings.EqualFold(execution.FeeAsset, "USDT") {
		quoteAmount -= execution.Fee
	}

	switch action {
	case quant.ActionBuy:
		portfolio.USDTBalance = math.Max(0, portfolio.USDTBalance-math.Max(quoteAmount, 0))
	case quant.ActionSell:
		portfolio.USDTBalance += math.Max(quoteAmount, 0)
	}
}

func normalizeExecutionStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return "filled"
	}
	return strings.ToLower(status)
}

func countUserInstances(tx *gorm.DB, ctx context.Context, userID uint) int64 {
	var count int64
	_ = tx.WithContext(ctx).
		Model(&store.StrategyInstance{}).
		Where("user_id = ? AND status <> ?", userID, store.InstanceStatusDeleted).
		Count(&count).Error
	return count
}
