package datalab

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"quantsaas/internal/saas/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestImportCSVAndCoverage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(store.AllModels()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	service := NewService(db, nil)
	csvData := strings.NewReader(strings.Join([]string{
		"open_time,open,high,low,close,volume",
		"1715385600000,61000,61200,60500,61100,123.4",
		"1715389200000,61100,61500,61050,61400,140.8",
	}, "\n"))

	result, err := service.ImportCSV(context.Background(), "BTCUSDT", csvData)
	if err != nil {
		t.Fatalf("import csv: %v", err)
	}
	if result.ProcessedRows != 2 {
		t.Fatalf("expected 2 processed rows, got %d", result.ProcessedRows)
	}

	coverage, err := service.Coverage(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if len(coverage) != 1 {
		t.Fatalf("expected 1 coverage item, got %d", len(coverage))
	}
	if coverage[0].Count != 2 {
		t.Fatalf("expected coverage count 2, got %d", coverage[0].Count)
	}

	bars, err := service.Recent(context.Background(), "BTCUSDT", 10)
	if err != nil {
		t.Fatalf("recent bars: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 recent bars, got %d", len(bars))
	}
	if bars[1].Close != 61400 {
		t.Fatalf("expected latest close 61400, got %.2f", bars[1].Close)
	}
}

func TestImportCSVLargeBatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(store.AllModels()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	var builder strings.Builder
	builder.WriteString("open_time,open,high,low,close,volume\n")
	openTime := int64(1715385600000)
	for i := 0; i < 1200; i++ {
		fmt.Fprintf(&builder, "%d,%0.2f,%0.2f,%0.2f,%0.2f,%0.2f\n",
			openTime+int64(i)*3600_000,
			61000.0+float64(i),
			61100.0+float64(i),
			60900.0+float64(i),
			61050.0+float64(i),
			100.0+float64(i)/10.0,
		)
	}

	service := NewService(db, nil)
	result, err := service.ImportCSV(context.Background(), "ETHUSDT", strings.NewReader(builder.String()))
	if err != nil {
		t.Fatalf("import csv: %v", err)
	}
	if result.ProcessedRows != 1200 {
		t.Fatalf("expected 1200 processed rows, got %d", result.ProcessedRows)
	}

	coverage, err := service.Coverage(context.Background(), "ETHUSDT")
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if len(coverage) != 1 || coverage[0].Count != 1200 {
		t.Fatalf("expected coverage count 1200, got %+v", coverage)
	}
}
