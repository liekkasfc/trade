package ledger

import (
	"context"
	"errors"
	"math"
	"time"

	"quantsaas/internal/quant"
	"quantsaas/internal/saas/store"

	"gorm.io/gorm"
)

const epsilon = 1e-9

func QuantLots(rows []store.SpotLot) []quant.SpotLot {
	out := make([]quant.SpotLot, 0, len(rows))
	for _, row := range rows {
		out = append(out, quant.SpotLot{
			LotType:      row.LotType,
			Amount:       row.Amount,
			CostPrice:    row.CostPrice,
			CreatedAt:    row.CreatedAt.UTC(),
			IsColdSealed: row.IsColdSealed,
		})
	}
	return out
}

func StoreLots(instanceID uint, lots []quant.SpotLot) []store.SpotLot {
	out := make([]store.SpotLot, 0, len(lots))
	for _, lot := range lots {
		out = append(out, store.SpotLot{
			StrategyInstanceID: instanceID,
			LotType:            lot.LotType,
			Amount:             lot.Amount,
			CostPrice:          lot.CostPrice,
			IsColdSealed:       lot.IsColdSealed,
			Model: gorm.Model{
				CreatedAt: lot.CreatedAt.UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		})
	}
	return out
}

func SaveLots(ctx context.Context, tx *gorm.DB, instanceID uint, lots []quant.SpotLot) error {
	if err := tx.WithContext(ctx).Where("strategy_instance_id = ?", instanceID).Delete(&store.SpotLot{}).Error; err != nil {
		return err
	}
	rows := StoreLots(instanceID, lots)
	if len(rows) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&rows).Error
}

func NormalizeLotsForSnapshot(lots []quant.SpotLot, snapshot quant.PortfolioSnapshot, now time.Time, fallbackPrice float64) []quant.SpotLot {
	normalized := quant.DeepCopyLots(lots)
	reportedTotal := snapshot.TotalAsset()
	if reportedTotal <= epsilon {
		return normalized
	}

	knownTotal := quant.DeadAssetAmount(normalized) + quant.FloatAssetAmount(normalized) + coldAssetAmount(normalized)
	missing := reportedTotal - knownTotal
	if missing <= epsilon {
		return normalized
	}

	normalized = append(normalized, quant.SpotLot{
		LotType:      quant.LotTypeFloating,
		Amount:       missing,
		CostPrice:    fallbackPrice,
		CreatedAt:    now.UTC(),
		IsColdSealed: false,
	})
	return normalized
}

func ApplyReleaseIntents(lots []quant.SpotLot, intents []quant.ReleaseIntent, at time.Time) []quant.SpotLot {
	updated := quant.DeepCopyLots(lots)
	for _, intent := range intents {
		switch intent.Mode {
		case "soft":
			updated, _ = quant.SoftReleaseLots(updated, at.UTC(), 0, 1.0, intent.AmountAsset)
		case "hard":
			updated, _ = quant.HardReleaseLots(updated, at.UTC(), 1.0, intent.AmountAsset)
		}
	}
	return updated
}

func ApplyExecution(lots []quant.SpotLot, action, lotType string, filledQty, filledPrice float64, at time.Time) ([]quant.SpotLot, error) {
	updated := quant.DeepCopyLots(lots)
	switch action {
	case quant.ActionBuy:
		updated = quant.ApplyBuyLot(updated, lotType, filledQty, filledPrice, at.UTC(), lotType == quant.LotTypeColdSealed)
		return updated, nil
	case quant.ActionSell:
		if filledQty <= epsilon {
			return updated, nil
		}
		return quant.SellFromFloatingLots(updated, filledQty)
	default:
		return nil, errors.New("unsupported execution action")
	}
}

func UpdatePortfolioFromLots(portfolio *store.PortfolioState, lots []quant.SpotLot, price float64) {
	dead, floating, cold := RebuildBuckets(lots)
	portfolio.DeadBTC = dead
	portfolio.FloatBTC = floating
	portfolio.ColdSealedBTC = cold
	portfolio.TotalEquity = portfolio.USDTBalance + (dead+floating+cold)*math.Max(price, 0)
}

func UpdatePortfolioFromBalances(portfolio *store.PortfolioState, lots []quant.SpotLot, baseAssetQty, usdtQty, price float64) {
	dead, floating, cold := scaledBuckets(lots, baseAssetQty)
	portfolio.USDTBalance = math.Max(usdtQty, 0)
	portfolio.DeadBTC = dead
	portfolio.FloatBTC = floating
	portfolio.ColdSealedBTC = cold
	portfolio.TotalEquity = portfolio.USDTBalance + math.Max(baseAssetQty, 0)*math.Max(price, 0)
	now := time.Now().UTC()
	portfolio.LastSyncedAt = &now
}

func RebuildBuckets(lots []quant.SpotLot) (dead, floating, cold float64) {
	for _, lot := range lots {
		switch lot.LotType {
		case quant.LotTypeDead:
			if lot.IsColdSealed {
				cold += lot.Amount
			} else {
				dead += lot.Amount
			}
		case quant.LotTypeFloating:
			floating += lot.Amount
		case quant.LotTypeColdSealed:
			cold += lot.Amount
		}
	}
	return dead, floating, cold
}

func coldAssetAmount(lots []quant.SpotLot) float64 {
	_, _, cold := RebuildBuckets(lots)
	return cold
}

func scaledBuckets(lots []quant.SpotLot, targetTotal float64) (dead, floating, cold float64) {
	targetTotal = math.Max(targetTotal, 0)
	dead, floating, cold = RebuildBuckets(lots)
	knownTotal := dead + floating + cold
	if targetTotal <= epsilon {
		return 0, 0, 0
	}
	if knownTotal <= epsilon {
		return 0, targetTotal, 0
	}
	scale := targetTotal / knownTotal
	return dead * scale, floating * scale, cold * scale
}
