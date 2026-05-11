package quant

import (
	"errors"
	"slices"
	"time"
)

func DeepCopyLots(lots []SpotLot) []SpotLot {
	out := make([]SpotLot, len(lots))
	copy(out, lots)
	return out
}

func DeadAssetAmount(lots []SpotLot) float64 {
	var total float64
	for _, lot := range lots {
		if lot.LotType == LotTypeDead && !lot.IsColdSealed {
			total += lot.Amount
		}
	}
	return total
}

func FloatAssetAmount(lots []SpotLot) float64 {
	var total float64
	for _, lot := range lots {
		if lot.LotType == LotTypeFloating {
			total += lot.Amount
		}
	}
	return total
}

func ApplyBuyLot(lots []SpotLot, lotType string, qty, price float64, at time.Time, isColdSealed bool) []SpotLot {
	if qty <= 0 {
		return lots
	}
	lots = append(lots, SpotLot{
		LotType:      lotType,
		Amount:       qty,
		CostPrice:    price,
		CreatedAt:    at,
		IsColdSealed: isColdSealed,
	})
	return lots
}

func SoftReleaseLots(lots []SpotLot, now time.Time, minAgeMonths int, maxReleasePct, desiredQty float64) ([]SpotLot, float64) {
	if desiredQty <= 0 || maxReleasePct <= 0 {
		return DeepCopyLots(lots), 0
	}
	updated := DeepCopyLots(lots)
	eligibleCap := DeadAssetAmount(updated) * maxReleasePct
	target := minFloat(desiredQty, eligibleCap)
	if target <= 0 {
		return updated, 0
	}

	cutoff := now.AddDate(0, -minAgeMonths, 0)
	sortLotsByCreatedAt(updated)

	var released float64
	for i := range updated {
		lot := &updated[i]
		if lot.LotType != LotTypeDead || lot.IsColdSealed || lot.CreatedAt.After(cutoff) || lot.Amount <= 0 {
			continue
		}
		move := minFloat(lot.Amount, target-released)
		if move <= 0 {
			continue
		}
		lot.Amount -= move
		updated = append(updated, SpotLot{
			LotType:      LotTypeFloating,
			Amount:       move,
			CostPrice:    lot.CostPrice,
			CreatedAt:    now,
			IsColdSealed: false,
		})
		released += move
		if released >= target {
			break
		}
	}

	return compactLots(updated), released
}

func HardReleaseLots(lots []SpotLot, now time.Time, maxReleasePct, desiredQty float64) ([]SpotLot, float64) {
	if desiredQty <= 0 || maxReleasePct <= 0 {
		return DeepCopyLots(lots), 0
	}
	updated := DeepCopyLots(lots)
	eligibleCap := DeadAssetAmount(updated) * maxReleasePct
	target := minFloat(desiredQty, eligibleCap)
	if target <= 0 {
		return updated, 0
	}

	sortLotsByCreatedAt(updated)

	var released float64
	for i := range updated {
		lot := &updated[i]
		if lot.LotType != LotTypeDead || lot.IsColdSealed || lot.Amount <= 0 {
			continue
		}
		move := minFloat(lot.Amount, target-released)
		if move <= 0 {
			continue
		}
		lot.Amount -= move
		updated = append(updated, SpotLot{
			LotType:      LotTypeFloating,
			Amount:       move,
			CostPrice:    lot.CostPrice,
			CreatedAt:    now,
			IsColdSealed: false,
		})
		released += move
		if released >= target {
			break
		}
	}

	return compactLots(updated), released
}

func SellFromFloatingLots(lots []SpotLot, qty float64) ([]SpotLot, error) {
	if qty <= 0 {
		return DeepCopyLots(lots), nil
	}
	updated := DeepCopyLots(lots)
	sortLotsByCreatedAt(updated)

	remaining := qty
	for i := range updated {
		lot := &updated[i]
		if lot.LotType != LotTypeFloating || lot.Amount <= 0 {
			continue
		}
		take := minFloat(lot.Amount, remaining)
		lot.Amount -= take
		remaining -= take
		if remaining <= epsilon {
			return compactLots(updated), nil
		}
	}
	return compactLots(updated), errors.New("insufficient floating lots")
}

func compactLots(lots []SpotLot) []SpotLot {
	out := make([]SpotLot, 0, len(lots))
	for _, lot := range lots {
		if lot.Amount <= epsilon {
			continue
		}
		out = append(out, lot)
	}
	return out
}

func sortLotsByCreatedAt(lots []SpotLot) {
	slices.SortFunc(lots, func(a, b SpotLot) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return 0
	})
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
