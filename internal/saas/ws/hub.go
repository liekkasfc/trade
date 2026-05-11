package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"quantsaas/internal/protocol"
	"quantsaas/internal/saas/auth"
	"quantsaas/internal/saas/marketdata"
	"quantsaas/internal/saas/store"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const defaultAuthTimeout = 10 * time.Second

type Hub struct {
	db          *gorm.DB
	authService *auth.Service
	marketData  *marketdata.Service
	logger      *zap.Logger
	authTimeout time.Duration

	upgrader websocket.Upgrader
	conns    sync.Map
}

type agentConn struct {
	userID          uint
	version         string
	conn            *websocket.Conn
	connectedAt     int64
	lastHeartbeatAt int64
	writeMu         sync.Mutex
}

func NewHub(db *gorm.DB, authService *auth.Service, marketData *marketdata.Service, logger *zap.Logger, authTimeout time.Duration) *Hub {
	if authTimeout <= 0 {
		authTimeout = defaultAuthTimeout
	}
	return &Hub{
		db:          db,
		authService: authService,
		marketData:  marketData,
		logger:      logger,
		authTimeout: authTimeout,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	}
}

func (h *Hub) HandleConnection(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	if err := conn.SetReadDeadline(time.Now().Add(h.authTimeout)); err != nil {
		_ = conn.Close()
		return
	}

	rawConn, err := h.authenticate(conn)
	if err != nil {
		h.logger.Warn("agent websocket auth failed", zap.Error(err))
		_ = conn.Close()
		return
	}
	defer h.unregister(rawConn)

	_ = conn.SetReadDeadline(time.Time{})
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				h.logger.Warn("agent websocket read failed", zap.Uint("user_id", rawConn.userID), zap.Error(err))
			}
			return
		}

		var env protocol.Envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			h.logger.Warn("invalid websocket payload", zap.Uint("user_id", rawConn.userID), zap.Error(err))
			continue
		}

		switch env.Type {
		case protocol.MessageTypeHeartbeat:
			rawConn.lastHeartbeatAt = time.Now().UTC().UnixMilli()
			_ = h.write(rawConn, protocol.MessageTypeHeartbeatAck, protocol.HeartbeatAck{AckedAt: time.Now().UTC().UnixMilli()})
		case protocol.MessageTypeCommandAck:
			var ack protocol.CommandAck
			if err := json.Unmarshal(env.Payload, &ack); err != nil {
				h.logger.Warn("decode command_ack", zap.Uint("user_id", rawConn.userID), zap.Error(err))
				continue
			}
			h.auditCommandAck(rawConn.userID, ack)
		case protocol.MessageTypeDeltaReport:
			var report protocol.DeltaReport
			if err := json.Unmarshal(env.Payload, &report); err != nil {
				h.logger.Warn("decode delta_report", zap.Uint("user_id", rawConn.userID), zap.Error(err))
				continue
			}
			if err := h.processDeltaReport(c.Request.Context(), rawConn.userID, report); err != nil {
				h.logger.Warn("process delta report", zap.Uint("user_id", rawConn.userID), zap.Error(err))
				continue
			}
			_ = h.write(rawConn, protocol.MessageTypeReportAck, protocol.ReportAck{
				ClientOrderID: report.ClientOrderID,
				AckedAt:       time.Now().UTC().UnixMilli(),
			})
		}
	}
}

func (h *Hub) SendToAgent(userID uint, cmd protocol.TradeCommand) error {
	raw, ok := h.conns.Load(userID)
	if !ok {
		return errors.New("agent is offline")
	}
	return h.write(raw.(*agentConn), protocol.MessageTypeCommand, cmd)
}

func (h *Hub) IsConnected(userID uint) bool {
	_, ok := h.conns.Load(userID)
	return ok
}

func (h *Hub) StatusForUser(userID uint) protocol.AgentStatus {
	raw, ok := h.conns.Load(userID)
	if !ok {
		return protocol.AgentStatus{}
	}
	conn := raw.(*agentConn)
	return protocol.AgentStatus{
		Connected:       true,
		Version:         conn.version,
		ConnectedAt:     conn.connectedAt,
		LastHeartbeatAt: conn.lastHeartbeatAt,
	}
}

func (h *Hub) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.conns.Range(func(key, value any) bool {
			h.unregister(value.(*agentConn))
			return true
		})
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (h *Hub) authenticate(conn *websocket.Conn) (*agentConn, error) {
	_, payload, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var env protocol.Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	if env.Type != protocol.MessageTypeAuth {
		_ = h.writeRaw(conn, protocol.MessageTypeAuthResult, protocol.AuthResult{
			OK:         false,
			Error:      "first message must be auth",
			ServerTime: time.Now().UTC().UnixMilli(),
		})
		return nil, errors.New("first message must be auth")
	}

	var authPayload protocol.AuthPayload
	if err := json.Unmarshal(env.Payload, &authPayload); err != nil {
		return nil, err
	}

	claims, err := h.authService.ParseToken(authPayload.Token)
	if err != nil {
		_ = h.writeRaw(conn, protocol.MessageTypeAuthResult, protocol.AuthResult{
			OK:         false,
			Error:      "invalid token",
			ServerTime: time.Now().UTC().UnixMilli(),
		})
		return nil, err
	}
	if _, err := h.authService.LoadUser(context.Background(), claims.UserID); err != nil {
		return nil, err
	}

	agent := &agentConn{
		userID:          claims.UserID,
		version:         authPayload.Version,
		conn:            conn,
		connectedAt:     time.Now().UTC().UnixMilli(),
		lastHeartbeatAt: time.Now().UTC().UnixMilli(),
	}

	if existing, ok := h.conns.Load(claims.UserID); ok {
		h.unregister(existing.(*agentConn))
	}
	h.conns.Store(claims.UserID, agent)

	if err := h.write(agent, protocol.MessageTypeAuthResult, protocol.AuthResult{
		OK:         true,
		UserID:     claims.UserID,
		ServerTime: time.Now().UTC().UnixMilli(),
	}); err != nil {
		h.conns.Delete(claims.UserID)
		return nil, err
	}

	return agent, nil
}

func (h *Hub) unregister(conn *agentConn) {
	if conn == nil {
		return
	}
	if current, ok := h.conns.Load(conn.userID); ok && current == conn {
		h.conns.Delete(conn.userID)
	}
	_ = conn.conn.Close()
}

func (h *Hub) write(conn *agentConn, messageType string, payload any) error {
	env, err := protocol.Wrap(messageType, payload)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}

	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()

	if err := conn.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return conn.conn.WriteMessage(websocket.TextMessage, raw)
}

func (h *Hub) writeRaw(conn *websocket.Conn, messageType string, payload any) error {
	env, err := protocol.Wrap(messageType, payload)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, raw)
}

func (h *Hub) auditCommandAck(userID uint, ack protocol.CommandAck) {
	payload, _ := json.Marshal(ack)
	if err := h.db.WithContext(context.Background()).Create(&store.AuditLog{
		UserID:    &userID,
		EventType: "command_ack",
		Payload:   payload,
	}).Error; err != nil {
		h.logger.Warn("persist command ack", zap.Uint("user_id", userID), zap.Error(err))
	}
}
