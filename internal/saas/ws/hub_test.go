package ws

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"quantsaas/internal/protocol"
	"quantsaas/internal/saas/auth"
	saasconfig "quantsaas/internal/saas/config"
	"quantsaas/internal/saas/store"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHubClosesUnauthenticatedConnectionAfterTimeout(t *testing.T) {
	hub, server := newTestHub(t, 100*time.Millisecond)
	defer server.Close()
	defer func() {
		_ = hub.Shutdown(context.Background())
	}()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	time.Sleep(250 * time.Millisecond)
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatal("expected unauthenticated connection to close")
	}
}

func TestHubAppliesDeltaReportSnapshot(t *testing.T) {
	hub, server := newTestHub(t, time.Second)
	defer server.Close()
	defer func() {
		_ = hub.Shutdown(context.Background())
	}()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := store.User{
		Email:        "user@example.com",
		PasswordHash: string(passwordHash),
		Role:         store.UserRoleUser,
		Plan:         "core",
	}
	if err := hub.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	template := store.StrategyTemplate{
		TemplateKey: "core-btc-v1",
		Name:        "BTC",
		Version:     "v1",
		IsSpot:      true,
	}
	if err := hub.db.Create(&template).Error; err != nil {
		t.Fatalf("create template: %v", err)
	}

	instance := store.StrategyInstance{
		UserID:           user.ID,
		TemplateID:       template.ID,
		Name:             "btc sleeve",
		Symbol:           "BTCUSDT",
		Status:           store.InstanceStatusStopped,
		CapitalQuotaUSDT: 1000,
	}
	if err := hub.db.Create(&instance).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}

	portfolio := store.PortfolioState{
		StrategyInstanceID: instance.ID,
		USDTBalance:        1000,
	}
	if err := hub.db.Create(&portfolio).Error; err != nil {
		t.Fatalf("create portfolio: %v", err)
	}

	token, err := hub.authService.SignToken(user.ID, user.Role)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	writeEnvelope(t, conn, protocol.MessageTypeAuth, protocol.AuthPayload{Token: token, Version: "test"})
	var authResult protocol.AuthResult
	readEnvelope(t, conn, protocol.MessageTypeAuthResult, &authResult)
	if !authResult.OK {
		t.Fatalf("expected auth ok, got %+v", authResult)
	}

	writeEnvelope(t, conn, protocol.MessageTypeDeltaReport, protocol.DeltaReport{
		Balances: []protocol.Balance{
			{Asset: "USDT", Available: 250, Frozen: 0},
			{Asset: "BTC", Available: 1.5, Frozen: 0},
		},
		ReportedAt: time.Now().UTC().UnixMilli(),
	})
	readEnvelope(t, conn, protocol.MessageTypeReportAck, &protocol.ReportAck{})

	var updated store.PortfolioState
	if err := hub.db.Where("strategy_instance_id = ?", instance.ID).Take(&updated).Error; err != nil {
		t.Fatalf("load updated portfolio: %v", err)
	}
	if updated.USDTBalance != 250 {
		t.Fatalf("expected usdt balance 250, got %.2f", updated.USDTBalance)
	}
	if updated.FloatBTC != 1.5 {
		t.Fatalf("expected float btc 1.5, got %.8f", updated.FloatBTC)
	}
	if updated.LastSyncedAt == nil {
		t.Fatal("expected last_synced_at to be populated")
	}
}

func newTestHub(t *testing.T, authTimeout time.Duration) (*Hub, *httptest.Server) {
	t.Helper()

	gin.SetMode(gin.ReleaseMode)

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(store.AllModels()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	authService, err := auth.NewService(db, saasconfig.JWTConfig{
		Issuer:   "test",
		Secret:   "secret",
		TTLHours: 1,
	})
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	hub := NewHub(db, authService, nil, zap.NewNop(), authTimeout)
	router := gin.New()
	router.GET("/ws/agent", hub.HandleConnection)

	return hub, httptest.NewServer(router)
}

func wsURL(httpURL string) string {
	parsed, _ := url.Parse(httpURL)
	parsed.Scheme = "ws"
	parsed.Path = "/ws/agent"
	return parsed.String()
}

func writeEnvelope(t *testing.T, conn *websocket.Conn, messageType string, payload any) {
	t.Helper()
	env, err := protocol.Wrap(messageType, payload)
	if err != nil {
		t.Fatalf("wrap payload: %v", err)
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatalf("write message: %v", err)
	}
}

func readEnvelope(t *testing.T, conn *websocket.Conn, expectedType string, out any) {
	t.Helper()
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}

	var env protocol.Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Type != expectedType {
		t.Fatalf("expected message type %s, got %s", expectedType, env.Type)
	}
	if out != nil && len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, out); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
	}
}
