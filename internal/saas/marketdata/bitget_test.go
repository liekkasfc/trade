package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"quantsaas/internal/saas/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSyncRecentBatchesBitgetRequests(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(store.AllModels()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	var requestedLimits []int
	var requestedEndTimes []int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/spot/market/candles", "/api/v2/spot/market/history-candles":
		default:
			http.NotFound(w, r)
			return
		}

		query := r.URL.Query()
		limit, _ := strconv.Atoi(query.Get("limit"))
		endTime, _ := strconv.ParseInt(query.Get("endTime"), 10, 64)
		requestedLimits = append(requestedLimits, limit)
		requestedEndTimes = append(requestedEndTimes, endTime)

		data := candleBatchFor(r.URL.Path, query)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "00000",
			"msg":  "success",
			"data": data,
		})
	}))
	defer server.Close()

	originalBaseURL := bitgetBaseURL
	bitgetBaseURL = server.URL
	defer func() {
		bitgetBaseURL = originalBaseURL
	}()
	originalMaxRecentBatchLimit := maxRecentBatchLimit
	maxRecentBatchLimit = 3
	defer func() {
		maxRecentBatchLimit = originalMaxRecentBatchLimit
	}()
	originalMaxHistoryBatchLimit := maxHistoryBatchLimit
	maxHistoryBatchLimit = 2
	defer func() {
		maxHistoryBatchLimit = originalMaxHistoryBatchLimit
	}()

	service := NewService(db)
	fetched, err := service.SyncRecent(context.Background(), "BTCUSDT", 5)
	if err != nil {
		t.Fatalf("sync recent: %v", err)
	}
	if fetched != 5 {
		t.Fatalf("expected fetched=5, got %d", fetched)
	}

	if len(requestedLimits) != 3 {
		t.Fatalf("expected 3 batched requests, got %d", len(requestedLimits))
	}
	if requestedLimits[0] != 3 || requestedLimits[1] != 2 || requestedLimits[2] != 1 {
		t.Fatalf("unexpected request limits: %+v", requestedLimits)
	}
	if !(requestedEndTimes[1] < requestedEndTimes[0] && requestedEndTimes[2] < requestedEndTimes[1]) {
		t.Fatalf("expected endTime to move backward across requests: %+v", requestedEndTimes)
	}

	var rows []store.KLine
	if err := db.Order("open_time ASC").Find(&rows).Error; err != nil {
		t.Fatalf("load klines: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("expected 5 persisted bars, got %d", len(rows))
	}
	if rows[0].OpenTime != 1000 || rows[len(rows)-1].OpenTime != 5000 {
		t.Fatalf("unexpected persisted range: first=%d last=%d", rows[0].OpenTime, rows[len(rows)-1].OpenTime)
	}
}

func TestSyncRecentPersistsLargeResultSet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(store.AllModels()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	const requested = 1200
	now := time.Now().UTC().Truncate(time.Hour).UnixMilli()
	historyRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/spot/market/candles":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": "00000",
				"msg":  "success",
				"data": buildDescendingCandles(now-1000*3600_000, now-1*3600_000),
			})
		case "/api/v2/spot/market/history-candles":
			historyRequests++
			switch historyRequests {
			case 1:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": "00000",
					"msg":  "success",
					"data": buildAscendingCandles(now-1200*3600_000, now-1001*3600_000),
				})
			default:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": "00000",
					"msg":  "success",
					"data": [][]string{},
				})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalBaseURL := bitgetBaseURL
	bitgetBaseURL = server.URL
	defer func() {
		bitgetBaseURL = originalBaseURL
	}()

	service := NewService(db)
	fetched, err := service.SyncRecent(context.Background(), "ETHUSDT", requested)
	if err != nil {
		t.Fatalf("sync recent: %v", err)
	}
	if fetched != requested {
		t.Fatalf("expected fetched=%d, got %d", requested, fetched)
	}

	var count int64
	if err := db.Model(&store.KLine{}).Where("symbol = ? AND interval = ?", "ETHUSDT", "1h").Count(&count).Error; err != nil {
		t.Fatalf("count klines: %v", err)
	}
	if count != requested {
		t.Fatalf("expected persisted=%d, got %d", requested, count)
	}
}

func candleBatchFor(path string, query url.Values) [][]string {
	endTime, _ := strconv.ParseInt(query.Get("endTime"), 10, 64)

	switch path {
	case "/api/v2/spot/market/candles":
		return buildCandles(4, 5)
	case "/api/v2/spot/market/history-candles":
		switch {
		case endTime > 3000:
			return buildCandles(2, 3)
		case endTime > 0:
			return buildCandles(1, 1)
		default:
			return nil
		}
	default:
		return nil
	}
}

func buildCandles(start, end int) [][]string {
	rows := make([][]string, 0, end-start+1)
	for i := start; i <= end; i++ {
		openTime := int64(i * 1000)
		price := strconv.Itoa(100 + i)
		rows = append(rows, []string{strconv.FormatInt(openTime, 10), price, price, price, price, "10"})
	}
	return rows
}

func buildAscendingCandles(startOpenTime, endOpenTime int64) [][]string {
	rows := make([][]string, 0, int((endOpenTime-startOpenTime)/3600_000)+1)
	for openTime := startOpenTime; openTime <= endOpenTime; openTime += 3600_000 {
		rows = append(rows, candleRow(openTime))
	}
	return rows
}

func buildDescendingCandles(startOpenTime, endOpenTime int64) [][]string {
	rows := make([][]string, 0, int((endOpenTime-startOpenTime)/3600_000)+1)
	for openTime := endOpenTime; openTime >= startOpenTime; openTime -= 3600_000 {
		rows = append(rows, candleRow(openTime))
	}
	return rows
}

func candleRow(openTime int64) []string {
	price := strconv.FormatFloat(1000+float64((openTime/3600_000)%1000), 'f', 2, 64)
	return []string{strconv.FormatInt(openTime, 10), price, price, price, price, "10"}
}
