package coreethv1

import "quantsaas/internal/strategies/coretemplate"

var Manifest = coretemplate.Manifest{
	ID:          "core-eth-v1",
	Name:        "Core ETH v1",
	Version:     "0.1.0",
	Symbol:      "ETHUSDT",
	Description: "Conservative DCA-enhanced core ETH strategy built for completed 1h bars.",
	IsSpot:      true,
}
