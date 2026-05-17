# QuantSaaS v1

QuantSaaS v1 is a hosted DCA-enhanced spot trading system for long-term core assets on Bitget. It exists to run one shared strategy model consistently across live trading, backtests, and GA without drifting into a generic altcoin trading platform.

## Language

**Template**:
A versioned strategy product that defines one fixed symbol, one fixed strategy semantics, and one shared champion parameter set.
_Avoid_: Strategy family, bot type

**Instance**:
A user's runtime configuration of a single Template.
_Avoid_: Bot, account, symbol config

**Champion Parameter Pack**:
The promoted chromosome set shared by all Instances of one Template.
_Avoid_: Instance params, custom live params

**DeadAsset / DeadStack**:
The macro core holding that is accumulated for long-term ownership and is not sold unless explicitly released into floating inventory.
_Avoid_: Main position, fixed bag

**FloatAsset / Floating**:
The micro floating inventory that Step() may increase or decrease.
_Avoid_: Trading bag, scalp position

**ColdSealedAsset / ColdSealed**:
The permanently sealed inventory that is never released and never sold by the system.
_Avoid_: Reserve, floating reserve

**ReleaseIntent**:
An internal ledger instruction that reclassifies asset from DeadStack into Floating without creating an exchange order.
_Avoid_: Sell, conversion trade

**Completed 1h Bar**:
The only market time unit allowed to advance Step() in v1.
_Avoid_: Live candle, partial bar, realtime tick

**LastProcessedBarTime**:
The OpenTime of the last Completed 1h Bar already processed for an Instance.
_Avoid_: Tick time, close time, scheduler time

**AI Display Signal**:
An informational signal used only for user-facing display and never for Step(), order generation, or GA in v1.
_Avoid_: Trading signal, execution factor

## Relationships

- A **Template** owns exactly one fixed symbol in v1: `core-btc-v1 -> BTCUSDT`, `core-eth-v1 -> ETHUSDT`
- An **Instance** belongs to exactly one **Template**
- A **Champion Parameter Pack** belongs to exactly one **Template**
- Many **Instances** may share the same **Champion Parameter Pack**
- An **Instance** contains **DeadAsset**, **FloatAsset**, and optional **ColdSealedAsset**
- A **ReleaseIntent** moves quantity from **DeadAsset** to **FloatAsset**
- A **Completed 1h Bar** advances an **Instance** at most once, keyed by **LastProcessedBarTime**
- An **AI Display Signal** may be shown beside an **Instance** but does not affect its strategy decisions

## Example dialogue

> **Dev:** "When a user creates an Instance for ETH, do they choose `ETHUSDT` directly?"
> **Domain expert:** "No. They choose the `core-eth-v1` Template, and that Template already implies `ETHUSDT`, shared champion params, and the ETH strategy semantics."

## Flagged ambiguities

- "instance = template + symbol + quota" was used loosely. Resolved: the **Template** already fixes the symbol in v1; the **Instance** only adds runtime configuration.
- "BTC" field names appear in storage code even for ETH flows. Resolved: domain language is asset-neutral; BTC-suffixed field names are implementation legacy, not glossary terms.
- "AI signal" was described in one plan document as part of Step() and GA. Resolved: in v1, **AI Display Signal** is presentation-only and not part of Step(), orders, or GA.
