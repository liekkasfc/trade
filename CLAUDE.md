# QuantSaaS Project Constitution

## Source Of Truth

Only the following docs define product behavior:

1. `docs/系统总体拓扑结构.md`
2. `docs/策略数学引擎.md`
3. `docs/进化计算引擎.md`

`docs/实施计划-v1.md` defines the execution order and first-slice boundaries. Historical docs in `docs/` may provide context but do not override these four files.

## Non-Negotiable Rules

1. `Step()` must remain a pure function.
2. Backtest, GA, and live trading must call the same `Step()` implementation.
3. Exchange API keys may exist only in `config.agent.yaml`.
4. All schema changes must be expressed through Go structs plus GORM AutoMigrate.
5. v1 only supports Bitget spot `BTCUSDT` and `ETHUSDT`.
6. AI must not influence orders or GA in v1.
7. Live, backtest, and GA all operate on completed `1h` bars.

## Working Order

1. Read the relevant source-of-truth doc before changing code.
2. Prefer behavior-preserving infrastructure changes before strategy changes.
3. Keep execution realism ahead of backtest aesthetics.
4. Do not add speculative abstractions for markets, exchanges, or strategies outside v1 scope.

## Directory Responsibilities

- `cmd/saas`: SaaS entrypoint
- `cmd/agent`: LocalAgent entrypoint
- `internal/saas`: backend infrastructure and HTTP services
- `internal/agent`: agent-side config and Bitget execution client
- `internal/quant`: shared math, backtest, and state helpers
- `internal/strategies`: pure-function strategy templates
- `web-frontend`: React/Vite control panel

## Validation

- `go list ./...`
- `go test ./...`
