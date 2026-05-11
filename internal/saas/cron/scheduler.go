package cron

import (
	"context"
	"sync"
	"time"

	"quantsaas/internal/saas/instance"

	"go.uber.org/zap"
)

type Scheduler struct {
	manager  *instance.Manager
	logger   *zap.Logger
	interval time.Duration

	stopCh chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
}

func NewScheduler(manager *instance.Manager, logger *zap.Logger, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Scheduler{
		manager:  manager,
		logger:   logger,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.loop()
}

func (s *Scheduler) Stop(ctx context.Context) error {
	s.once.Do(func() {
		close(s.stopCh)
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (s *Scheduler) loop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.runOnce()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.runOnce()
		}
	}
}

func (s *Scheduler) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	instances, err := s.manager.ListRunningInstances(ctx)
	if err != nil {
		s.logger.Error("list running instances", zap.Error(err))
		return
	}

	for _, inst := range instances {
		instanceID := inst.ID
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			tickCtx, tickCancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer tickCancel()

			if err := s.manager.Tick(tickCtx, instanceID); err != nil {
				s.logger.Warn("instance tick failed", zap.Uint("instance_id", instanceID), zap.Error(err))
			}
		}()
	}
}
