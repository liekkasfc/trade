package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	agentconfig "quantsaas/internal/agent/config"
	"quantsaas/internal/agent/exchange"
	"quantsaas/internal/protocol"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const agentVersion = "local-agent-v1"

type Client struct {
	cfg      agentconfig.Config
	exchange exchange.Executor
	logger   *zap.Logger

	httpClient *http.Client
	dialer     *websocket.Dialer
}

type session struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func NewClient(cfg agentconfig.Config, executor exchange.Executor, logger *zap.Logger) *Client {
	return &Client{
		cfg:      cfg,
		exchange: executor,
		logger:   logger,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		dialer: websocket.DefaultDialer,
	}
}

func (c *Client) Run(ctx context.Context) error {
	backoff := time.Second

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := c.runOnce(ctx); err != nil {
			c.logger.Warn("agent connection loop failed", zap.Error(err), zap.Duration("retry_in", backoff))
		} else {
			backoff = time.Second
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > 5*time.Minute {
			backoff = 5 * time.Minute
		}
	}
}

func (c *Client) runOnce(ctx context.Context) error {
	token, err := c.login(ctx)
	if err != nil {
		return err
	}

	wsURL, err := buildWSURL(c.cfg.SaaSURL)
	if err != nil {
		return err
	}

	conn, _, err := c.dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	sess := &session{conn: conn}
	if err := c.send(sess, protocol.MessageTypeAuth, protocol.AuthPayload{
		Token:   token,
		Version: agentVersion,
	}); err != nil {
		return err
	}

	authEnv, err := c.readEnvelope(sess.conn)
	if err != nil {
		return err
	}
	if authEnv.Type != protocol.MessageTypeAuthResult {
		return fmt.Errorf("expected auth_result, got %s", authEnv.Type)
	}
	var authResult protocol.AuthResult
	if err := json.Unmarshal(authEnv.Payload, &authResult); err != nil {
		return err
	}
	if !authResult.OK {
		return fmt.Errorf("agent auth rejected: %s", authResult.Error)
	}

	if err := c.sendBalanceSnapshot(ctx, sess, ""); err != nil {
		c.logger.Warn("send initial balance snapshot", zap.Error(err))
	}

	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.readLoop(ctx, sess)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case <-heartbeatTicker.C:
			if err := c.send(sess, protocol.MessageTypeHeartbeat, protocol.Heartbeat{
				SentAt: time.Now().UTC().UnixMilli(),
			}); err != nil {
				return err
			}
		}
	}
}

func (c *Client) readLoop(ctx context.Context, sess *session) error {
	for {
		env, err := c.readEnvelope(sess.conn)
		if err != nil {
			return err
		}

		switch env.Type {
		case protocol.MessageTypeHeartbeatAck, protocol.MessageTypeReportAck, protocol.MessageTypeAuthResult:
			continue
		case protocol.MessageTypeCommand:
			var cmd protocol.TradeCommand
			if err := json.Unmarshal(env.Payload, &cmd); err != nil {
				c.logger.Warn("decode trade command", zap.Error(err))
				continue
			}
			if err := c.send(sess, protocol.MessageTypeCommandAck, protocol.CommandAck{
				ClientOrderID: cmd.ClientOrderID,
				AckedAt:       time.Now().UTC().UnixMilli(),
			}); err != nil {
				return err
			}
			go c.executeCommand(ctx, sess, cmd)
		}
	}
}

func (c *Client) executeCommand(ctx context.Context, sess *session, cmd protocol.TradeCommand) {
	execution, err := c.exchange.PlaceOrder(ctx, cmd)
	if err != nil {
		raw, _ := json.Marshal(map[string]any{"error": err.Error()})
		execution = protocol.Execution{
			Status: "error",
			Raw:    raw,
		}
		c.logger.Warn("execute trade command", zap.String("client_order_id", cmd.ClientOrderID), zap.Error(err))
	}

	balances, balanceErr := c.exchange.GetBalances(ctx)
	if balanceErr != nil {
		c.logger.Warn("refresh balances after execution", zap.Error(balanceErr))
	}

	if err := c.send(sess, protocol.MessageTypeDeltaReport, protocol.DeltaReport{
		ClientOrderID: cmd.ClientOrderID,
		Symbol:        cmd.Symbol,
		Balances:      balances,
		Execution:     &execution,
		ReportedAt:    time.Now().UTC().UnixMilli(),
	}); err != nil {
		c.logger.Warn("send delta report", zap.String("client_order_id", cmd.ClientOrderID), zap.Error(err))
	}
}

func (c *Client) sendBalanceSnapshot(ctx context.Context, sess *session, symbol string) error {
	balances, err := c.exchange.GetBalances(ctx)
	if err != nil {
		return err
	}
	return c.send(sess, protocol.MessageTypeDeltaReport, protocol.DeltaReport{
		ClientOrderID: "",
		Symbol:        symbol,
		Balances:      balances,
		ReportedAt:    time.Now().UTC().UnixMilli(),
	})
}

func (c *Client) send(sess *session, messageType string, payload any) error {
	env, err := protocol.Wrap(messageType, payload)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}

	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()

	if err := sess.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return sess.conn.WriteMessage(websocket.TextMessage, raw)
}

func (c *Client) readEnvelope(conn *websocket.Conn) (protocol.Envelope, error) {
	_, payload, err := conn.ReadMessage()
	if err != nil {
		return protocol.Envelope{}, err
	}
	var env protocol.Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return protocol.Envelope{}, err
	}
	return env, nil
}

func (c *Client) login(ctx context.Context) (string, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"email":    c.cfg.Email,
		"password": c.cfg.Password,
	})

	endpoint := strings.TrimRight(c.cfg.SaaSURL, "/") + "/api/v1/auth/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload struct {
		Token string `json:"token"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("login failed: %s", payload.Error)
	}
	if payload.Token == "" {
		return "", fmt.Errorf("login response missing token")
	}
	return payload.Token, nil
}

func buildWSURL(base string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported saas_url scheme %q", parsed.Scheme)
	}
	parsed.Path = "/ws/agent"
	parsed.RawQuery = ""
	return parsed.String(), nil
}
