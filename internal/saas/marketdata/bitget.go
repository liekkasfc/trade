package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"quantsaas/internal/quant"
	"quantsaas/internal/saas/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultLimit      = 600
	recentBatchLimit  = 1000
	historyBatchLimit = 200
	upsertBatchSize   = 500
)

var bitgetBaseURL = "https://api.bitget.com"
var maxRecentBatchLimit = recentBatchLimit
var maxHistoryBatchLimit = historyBatchLimit

type Service struct {
	db         *gorm.DB
	httpClient *http.Client
}

func NewService(db *gorm.DB) *Service {
	return &Service{
		db: db,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *Service) LoadRecent(ctx context.Context, symbol string, limit int) ([]quant.Bar, error) {
	if limit <= 0 {
		limit = defaultLimit
	}

	_, syncErr := s.SyncRecent(ctx, symbol, limit)
	bars, dbErr := s.loadFromDB(ctx, symbol, limit)
	if dbErr == nil && len(bars) > 0 {
		return bars, nil
	}
	if syncErr != nil {
		return nil, syncErr
	}
	return nil, dbErr
}

func (s *Service) LatestClose(ctx context.Context, symbol string) float64 {
	bars, err := s.LoadRecent(ctx, symbol, 1)
	if err != nil || len(bars) == 0 {
		return 0
	}
	return bars[len(bars)-1].Close
}

func (s *Service) SyncRecent(ctx context.Context, symbol string, limit int) (int, error) {
	bars, err := s.fetchCandles(ctx, symbol, limit)
	if err != nil {
		return 0, err
	}
	if len(bars) == 0 {
		return 0, fmt.Errorf("bitget returned no completed candles for %s", symbol)
	}

	rows := make([]store.KLine, 0, len(bars))
	for _, bar := range bars {
		rows = append(rows, store.KLine{
			Symbol:   symbol,
			Interval: "1h",
			OpenTime: bar.OpenTime,
			Open:     bar.Open,
			High:     bar.High,
			Low:      bar.Low,
			Close:    bar.Close,
			Volume:   bar.Volume,
		})
	}

	if err := s.db.WithContext(ctx).
		Clauses(klineUpsertClause()).
		CreateInBatches(rows, upsertBatchSize).Error; err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (s *Service) fetchCandles(ctx context.Context, symbol string, limit int) ([]quant.Bar, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit <= maxRecentBatchLimit {
		return s.fetchRecentCandlesWindow(ctx, symbol, limit, time.Now().UTC().UnixMilli())
	}

	seen := make(map[int64]quant.Bar, limit)
	collected := make([]quant.Bar, 0, limit)
	recentBatch, err := s.fetchRecentCandlesWindow(ctx, symbol, maxRecentBatchLimit, time.Now().UTC().UnixMilli())
	if err != nil {
		return nil, err
	}
	if len(recentBatch) == 0 {
		return nil, fmt.Errorf("bitget returned no completed candles for %s", symbol)
	}
	appendUniqueBars(seen, &collected, recentBatch)

	endTime := recentBatch[0].OpenTime - 1
	for len(collected) < limit && endTime > 0 {
		batchSize := limit - len(collected)
		if batchSize > maxHistoryBatchLimit {
			batchSize = maxHistoryBatchLimit
		}

		batch, err := s.fetchHistoryCandlesWindow(ctx, symbol, batchSize, endTime)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		appendUniqueBars(seen, &collected, batch)
		oldestOpenTime := batch[0].OpenTime
		if oldestOpenTime <= 0 {
			break
		}
		endTime = oldestOpenTime - 1
	}

	sort.Slice(collected, func(i, j int) bool {
		return collected[i].OpenTime < collected[j].OpenTime
	})
	return collected, nil
}

func (s *Service) fetchRecentCandlesWindow(ctx context.Context, symbol string, limit int, endTime int64) ([]quant.Bar, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxRecentBatchLimit {
		limit = maxRecentBatchLimit
	}
	if endTime <= 0 {
		endTime = time.Now().UTC().UnixMilli()
	}

	query := url.Values{}
	query.Set("symbol", symbol)
	query.Set("granularity", "1h")
	query.Set("limit", strconv.Itoa(limit))
	query.Set("endTime", strconv.FormatInt(endTime, 10))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bitgetBaseURL+"/api/v2/spot/market/candles?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	return s.executeCandlesRequest(req)
}

func (s *Service) fetchHistoryCandlesWindow(ctx context.Context, symbol string, limit int, endTime int64) ([]quant.Bar, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxHistoryBatchLimit {
		limit = maxHistoryBatchLimit
	}
	if endTime <= 0 {
		endTime = time.Now().UTC().UnixMilli()
	}

	query := url.Values{}
	query.Set("symbol", symbol)
	query.Set("granularity", "1h")
	query.Set("limit", strconv.Itoa(limit))
	query.Set("endTime", strconv.FormatInt(endTime, 10))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bitgetBaseURL+"/api/v2/spot/market/history-candles?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	return s.executeCandlesRequest(req)
}

func (s *Service) executeCandlesRequest(req *http.Request) ([]quant.Bar, error) {

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("bitget candles status %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Code string     `json:"code"`
		Msg  string     `json:"msg"`
		Data [][]string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Code != "00000" {
		return nil, fmt.Errorf("bitget candles error %s: %s", payload.Code, payload.Msg)
	}

	currentHour := time.Now().UTC().Truncate(time.Hour).UnixMilli()
	bars := make([]quant.Bar, 0, len(payload.Data))
	for _, row := range payload.Data {
		if len(row) < 6 {
			continue
		}
		openTime, err := strconv.ParseInt(row[0], 10, 64)
		if err != nil || openTime >= currentHour {
			continue
		}

		openValue, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			return nil, err
		}
		highValue, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			return nil, err
		}
		lowValue, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			return nil, err
		}
		closeValue, err := strconv.ParseFloat(row[4], 64)
		if err != nil {
			return nil, err
		}
		volumeValue, err := strconv.ParseFloat(row[5], 64)
		if err != nil {
			return nil, err
		}

		bars = append(bars, quant.Bar{
			OpenTime: openTime,
			Open:     openValue,
			High:     highValue,
			Low:      lowValue,
			Close:    closeValue,
			Volume:   volumeValue,
		})
	}

	sort.Slice(bars, func(i, j int) bool {
		return bars[i].OpenTime < bars[j].OpenTime
	})
	return bars, nil
}

func appendUniqueBars(seen map[int64]quant.Bar, collected *[]quant.Bar, batch []quant.Bar) {
	for _, bar := range batch {
		if _, ok := seen[bar.OpenTime]; ok {
			continue
		}
		seen[bar.OpenTime] = bar
		*collected = append(*collected, bar)
	}
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

func (s *Service) loadFromDB(ctx context.Context, symbol string, limit int) ([]quant.Bar, error) {
	var rows []store.KLine
	if err := s.db.WithContext(ctx).
		Where("symbol = ? AND interval = ?", symbol, "1h").
		Order("open_time DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no 1h bars found for %s", symbol)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].OpenTime < rows[j].OpenTime
	})

	bars := make([]quant.Bar, 0, len(rows))
	for _, row := range rows {
		bars = append(bars, quant.Bar{
			OpenTime: row.OpenTime,
			Open:     row.Open,
			High:     row.High,
			Low:      row.Low,
			Close:    row.Close,
			Volume:   row.Volume,
		})
	}
	return bars, nil
}
