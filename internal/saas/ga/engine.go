package ga

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"time"

	"quantsaas/internal/quant"
	"quantsaas/internal/saas/store"
	"quantsaas/internal/strategies"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const DefaultEvolutionInitialCapitalUSDT = 10000

type ProgressUpdate struct {
	Generation      int     `json:"generation"`
	BestScore       float64 `json:"best_score"`
	MutationProb    float64 `json:"mutation_prob"`
	MutationScale   float64 `json:"mutation_scale"`
	CurrentDrawdown float64 `json:"current_drawdown"`
}

type EpochConfig struct {
	PopSize            int
	MaxGenerations     int
	LotStepSize        float64
	LotMinQty          float64
	OnProgress         func(ProgressUpdate)
	SpawnPointOverride *quant.SpawnPoint
}

type EpochResult struct {
	TemplateID  string
	Symbol      string
	BestScore   float64
	MaxDrawdown float64
	Record      store.GeneRecord
}

type Engine struct {
	db *gorm.DB

	evolvables map[string]EvolvableStrategy

	PopSize                int
	MaxGenerations         int
	EliteCount             int
	MutationProbability    float64
	MutationScale          float64
	MutationProbabilityMax float64
	MutationScaleMax       float64
	MutationRampFactor     float64
	EarlyStopPatience      int
	EarlyStopMinDelta      float64
	TournamentSize         int
}

func NewEngine(db *gorm.DB) *Engine {
	evolvables := make(map[string]EvolvableStrategy)
	for _, spec := range strategies.Catalog() {
		evolvables[spec.Manifest.ID] = NewTemplateEvolvable(spec)
	}

	return &Engine{
		db:                     db,
		evolvables:             evolvables,
		PopSize:                300,
		MaxGenerations:         25,
		EliteCount:             8,
		MutationProbability:    0.15,
		MutationScale:          1.0,
		MutationProbabilityMax: 0.55,
		MutationScaleMax:       3.0,
		MutationRampFactor:     1.25,
		EarlyStopPatience:      5,
		EarlyStopMinDelta:      0.001,
		TournamentSize:         3,
	}
}

func (e *Engine) RunEpoch(ctx context.Context, templateID string, cfg EpochConfig) (EpochResult, error) {
	evolvable, ok := e.evolvables[templateID]
	if !ok {
		return EpochResult{}, fmt.Errorf("unknown template %q", templateID)
	}
	spec, err := strategies.Lookup(templateID)
	if err != nil {
		return EpochResult{}, err
	}

	popSize := cfg.PopSize
	if popSize <= 0 {
		popSize = e.PopSize
	}
	maxGenerations := cfg.MaxGenerations
	if maxGenerations <= 0 {
		maxGenerations = e.MaxGenerations
	}

	plan, err := e.buildPlan(ctx, spec, cfg)
	if err != nil {
		return EpochResult{}, err
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	population, err := e.initializePopulation(ctx, evolvable, plan, popSize, rng)
	if err != nil {
		return EpochResult{}, err
	}

	mutProb := e.MutationProbability
	mutScale := e.MutationScale
	bestEver := -math.MaxFloat64
	patience := 0

	scores, fitness, err := e.evaluatePopulation(ctx, evolvable, population, plan)
	if err != nil {
		return EpochResult{}, err
	}

	for gen := 0; gen < maxGenerations; gen++ {
		sortScoredPopulation(population, scores, fitness)
		best := scores[0]
		if best-bestEver < e.EarlyStopMinDelta {
			patience++
		} else {
			bestEver = best
			patience = 0
		}

		if cfg.OnProgress != nil {
			cfg.OnProgress(ProgressUpdate{
				Generation:      gen + 1,
				BestScore:       scores[0],
				MutationProb:    mutProb,
				MutationScale:   mutScale,
				CurrentDrawdown: fitness[0].MaxDrawdown,
			})
		}

		if patience >= e.EarlyStopPatience {
			newProb := math.Min(e.MutationProbabilityMax, mutProb*e.MutationRampFactor)
			newScale := math.Min(e.MutationScaleMax, mutScale*e.MutationRampFactor)
			if newProb == mutProb && newScale == mutScale {
				break
			}
			mutProb = newProb
			mutScale = newScale
			patience = 0
		}

		if gen == maxGenerations-1 {
			break
		}

		next := make([]Gene, 0, popSize)
		eliteCount := minInt(e.EliteCount, len(population))
		next = append(next, population[:eliteCount]...)
		for len(next) < popSize {
			p1 := tournamentSelect(population, scores, e.TournamentSize, rng)
			p2 := tournamentSelect(population, scores, e.TournamentSize, rng)
			child := evolvable.Crossover(p1, p2, rng)
			child = evolvable.Mutate(child, mutProb, mutScale, rng)
			next = append(next, child)
		}
		population = next
		scores, fitness, err = e.evaluatePopulation(ctx, evolvable, population, plan)
		if err != nil {
			return EpochResult{}, err
		}
	}

	sortScoredPopulation(population, scores, fitness)
	record, err := e.persistChallenger(ctx, evolvable, plan, population[0], fitness[0])
	if err != nil {
		return EpochResult{}, err
	}

	return EpochResult{
		TemplateID:  templateID,
		Symbol:      plan.Symbol,
		BestScore:   fitness[0].ScoreTotal,
		MaxDrawdown: fitness[0].MaxDrawdown,
		Record:      record,
	}, nil
}

func (e *Engine) buildPlan(ctx context.Context, spec strategies.Spec, cfg EpochConfig) (EvaluablePlan, error) {
	bars, err := e.loadBars(ctx, spec.Manifest.Symbol)
	if err != nil {
		return EvaluablePlan{}, err
	}
	windows, err := quant.BuildCrucibleWindows(bars, 1200)
	if err != nil {
		return EvaluablePlan{}, err
	}

	spawn := quant.DefaultSpawnPoint
	if cfg.SpawnPointOverride != nil {
		spawn = *cfg.SpawnPointOverride
	}

	baselines := make([]DCABaseline, 0, len(windows))
	for _, window := range windows {
		result, err := quant.SimulateGhostDCA(quant.FilterEvaluationBars(window), quant.GhostDCAConfig{
			InitialCapital: DefaultEvolutionInitialCapitalUSDT,
			MonthlyInject:  spawn.Policy.MonthlyInjectUSDT,
		})
		if err != nil {
			return EvaluablePlan{}, err
		}
		baselines = append(baselines, DCABaseline{
			FinalEquity:   result.FinalEquity,
			TotalInjected: result.TotalInjected,
			MaxDrawdown:   result.MaxDrawdown,
			ROI:           result.ROI,
		})
	}

	return EvaluablePlan{
		TemplateID:         spec.Manifest.ID,
		Symbol:             spec.Manifest.Symbol,
		SpawnPoint:         spawn,
		Windows:            windows,
		DCABaselines:       baselines,
		LotStep:            cfg.LotStepSize,
		LotMin:             cfg.LotMinQty,
		InitialCapitalUSDT: DefaultEvolutionInitialCapitalUSDT,
	}, nil
}

func (e *Engine) initializePopulation(ctx context.Context, evolvable EvolvableStrategy, plan EvaluablePlan, popSize int, rng *rand.Rand) ([]Gene, error) {
	elites, champion, err := e.loadEliteGenes(ctx, evolvable.StrategyID())
	if err != nil {
		return nil, err
	}

	population := make([]Gene, 0, popSize)
	if champion != nil {
		population = append(population, champion)
	} else {
		population = append(population, evolvable.Sample(rng))
	}

	if len(elites) == 0 {
		for len(population) < popSize {
			population = append(population, evolvable.Sample(rng))
		}
		return population, nil
	}

	remaining := popSize - 1
	copyCount := int(math.Round(float64(remaining) * 0.10))
	mutateCount := int(math.Round(float64(remaining) * 0.40))
	for i := 0; i < copyCount && len(population) < popSize; i++ {
		population = append(population, elites[i%len(elites)])
	}
	for i := 0; i < mutateCount && len(population) < popSize; i++ {
		base := elites[i%len(elites)]
		population = append(population, evolvable.Mutate(base, 0.15, 1.5, rng))
	}
	for len(population) < popSize {
		population = append(population, evolvable.Sample(rng))
	}
	_ = plan
	return population, nil
}

func (e *Engine) loadEliteGenes(ctx context.Context, strategyID string) ([]Gene, Gene, error) {
	var records []store.GeneRecord
	if err := e.db.WithContext(ctx).
		Where("strategy_id = ? AND role IN ?", strategyID, []string{store.GeneRoleChampion, store.GeneRoleChallenger}).
		Order("score_total DESC, created_at DESC").
		Limit(64).
		Find(&records).Error; err != nil {
		return nil, nil, err
	}

	evolvable := e.evolvables[strategyID]
	var champion Gene
	elites := make([]Gene, 0, len(records))
	for _, record := range records {
		gene := evolvable.DecodeElite(json.RawMessage(record.ParamPack))
		if record.Role == store.GeneRoleChampion && champion == nil {
			champion = gene
		}
		elites = append(elites, gene)
	}
	return elites, champion, nil
}

func (e *Engine) evaluatePopulation(ctx context.Context, evolvable EvolvableStrategy, population []Gene, plan EvaluablePlan) ([]float64, []FitnessResult, error) {
	workers := minInt(runtime.NumCPU(), len(population))
	if workers <= 0 {
		return nil, nil, errors.New("population is empty")
	}

	type job struct {
		index int
		gene  Gene
	}
	type result struct {
		index   int
		fitness FitnessResult
		err     error
	}

	jobs := make(chan job, len(population))
	results := make(chan result, len(population))
	var cache sync.Map
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for job := range jobs {
			fingerprint := evolvable.Fingerprint(job.gene)
			if cached, ok := cache.Load(fingerprint); ok {
				results <- result{index: job.index, fitness: cached.(FitnessResult)}
				continue
			}
			fitness, err := evolvable.Evaluate(ctx, job.gene, plan)
			if err == nil {
				cache.Store(fingerprint, fitness)
			}
			results <- result{index: job.index, fitness: fitness, err: err}
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}
	for i, gene := range population {
		jobs <- job{index: i, gene: gene}
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	scores := make([]float64, len(population))
	fitnesses := make([]FitnessResult, len(population))
	for res := range results {
		if res.err != nil {
			return nil, nil, res.err
		}
		scores[res.index] = res.fitness.ScoreTotal
		fitnesses[res.index] = res.fitness
	}

	return scores, fitnesses, nil
}

func (e *Engine) persistChallenger(ctx context.Context, evolvable EvolvableStrategy, plan EvaluablePlan, gene Gene, fitness FitnessResult) (store.GeneRecord, error) {
	paramPack, err := evolvable.EncodeResult(gene, plan.SpawnPoint)
	if err != nil {
		return store.GeneRecord{}, err
	}
	windowJSON, err := json.Marshal(fitness.WindowScores)
	if err != nil {
		return store.GeneRecord{}, err
	}
	record := store.GeneRecord{
		StrategyID:   plan.TemplateID,
		Symbol:       plan.Symbol,
		Role:         store.GeneRoleChallenger,
		ParamPack:    datatypes.JSON(paramPack),
		ScoreTotal:   fitness.ScoreTotal,
		MaxDrawdown:  fitness.MaxDrawdown,
		WindowScores: datatypes.JSON(windowJSON),
	}
	if err := e.db.WithContext(ctx).Create(&record).Error; err != nil {
		return store.GeneRecord{}, err
	}
	return record, nil
}

func (e *Engine) loadBars(ctx context.Context, symbol string) ([]quant.Bar, error) {
	var rows []store.KLine
	if err := e.db.WithContext(ctx).
		Where("symbol = ? AND interval = ?", symbol, "1h").
		Order("open_time ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no 1h bars found for %s", symbol)
	}
	bars := make([]quant.Bar, 0, len(rows))
	for _, row := range rows {
		bars = append(bars, quant.Bar{
			OpenTime: row.OpenTime,
			Open:     row.Open,
			High:     row.High,
			Low:      row.Low,
			Close:    row.Close,
			Volume:   row.Volume,
		})
	}
	return bars, nil
}

func sortScoredPopulation(population []Gene, scores []float64, fitness []FitnessResult) {
	type item struct {
		gene    Gene
		score   float64
		fitness FitnessResult
	}
	items := make([]item, 0, len(population))
	for i := range population {
		items = append(items, item{
			gene:    population[i],
			score:   scores[i],
			fitness: fitness[i],
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].fitness.MaxDrawdown < items[j].fitness.MaxDrawdown
		}
		return items[i].score > items[j].score
	})
	for i, item := range items {
		population[i] = item.gene
		scores[i] = item.score
		fitness[i] = item.fitness
	}
}

func tournamentSelect(population []Gene, scores []float64, tournamentSize int, rng *rand.Rand) Gene {
	if len(population) == 0 {
		return nil
	}
	bestIdx := rng.Intn(len(population))
	seen := map[int]struct{}{bestIdx: {}}
	for len(seen) < minInt(tournamentSize, len(population)) {
		idx := rng.Intn(len(population))
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		if scores[idx] > scores[bestIdx] {
			bestIdx = idx
		}
	}
	return population[bestIdx]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
