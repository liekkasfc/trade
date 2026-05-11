package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	agentconfig "quantsaas/internal/agent/config"
	"quantsaas/internal/protocol"

	"go.uber.org/zap"
)

const bitgetBaseURL = "https://api.bitget.com"

type Executor interface {
	PlaceOrder(ctx context.Context, cmd protocol.TradeCommand) (protocol.Execution, error)
	GetBalances(ctx context.Context) ([]protocol.Balance, error)
}

type BitgetClient struct {
	apiKey     string
	secretKey  string
	passphrase string
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger

	metaMu     sync.Mutex
	symbolMeta map[string]symbolMeta
}

type symbolMeta struct {
	quantityPrecision int
	quotePrecision    int
}

func NewBitgetClient(cfg agentconfig.ExchangeConfig, logger *zap.Logger) (*BitgetClient, error) {
	if strings.TrimSpace(strings.ToLower(cfg.Name)) != "bitget" {
		return nil, fmt.Errorf("unsupported exchange %q", cfg.Name)
	}
	if cfg.Sandbox {
		return nil, errors.New("bitget sandbox mode is not supported in v1")
	}

	return &BitgetClient{
		apiKey:     cfg.APIKey,
		secretKey:  cfg.SecretKey,
		passphrase: cfg.Passphrase,
		baseURL:    bitgetBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		logger:     logger,
		symbolMeta: map[string]symbolMeta{},
	}, nil
}

func (c *BitgetClient) PlaceOrder(ctx context.Context, cmd protocol.TradeCommand) (protocol.Execution, error) {
	meta, err := c.getSymbolMeta(ctx, cmd.Symbol)
	if err != nil {
		return protocol.Execution{}, err
	}

	body := map[string]string{
		"symbol":    cmd.Symbol,
		"side":      strings.ToLower(cmd.Action),
		"orderType": "market",
		"clientOid": cmd.ClientOrderID,
		"force":     "gtc",
	}
	switch cmd.Action {
	case "BUY":
		body["size"] = formatDecimal(cmd.AmountUSDT, meta.quotePrecision)
	case "SELL":
		body["size"] = formatDecimal(cmd.QtyAsset, meta.quantityPrecision)
	default:
		return protocol.Execution{}, fmt.Errorf("unsupported action %q", cmd.Action)
	}

	var placed struct {
		OrderID   string `json:"orderId"`
		ClientOid string `json:"clientOid"`
	}
	if err := c.signedRequest(ctx, http.MethodPost, "/api/v2/spot/trade/place-order", nil, body, &placed); err != nil {
		return protocol.Execution{}, err
	}

	execution, err := c.waitOrderInfo(ctx, cmd.Symbol, placed.OrderID)
	if err != nil {
		c.logger.Warn("bitget order info poll failed", zap.String("order_id", placed.OrderID), zap.Error(err))
		raw, _ := json.Marshal(map[string]any{
			"order_id":   placed.OrderID,
			"client_oid": placed.ClientOid,
		})
		return protocol.Execution{
			ExchangeOrderID: placed.OrderID,
			Status:          "submitted",
			Raw:             raw,
		}, nil
	}
	return execution, nil
}

func (c *BitgetClient) GetBalances(ctx context.Context) ([]protocol.Balance, error) {
	var response []struct {
		Coin      string `json:"coin"`
		Available string `json:"available"`
		Frozen    string `json:"frozen"`
		Locked    string `json:"locked"`
	}
	if err := c.signedRequest(ctx, http.MethodGet, "/api/v2/spot/account/assets", url.Values{"assetType": []string{"hold_only"}}, nil, &response); err != nil {
		return nil, err
	}

	balances := make([]protocol.Balance, 0, len(response))
	for _, item := range response {
		available, _ := strconv.ParseFloat(item.Available, 64)
		frozen, _ := strconv.ParseFloat(item.Frozen, 64)
		locked, _ := strconv.ParseFloat(item.Locked, 64)
		balances = append(balances, protocol.Balance{
			Asset:     item.Coin,
			Available: available,
			Frozen:    frozen + locked,
		})
	}
	return balances, nil
}

func (c *BitgetClient) waitOrderInfo(ctx context.Context, symbol, orderID string) (protocol.Execution, error) {
	type orderInfo struct {
		OrderID     string          `json:"orderId"`
		Status      string          `json:"status"`
		BaseVolume  string          `json:"baseVolume"`
		QuoteVolume string          `json:"quoteVolume"`
		PriceAvg    string          `json:"priceAvg"`
		TotalFee    string          `json:"totalFee"`
		FeeDetail   json.RawMessage `json:"feeDetail"`
		NewFees     json.RawMessage `json:"newFees"`
		FillFee     string          `json:"fillFee"`
		FeeCoin     string          `json:"feeCoin"`
	}

	var last orderInfo
	for attempt := 0; attempt < 10; attempt++ {
		select {
		case <-ctx.Done():
			return protocol.Execution{}, ctx.Err()
		default:
		}

		var info orderInfo
		query := url.Values{
			"symbol":  []string{symbol},
			"orderId": []string{orderID},
		}
		if err := c.signedRequest(ctx, http.MethodGet, "/api/v2/spot/trade/orderInfo", query, nil, &info); err != nil {
			return protocol.Execution{}, err
		}
		last = info
		if isFinalOrderStatus(info.Status) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	filledQty, _ := strconv.ParseFloat(last.BaseVolume, 64)
	quoteAmount, _ := strconv.ParseFloat(last.QuoteVolume, 64)
	filledPrice, _ := strconv.ParseFloat(last.PriceAvg, 64)
	fee, feeAsset := parseFee(last)
	raw, _ := json.Marshal(last)
	return protocol.Execution{
		ExchangeOrderID: last.OrderID,
		Status:          strings.ToLower(last.Status),
		FilledQty:       filledQty,
		FilledPrice:     filledPrice,
		QuoteAmount:     quoteAmount,
		Fee:             fee,
		FeeAsset:        feeAsset,
		Raw:             raw,
	}, nil
}

func (c *BitgetClient) getSymbolMeta(ctx context.Context, symbol string) (symbolMeta, error) {
	c.metaMu.Lock()
	if meta, ok := c.symbolMeta[symbol]; ok {
		c.metaMu.Unlock()
		return meta, nil
	}
	c.metaMu.Unlock()

	var response []struct {
		Symbol            string `json:"symbol"`
		QuantityPrecision string `json:"quantityPrecision"`
		QuotePrecision    string `json:"quotePrecision"`
	}
	query := url.Values{"symbol": []string{symbol}}
	if err := c.publicRequest(ctx, http.MethodGet, "/api/v2/spot/public/symbols", query, &response); err != nil {
		return symbolMeta{}, err
	}
	if len(response) == 0 {
		return symbolMeta{}, fmt.Errorf("symbol %s not found on bitget", symbol)
	}

	quantityPrecision, _ := strconv.Atoi(response[0].QuantityPrecision)
	quotePrecision, _ := strconv.Atoi(response[0].QuotePrecision)
	meta := symbolMeta{
		quantityPrecision: quantityPrecision,
		quotePrecision:    quotePrecision,
	}

	c.metaMu.Lock()
	c.symbolMeta[symbol] = meta
	c.metaMu.Unlock()
	return meta, nil
}

func (c *BitgetClient) publicRequest(ctx context.Context, method, path string, query url.Values, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return decodeBitgetResponse(resp, out)
}

func (c *BitgetClient) signedRequest(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var bodyBytes []byte
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyBytes = raw
	}

	requestPath := path
	if len(query) > 0 {
		requestPath += "?" + query.Encode()
	}

	timestamp := strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
	preHash := timestamp + strings.ToUpper(method) + requestPath + string(bodyBytes)
	signature := signBitget(c.secretKey, preHash)

	endpoint := c.baseURL + requestPath
	req, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}
	req.Header.Set("ACCESS-KEY", c.apiKey)
	req.Header.Set("ACCESS-SIGN", signature)
	req.Header.Set("ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("ACCESS-PASSPHRASE", c.passphrase)
	req.Header.Set("locale", "en-US")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return decodeBitgetResponse(resp, out)
}

func decodeBitgetResponse(resp *http.Response, out any) error {
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("bitget status %d: %s", resp.StatusCode, string(body))
	}

	var envelope struct {
		Code string          `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Code != "00000" {
		return fmt.Errorf("bitget error %s: %s", envelope.Code, envelope.Msg)
	}
	if out == nil || len(envelope.Data) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Data, out)
}

func signBitget(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func formatDecimal(v float64, precision int) string {
	if precision < 0 {
		precision = 8
	}
	scale := math.Pow10(precision)
	rounded := math.Floor(v*scale) / scale
	text := strconv.FormatFloat(rounded, 'f', precision, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" {
		return "0"
	}
	return text
}

func isFinalOrderStatus(status string) bool {
	switch strings.ToLower(status) {
	case "full_fill", "filled", "cancelled", "canceled", "rejected":
		return true
	default:
		return false
	}
}

func parseFee(info struct {
	OrderID     string          `json:"orderId"`
	Status      string          `json:"status"`
	BaseVolume  string          `json:"baseVolume"`
	QuoteVolume string          `json:"quoteVolume"`
	PriceAvg    string          `json:"priceAvg"`
	TotalFee    string          `json:"totalFee"`
	FeeDetail   json.RawMessage `json:"feeDetail"`
	NewFees     json.RawMessage `json:"newFees"`
	FillFee     string          `json:"fillFee"`
	FeeCoin     string          `json:"feeCoin"`
}) (float64, string) {
	if info.TotalFee != "" {
		if fee, err := strconv.ParseFloat(info.TotalFee, 64); err == nil {
			return math.Abs(fee), info.FeeCoin
		}
	}
	if info.FillFee != "" {
		if fee, err := strconv.ParseFloat(info.FillFee, 64); err == nil {
			return math.Abs(fee), info.FeeCoin
		}
	}
	type feeNode struct {
		TotalFee string `json:"totalFee"`
		FeeCoin  string `json:"feeCoin"`
	}
	for _, raw := range []json.RawMessage{info.NewFees, info.FeeDetail} {
		if len(raw) == 0 {
			continue
		}
		var node feeNode
		if err := json.Unmarshal(raw, &node); err == nil && node.TotalFee != "" {
			fee, _ := strconv.ParseFloat(node.TotalFee, 64)
			return math.Abs(fee), node.FeeCoin
		}
		var arr []feeNode
		if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 && arr[0].TotalFee != "" {
			fee, _ := strconv.ParseFloat(arr[0].TotalFee, 64)
			return math.Abs(fee), arr[0].FeeCoin
		}
	}
	return 0, info.FeeCoin
}
