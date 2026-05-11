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
