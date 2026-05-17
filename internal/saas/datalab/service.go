package datalab

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"quantsaas/internal/quant"
	"quantsaas/internal/saas/marketdata"
	"quantsaas/internal/saas/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var allowedSymbols = map[string]struct{}{
	"BTCUSDT": {},
	"ETHUSDT": {},
}

const importBatchSize = 500

type SyncRequest struct {
	Symbol string `json:"symbol"`
	Limit  int    `json:"limit"`
}

type ImportResult struct {
	Symbol        string `json:"symbol"`
	ProcessedRows int    `json:"processed_rows"`
	FirstOpenTime int64  `json:"first_open_time,omitempty"`
	LastOpenTime  int64  `json:"last_open_time,omitempty"`
}

type SyncResult struct {
	Symbol         string `json:"symbol"`
	RequestedLimit int    `json:"requested_limit"`
	FetchedBars    int    `json:"fetched_bars"`
}

type CoverageItem struct {
	Symbol        string  `json:"symbol"`
	Interval      string  `json:"interval"`
	Count         int64   `json:"count"`
	FirstOpenTime int64   `json:"first_open_time"`
	LastOpenTime  int64   `json:"last_open_time"`
	LastClose     float64 `json:"last_close"`
}

type Service struct {
	db         *gorm.DB
	marketData *marketdata.Service
}

func NewService(db *gorm.DB, marketData *marketdata.Service) *Service {
	return &Service{
		db:         db,
		marketData: marketData,
	}
}

func AllowedSymbols() []string {
	return []string{"BTCUSDT", "ETHUSDT"}
}

func (s *Service) Sync(ctx context.Context, req SyncRequest) (SyncResult, error) {
	symbol, err := normalizeSymbol(req.Symbol)
	if err != nil {
		return SyncResult{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 600
	}
	fetched, err := s.marketData.SyncRecent(ctx, symbol, limit)
	if err != nil {
		return SyncResult{}, err
	}
	return SyncResult{
		Symbol:         symbol,
		RequestedLimit: limit,
		FetchedBars:    fetched,
	}, nil
}

func (s *Service) ImportCSV(ctx context.Context, symbol string, reader io.Reader) (ImportResult, error) {
	symbol, err := normalizeSymbol(symbol)
	if err != nil {
		return ImportResult{}, err
	}

	rows, err := parseCSV(reader)
	if err != nil {
		return ImportResult{}, err
	}
	if len(rows) == 0 {
		return ImportResult{}, errors.New("csv contains no valid rows")
	}

	kLines := make([]store.KLine, 0, len(rows))
	for _, row := range rows {
		kLines = append(kLines, store.KLine{
			Symbol:   symbol,
			Interval: "1h",
			OpenTime: row.OpenTime,
			Open:     row.Open,
			High:     row.High,
			Low:      row.Low,
			Close:    row.Close,
			Volume:   row.Volume,
		})
	}

	if err := s.db.WithContext(ctx).
		Clauses(klineUpsertClause()).
		CreateInBatches(kLines, importBatchSize).Error; err != nil {
		return ImportResult{}, err
	}

	return ImportResult{
		Symbol:        symbol,
		ProcessedRows: len(kLines),
		FirstOpenTime: kLines[0].OpenTime,
		LastOpenTime:  kLines[len(kLines)-1].OpenTime,
	}, nil
}

func klineUpsertClause() clause.OnConflict {
	return clause.OnConflict{
		Columns: []clause.Column{
			{Name: "symbol"},
			{Name: "interval"},
			{Name: "open_time"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"open", "high", "low", "close", "volume", "updated_at"}),
	}
}

func (s *Service) Coverage(ctx context.Context, symbol string) ([]CoverageItem, error) {
	query := s.db.WithContext(ctx).Model(&store.KLine{}).Where("interval = ?", "1h")
	if strings.TrimSpace(symbol) != "" {
		normalized, err := normalizeSymbol(symbol)
		if err != nil {
			return nil, err
		}
		query = query.Where("symbol = ?", normalized)
	}

	type row struct {
		Symbol        string
		Interval      string
		Count         int64
		FirstOpenTime int64
		LastOpenTime  int64
	}
	var rows []row
	if err := query.
		Select("symbol, interval, count(*) as count, min(open_time) as first_open_time, max(open_time) as last_open_time").
		Group("symbol, interval").
		Order("symbol asc").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]CoverageItem, 0, len(rows))
	for _, item := range rows {
		var last store.KLine
		if err := s.db.WithContext(ctx).
			Where("symbol = ? AND interval = ?", item.Symbol, item.Interval).
			Order("open_time DESC").
			Take(&last).Error; err != nil {
			return nil, err
		}
		items = append(items, CoverageItem{
			Symbol:        item.Symbol,
			Interval:      item.Interval,
			Count:         item.Count,
			FirstOpenTime: item.FirstOpenTime,
			LastOpenTime:  item.LastOpenTime,
			LastClose:     last.Close,
		})
	}
	return items, nil
}

func (s *Service) Recent(ctx context.Context, symbol string, limit int) ([]quant.Bar, error) {
	normalized, err := normalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 24
	}
	var rows []store.KLine
	if err := s.db.WithContext(ctx).
		Where("symbol = ? AND interval = ?", normalized, "1h").
		Order("open_time DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	slices.Reverse(rows)

	out := make([]quant.Bar, 0, len(rows))
	for _, row := range rows {
		out = append(out, quant.Bar{
			OpenTime: row.OpenTime,
			Open:     row.Open,
			High:     row.High,
			Low:      row.Low,
			Close:    row.Close,
			Volume:   row.Volume,
		})
	}
	return out, nil
}

func normalizeSymbol(symbol string) (string, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if _, ok := allowedSymbols[symbol]; !ok {
		return "", fmt.Errorf("unsupported symbol %q", symbol)
	}
	return symbol, nil
}

type csvRow struct {
	OpenTime int64
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   float64
}

func parseCSV(reader io.Reader) ([]csvRow, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	header := make(map[string]int)
	start := 0
	if looksLikeHeader(records[0]) {
		for idx, column := range records[0] {
			header[strings.ToLower(strings.TrimSpace(column))] = idx
		}
		start = 1
	}

	byTimestamp := make(map[int64]csvRow, len(records))
	for _, record := range records[start:] {
		if len(record) == 0 {
			continue
		}

		row, err := parseRecord(record, header)
		if err != nil {
			return nil, err
		}
		byTimestamp[row.OpenTime] = row
	}

	rows := make([]csvRow, 0, len(byTimestamp))
	for _, row := range byTimestamp {
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b csvRow) int {
		switch {
		case a.OpenTime < b.OpenTime:
			return -1
		case a.OpenTime > b.OpenTime:
			return 1
		default:
			return 0
		}
	})
	return rows, nil
}

func looksLikeHeader(record []string) bool {
	if len(record) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(record[0]))
	return strings.Contains(first, "time") || first == "open_time" || first == "timestamp"
}

func parseRecord(record []string, header map[string]int) (csvRow, error) {
	readColumn := func(keys ...string) (string, error) {
		for _, key := range keys {
			if idx, ok := header[key]; ok && idx < len(record) {
				return strings.TrimSpace(record[idx]), nil
			}
		}
		return "", fmt.Errorf("missing column %v", keys)
	}

	if len(header) == 0 {
		if len(record) < 6 {
			return csvRow{}, errors.New("csv rows must have at least 6 columns")
		}
		return buildRow(record[0], record[1], record[2], record[3], record[4], record[5])
	}

	openTime, err := readColumn("open_time", "timestamp", "time")
	if err != nil {
		return csvRow{}, err
	}
	openValue, err := readColumn("open")
	if err != nil {
		return csvRow{}, err
	}
	highValue, err := readColumn("high")
	if err != nil {
		return csvRow{}, err
	}
	lowValue, err := readColumn("low")
	if err != nil {
		return csvRow{}, err
	}
	closeValue, err := readColumn("close")
	if err != nil {
		return csvRow{}, err
	}
	volumeValue, err := readColumn("volume", "vol")
	if err != nil {
		return csvRow{}, err
	}
	return buildRow(openTime, openValue, highValue, lowValue, closeValue, volumeValue)
}

func buildRow(openTimeValue, openValue, highValue, lowValue, closeValue, volumeValue string) (csvRow, error) {
	openTime, err := parseTimestamp(openTimeValue)
	if err != nil {
		return csvRow{}, err
	}
	openFloat, err := strconv.ParseFloat(strings.TrimSpace(openValue), 64)
	if err != nil {
		return csvRow{}, err
	}
	highFloat, err := strconv.ParseFloat(strings.TrimSpace(highValue), 64)
	if err != nil {
		return csvRow{}, err
	}
	lowFloat, err := strconv.ParseFloat(strings.TrimSpace(lowValue), 64)
	if err != nil {
		return csvRow{}, err
	}
	closeFloat, err := strconv.ParseFloat(strings.TrimSpace(closeValue), 64)
	if err != nil {
		return csvRow{}, err
	}
	volumeFloat, err := strconv.ParseFloat(strings.TrimSpace(volumeValue), 64)
	if err != nil {
		return csvRow{}, err
	}
	return csvRow{
		OpenTime: openTime,
		Open:     openFloat,
		High:     highFloat,
		Low:      lowFloat,
		Close:    closeFloat,
		Volume:   volumeFloat,
	}, nil
}

func parseTimestamp(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("empty timestamp")
	}

	if unixValue, err := strconv.ParseInt(value, 10, 64); err == nil {
		switch {
		case unixValue > 1_000_000_000_000:
			return unixValue, nil
		case unixValue > 1_000_000_000:
			return unixValue * 1000, nil
		}
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().UnixMilli(), nil
		}
	}
	return 0, fmt.Errorf("unsupported timestamp %q", value)
}
