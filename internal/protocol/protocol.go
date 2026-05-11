package protocol

import "encoding/json"

const (
	MessageTypeAuth         = "auth"
	MessageTypeAuthResult   = "auth_result"
	MessageTypeHeartbeat    = "heartbeat"
	MessageTypeHeartbeatAck = "heartbeat_ack"
	MessageTypeCommand      = "command"
	MessageTypeCommandAck   = "command_ack"
	MessageTypeDeltaReport  = "delta_report"
	MessageTypeReportAck    = "report_ack"
)

type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func Wrap(messageType string, payload any) (Envelope, error) {
	if payload == nil {
		return Envelope{Type: messageType}, nil
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		Type:    messageType,
		Payload: raw,
	}, nil
}

type AuthPayload struct {
	Token   string `json:"token"`
	Version string `json:"version,omitempty"`
}

type AuthResult struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	UserID     uint   `json:"user_id,omitempty"`
	ServerTime int64  `json:"server_time,omitempty"`
}

type Heartbeat struct {
	SentAt int64 `json:"sent_at"`
}

type HeartbeatAck struct {
	AckedAt int64 `json:"acked_at"`
}

type TradeCommand struct {
	ClientOrderID      string  `json:"client_order_id"`
	StrategyInstanceID uint    `json:"strategy_instance_id"`
	Action             string  `json:"action"`
	Engine             string  `json:"engine"`
	Symbol             string  `json:"symbol"`
	AmountUSDT         float64 `json:"amount_usdt,omitempty"`
	QtyAsset           float64 `json:"qty_asset,omitempty"`
	LotType            string  `json:"lot_type"`
	Reason             string  `json:"reason,omitempty"`
	SentAt             int64   `json:"sent_at"`
}

type CommandAck struct {
	ClientOrderID string `json:"client_order_id"`
	AckedAt       int64  `json:"acked_at"`
}

type Balance struct {
	Asset     string  `json:"asset"`
	Available float64 `json:"available"`
	Frozen    float64 `json:"frozen"`
}

func (b Balance) Total() float64 {
	return b.Available + b.Frozen
}

type Execution struct {
	ExchangeOrderID string          `json:"exchange_order_id,omitempty"`
	Status          string          `json:"status"`
	FilledQty       float64         `json:"filled_qty,omitempty"`
	FilledPrice     float64         `json:"filled_price,omitempty"`
	QuoteAmount     float64         `json:"quote_amount,omitempty"`
	Fee             float64         `json:"fee,omitempty"`
	FeeAsset        string          `json:"fee_asset,omitempty"`
	Raw             json.RawMessage `json:"raw,omitempty"`
}

type DeltaReport struct {
	ClientOrderID string     `json:"client_order_id,omitempty"`
	Symbol        string     `json:"symbol,omitempty"`
	Balances      []Balance  `json:"balances,omitempty"`
	Execution     *Execution `json:"execution,omitempty"`
	ReportedAt    int64      `json:"reported_at"`
}

type ReportAck struct {
	ClientOrderID string `json:"client_order_id,omitempty"`
	AckedAt       int64  `json:"acked_at"`
}

type AgentStatus struct {
	Connected       bool   `json:"connected"`
	Version         string `json:"version,omitempty"`
	ConnectedAt     int64  `json:"connected_at,omitempty"`
	LastHeartbeatAt int64  `json:"last_heartbeat_at,omitempty"`
}
