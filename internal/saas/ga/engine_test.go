package ga

import (
	"context"
	"math"
	"math/rand"
	"testing"

	"quantsaas/internal/saas/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRunEpochCreatesChallenger(t *testing.T) {
	db := setupTestDB(t)
	seedKLines(t, db, "BTCUSDT", 1800)

	engine := NewEngine(db)
	result, err := engine.RunEpoch(context.Background(), "core-btc-v1", EpochConfig{
		PopSize:        8,
		MaxGenerations: 2,
	})
	if err != nil {
		t.Fatalf("run epoch failed: %v", err)
	}
	if result.Record.ID == 0 {
		t.Fatalf("expected persisted challenger record")
	}
	if result.Record.Role != store.GeneRoleChallenger {
		t.Fatalf("expected challenger role, got %s", result.Record.Role)
	}
	if math.IsNaN(result.BestScore) || math.IsInf(result.BestScore, 0) {
		t.Fatalf("expected finite best score, got %f", result.BestScore)
	}

	var records []store.GeneRecord
	if err := db.Find(&records).Error; err != nil {
		t.Fatalf("load gene records: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("expected at least one gene record")
	}
}

func TestTournamentSelectPrefersHigherScore(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	population := []Gene{"a", "b", "c", "d"}
	scores := []float64{-10, 5, 2, 1}

	var wins int
	for i := 0; i < 1000; i++ {
		if tournamentSelect(population, scores, 3, rng) == "b" {
			wins++
		}
	}
	if wins < 350 {
		t.Fatalf("expected strong preference for best score, got only %d wins", wins)
	}
}

func TestScoreFitnessWindowPenalizesTargetDrawdownAndTrades(t *testing.T) {
	cfg := NormalizeFitnessConfig(FitnessConfig{
		TargetMaxDrawdown:       0.42,
		DrawdownPenaltyFactor:   4.0,
		TradeCountPenaltyFactor: 0.00002,
		BaselineDrawdownPenalty: 1.5,
	})

	score := scoreFitnessWindow(0.20, 0.45, 0.40, 650, cfg)
	want := 0.20 - 1.5*(0.45-0.40) - 4.0*(0.45-0.42) - 0.00002*650
	if math.Abs(score-want) > 1e-12 {
		t.Fatalf("expected score %.12f, got %.12f", want, score)
	}

	lowerRiskScore := scoreFitnessWindow(0.20, 0.41, 0.40, 650, cfg)
	if lowerRiskScore <= score {
		t.Fatalf("expected lower drawdown to score higher: low risk %.12f high risk %.12f", lowerRiskScore, score)
	}
}

func TestNormalizeFitnessConfigDefaultsRiskControls(t *testing.T) {
	cfg := NormalizeFitnessConfig(FitnessConfig{})

	if cfg.TargetMaxDrawdown != DefaultTargetMaxDrawdown {
		t.Fatalf("expected target drawdown %.2f, got %.2f", DefaultTargetMaxDrawdown, cfg.TargetMaxDrawdown)
	}
	if cfg.DrawdownPenaltyFactor != DefaultDrawdownPenaltyFactor {
		t.Fatalf("expected drawdown penalty %.2f, got %.2f", DefaultDrawdownPenaltyFactor, cfg.DrawdownPenaltyFactor)
	}
	if cfg.TradeCountPenaltyFactor != DefaultTradeCountPenaltyFactor {
		t.Fatalf("expected trade penalty %.5f, got %.5f", DefaultTradeCountPenaltyFactor, cfg.TradeCountPenaltyFactor)
	}
	if cfg.FatalMaxDrawdown != DefaultFatalMaxDrawdown {
		t.Fatalf("expected fatal drawdown %.2f, got %.2f", DefaultFatalMaxDrawdown, cfg.FatalMaxDrawdown)
	}
	if cfg.BaselineDrawdownPenalty != DefaultBaselineDrawdownPenalty {
		t.Fatalf("expected baseline penalty %.2f, got %.2f", DefaultBaselineDrawdownPenalty, cfg.BaselineDrawdownPenalty)
	}
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(store.AllModels()...); err != nil {
		t.Fatalf("migrate models: %v", err)
	}
	return db
}

func seedKLines(t *testing.T, db *gorm.DB, symbol string, count int) {
	t.Helper()
	rows := make([]store.KLine, 0, count)
	price := 25000.0
	openTime := int64(1704067200000)
	for i := 0; i < count; i++ {
		drift := 8.0 + float64((i%48)-24)*0.15
		wave := math.Sin(float64(i)/18.0) * 90
		shock := float64((i%11)-5) * 3.5
		price += drift + wave + shock
		if price < 1000 {
			price = 1000
		}
		rows = append(rows, store.KLine{
			Symbol:   symbol,
			Interval: "1h",
			OpenTime: openTime + int64(i)*3600_000,
			Open:     price - 12,
			High:     price + 22,
			Low:      price - 18,
			Close:    price,
			Volume:   100 + float64(i%20),
		})
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed klines: %v", err)
	}
}
