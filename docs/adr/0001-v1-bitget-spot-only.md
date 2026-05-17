# V1 is Bitget spot only

QuantSaaS v1 is intentionally constrained to Bitget spot trading for `BTCUSDT` and `ETHUSDT`, with market-order execution and `1h` completed-bar semantics. We are not building a generic exchange, perp, or multi-order-type abstraction in v1 because that complexity would distort the strategy interface, ledger model, agent protocol, and backtest/live parity before the core product is proven.
