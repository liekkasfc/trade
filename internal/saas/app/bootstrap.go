package app

import (
	"context"
	"encoding/json"
	"fmt"

	"quantsaas/internal/saas/store"
	"quantsaas/internal/strategies"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func SeedStrategyTemplates(ctx context.Context, db *gorm.DB) error {
	for _, spec := range strategies.Catalog() {
		manifestJSON, err := json.Marshal(spec.Manifest)
		if err != nil {
			return fmt.Errorf("marshal manifest for %s: %w", spec.Manifest.ID, err)
		}

		record := store.StrategyTemplate{
			TemplateKey: spec.Manifest.ID,
			Name:        spec.Manifest.Name,
			Version:     spec.Manifest.Version,
			IsSpot:      spec.Manifest.IsSpot,
			Manifest:    datatypes.JSON(manifestJSON),
		}

		if err := db.WithContext(ctx).Where("template_key = ?", spec.Manifest.ID).Assign(record).FirstOrCreate(&record).Error; err != nil {
			return fmt.Errorf("seed template %s: %w", spec.Manifest.ID, err)
		}
	}
	return nil
}
