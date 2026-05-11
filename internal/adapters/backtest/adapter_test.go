package backtest_test

import (
	"context"
	"reflect"
	"testing"

	"quantsaas/internal/adapters/backtest"
	"quantsaas/internal/quant"
	"quantsaas/internal/strategies/corebtcv1"
)

func TestRunIsDeterministic(t *testing.T) {
	bars := syntheticBars(900)
	params := corebtcv1.DefaultParams()

	opts := backtest.RunOptions{
		TemplateID:         corebtcv1.Manifest.ID,
		Symbol:             corebtcv1.Manifest.Symbol,
		Bars:               bars,
		InitialCapitalUSDT: 10000,
		SpawnPoint:         params.SpawnPoint,
		Runner:             corebtcv1.NewBacktestRunner(params),
	}

	first, err := backtest.Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	second, err := backtest.Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic results, got\nfirst: %+v\nsecond: %+v", first, second)
	}
}

func syntheticBars(count int) []quant.Bar {
	bars := make([]quant.Bar, 0, count)
	price := 25000.0
	openTime := int64(1704067200000) // 2024-01-01T00:00:00Z

	for i := 0; i < count; i++ {
		drift := 18.0
		seasonal := float64((i%24)-12) * 1.3
		shock := float64((i%7)-3) * 4.2
		price = price + drift + seasonal + shock
		if price < 1000 {
			price = 1000
		}
		bar := quant.Bar{
			OpenTime: openTime + int64(i)*3600_000,
			Open:     price - 10,
			High:     price + 25,
			Low:      price - 20,
			Close:    price,
			Volume:   100 + float64(i%10),
		}
		bars = append(bars, bar)
	}

	return bars
}
