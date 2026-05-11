# AGENTS

## Project Scope

This repository is building QuantSaaS v1 for Bitget spot `BTCUSDT` and `ETHUSDT`.

## Guardrails

- Do not add support for arbitrary altcoins in v1.
- Do not let AI change orders or GA results in v1.
- Keep `Step()` free of network, database, filesystem, and wall-clock calls.
- Keep live/backtest/GA on the same completed `1h` bar semantics.

## Preferred Workflow

1. Freeze the docs first.
2. Add minimal infrastructure before business logic.
3. Validate compile/test after each slice.
