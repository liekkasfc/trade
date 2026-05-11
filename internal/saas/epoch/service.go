package epoch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"quantsaas/internal/quant"
	"quantsaas/internal/saas/ga"
	"quantsaas/internal/saas/store"
	"quantsaas/internal/strategies"

	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	EvolutionStatusRunning   = "running"
	EvolutionStatusCompleted = "completed"
	EvolutionStatusFailed    = "failed"
)

type CreateTaskRequest struct {
	TemplateID     string            `json:"template_id"`
	PopSize        int               `json:"pop_size"`
	MaxGenerations int               `json:"max_generations"`
	SpawnMode      string            `json:"spawn_mode"`
	SpawnPoint     *quant.SpawnPoint `json:"spawn_point,omitempty"`
}

type Service struct {
	db     *gorm.DB
	cache  *store.Cache
	logger *zap.Logger
	engine *ga.Engine

	mu          sync.Mutex
	currentTask *uint
}

func NewService(db *gorm.DB, cache *store.Cache, logger *zap.Logger) *Service {
	return &Service{
		db:     db,
		cache:  cache,
		logger: logger,
		engine: ga.NewEngine(db),
	}
}

func (s *Service) CreateAndRunTask(ctx context.Context, req CreateTaskRequest) (*store.EvolutionTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentTask != nil {
		return nil, errors.New("an evolution task is already running")
	}
	spec, err := strategies.Lookup(req.TemplateID)
	if err != nil {
		return nil, err
	}

	spawn, err := s.resolveSpawnPoint(ctx, spec, req)
	if err != nil {
		return nil, err
	}

	configJSON, _ := json.Marshal(map[string]any{
		"template_id":     req.TemplateID,
		"pop_size":        req.PopSize,
		"max_generations": req.MaxGenerations,
		"spawn_mode":      req.SpawnMode,
		"spawn_point":     spawn,
	})
	task := &store.EvolutionTask{
		StrategyID: spec.Manifest.ID,
		Symbol:     spec.Manifest.Symbol,
		Status:     EvolutionStatusRunning,
		ConfigJSON: datatypes.JSON(configJSON),
	}
	if err := s.db.WithContext(ctx).Create(task).Error; err != nil {
		return nil, err
	}
	s.currentTask = &task.ID

	go s.run(task.ID, req, spawn)
	return task, nil
}

func (s *Service) ListTasks(ctx context.Context, templateID string) ([]store.EvolutionTask, error) {
	query := s.db.WithContext(ctx).Order("created_at DESC")
	if templateID != "" {
		query = query.Where("strategy_id = ?", templateID)
	}
	var tasks []store.EvolutionTask
	if err := query.Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *Service) ListGenomes(ctx context.Context, templateID string) ([]store.GeneRecord, error) {
	query := s.db.WithContext(ctx).Order("created_at DESC")
	if templateID != "" {
		query = query.Where("strategy_id = ?", templateID)
	}
	var records []store.GeneRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Service) ListChallengers(ctx context.Context, templateID string) ([]store.GeneRecord, error) {
	query := s.db.WithContext(ctx).Where("role = ?", store.GeneRoleChallenger).Order("created_at DESC")
	if templateID != "" {
		query = query.Where("strategy_id = ?", templateID)
	}
	var records []store.GeneRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Service) GetChampion(ctx context.Context, templateID string) (*store.GeneRecord, error) {
	if _, err := strategies.Lookup(templateID); err != nil {
		return nil, err
	}

	var champion store.GeneRecord
	if err := s.db.WithContext(ctx).
		Where("strategy_id = ? AND role = ?", templateID, store.GeneRoleChampion).
		Order("created_at DESC").
		Take(&champion).Error; err != nil {
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, "champion:param_pack:"+templateID, string(champion.ParamPack), 24*time.Hour)
	}
	return &champion, nil
}

func (s *Service) Promote(ctx context.Context, taskID uint) error {
	var task store.EvolutionTask
	if err := s.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return err
	}
	if task.ResultGeneID == nil {
		return errors.New("task has no challenger result")
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&store.GeneRecord{}).
			Where("strategy_id = ? AND role = ?", task.StrategyID, store.GeneRoleChampion).
			Update("role", store.GeneRoleRetired).Error; err != nil {
			return err
		}
		if err := tx.Model(&store.GeneRecord{}).
			Where("id = ? AND role = ?", *task.ResultGeneID, store.GeneRoleChallenger).
			Update("role", store.GeneRoleChampion).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if s.cache != nil {
		_ = s.cache.Del(ctx, "champion:param_pack:"+task.StrategyID)
	}
	return nil
}

func (s *Service) resolveSpawnPoint(ctx context.Context, spec strategies.Spec, req CreateTaskRequest) (quant.SpawnPoint, error) {
	switch req.SpawnMode {
	case "", "inherit":
		var champion store.GeneRecord
		if err := s.db.WithContext(ctx).
			Where("strategy_id = ? AND role = ?", spec.Manifest.ID, store.GeneRoleChampion).
			Order("created_at DESC").
			Take(&champion).Error; err == nil {
			params, parseErr := spec.ParseParamPack(json.RawMessage(champion.ParamPack))
			if parseErr == nil {
				return params.SpawnPoint, nil
			}
		}
		return spec.DefaultParams().SpawnPoint, nil
	case "random_once":
		return quant.RandomSpawnPoint(rand.New(rand.NewSource(time.Now().UnixNano()))), nil
	case "manual":
		if req.SpawnPoint == nil {
			return quant.SpawnPoint{}, errors.New("spawn_point is required for manual mode")
		}
		return *req.SpawnPoint, nil
	default:
		return quant.SpawnPoint{}, fmt.Errorf("unsupported spawn_mode %q", req.SpawnMode)
	}
}

func (s *Service) run(taskID uint, req CreateTaskRequest, spawn quant.SpawnPoint) {
	ctx := context.Background()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.currentTask = nil
	}()

	progressWriter := func(update ga.ProgressUpdate) {
		raw, _ := json.Marshal(update)
		if err := s.db.WithContext(ctx).Model(&store.EvolutionTask{}).Where("id = ?", taskID).Update("progress_json", datatypes.JSON(raw)).Error; err != nil {
			s.logger.Error("update evolution progress", zap.Uint("task_id", taskID), zap.Error(err))
		}
	}

	result, err := s.engine.RunEpoch(ctx, req.TemplateID, ga.EpochConfig{
		PopSize:            req.PopSize,
		MaxGenerations:     req.MaxGenerations,
		SpawnPointOverride: &spawn,
		OnProgress:         progressWriter,
	})
	if err != nil {
		s.logger.Error("evolution task failed", zap.Uint("task_id", taskID), zap.Error(err))
		_ = s.db.WithContext(ctx).Model(&store.EvolutionTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":     EvolutionStatusFailed,
			"error_text": err.Error(),
		}).Error
		return
	}

	if err := s.db.WithContext(ctx).Model(&store.EvolutionTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status":         EvolutionStatusCompleted,
		"result_gene_id": result.Record.ID,
		"error_text":     "",
	}).Error; err != nil {
		s.logger.Error("complete evolution task", zap.Uint("task_id", taskID), zap.Error(err))
	}
}
