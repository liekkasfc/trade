package corebtcv1

import "quantsaas/internal/strategies/coretemplate"

var Manifest = coretemplate.Manifest{
	ID:          "core-btc-v1",
	Name:        "Core BTC v1",
	Version:     "0.1.0",
	Symbol:      "BTCUSDT",
	Description: "Conservative DCA-enhanced core BTC strategy built for completed 1h bars.",
	IsSpot:      true,
}
