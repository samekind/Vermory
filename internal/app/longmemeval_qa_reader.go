package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"vermory/internal/artifact"
	"vermory/internal/benchmark"
	"vermory/internal/provider"
)

type LongMemEvalQAOptions struct {
	Qualification          benchmark.Qualification
	Execution              benchmark.ExecutionManifest
	SourceDatasetPath      string
	RetrievalResultsPath   string
	ArtifactRoot           string
	RunID                  string
	ImplementationRevision string
	Resume                 bool
	ReaderProvider         provider.Provider
	JudgeProvider          provider.Provider
	RetrySleeper           func(context.Context, time.Duration) error
}

type LongMemEvalQAReaderSummary struct {
	RunID         string `json:"run_id"`
	Total         int    `json:"total"`
	Completed     int    `json:"completed"`
	Failed        int    `json:"failed"`
	Resumed       int    `json:"resumed"`
	ProviderCalls int    `json:"provider_calls"`
}

type longMemEvalQAReaderRuntime struct {
	opts      LongMemEvalQAOptions
	contract  longMemEvalQACheckpointContract
	store     *artifact.LocalStore
	sleep     func(context.Context, time.Duration) error
	summaryMu sync.Mutex
	summary   LongMemEvalQAReaderSummary
}

func RunLongMemEvalQAReader(ctx context.Context, opts LongMemEvalQAOptions) (LongMemEvalQAReaderSummary, error) {
	runtime, retrieval, sourcePath, err := prepareLongMemEvalQAReader(opts)
	if err != nil {
		return LongMemEvalQAReaderSummary{}, err
	}
	opts = runtime.opts
	summary, err := benchmark.ScanLongMemEval(sourcePath, nil)
	if err != nil {
		return LongMemEvalQAReaderSummary{}, err
	}
	if err := validateLongMemEvalRetrievalSummary(opts.Qualification, opts.Execution, summary); err != nil {
		return LongMemEvalQAReaderSummary{}, err
	}
	if len(retrieval) != opts.Qualification.Dataset.RecordCount {
		return LongMemEvalQAReaderSummary{}, fmt.Errorf("retrieval input contains %d records, want %d", len(retrieval), opts.Qualification.Dataset.RecordCount)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	tasks := make(chan LongMemEvalQATask, opts.Execution.Reader.Workers*2)
	var workers sync.WaitGroup
	var fatalMu sync.Mutex
	var fatalErr error
	setFatal := func(err error) {
		fatalMu.Lock()
		if fatalErr == nil {
			fatalErr = err
			cancel()
		}
		fatalMu.Unlock()
	}
	for index := 0; index < opts.Execution.Reader.Workers; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for task := range tasks {
				if err := runtime.processTask(workerCtx, task); err != nil {
					setFatal(err)
					return
				}
			}
		}()
	}

	seen := make(map[string]struct{}, len(retrieval))
	_, scanErr := benchmark.ScanLongMemEval(sourcePath, func(record benchmark.LongMemEvalRecord) error {
		result, exists := retrieval[record.QuestionID]
		if !exists {
			return fmt.Errorf("retrieval input is missing record %q", record.QuestionID)
		}
		seen[record.QuestionID] = struct{}{}
		built, err := BuildLongMemEvalQATasks(record, result, opts.Execution.RetrievalInput.K)
		if err != nil {
			return err
		}
		for _, task := range built {
			select {
			case tasks <- task:
			case <-workerCtx.Done():
				fatalMu.Lock()
				current := fatalErr
				fatalMu.Unlock()
				if current != nil {
					return current
				}
				return workerCtx.Err()
			}
		}
		return nil
	})
	close(tasks)
	workers.Wait()
	fatalMu.Lock()
	currentFatal := fatalErr
	fatalMu.Unlock()
	if currentFatal != nil {
		return runtime.summary, currentFatal
	}
	if scanErr != nil {
		return runtime.summary, scanErr
	}
	if err := ctx.Err(); err != nil {
		return runtime.summary, err
	}
	if len(seen) != len(retrieval) {
		return runtime.summary, fmt.Errorf("retrieval input contains records outside the qualified source")
	}
	expectedTasks := opts.Qualification.Dataset.RecordCount * len(opts.Execution.Conditions)
	if runtime.summary.Total != expectedTasks {
		return runtime.summary, fmt.Errorf("reader represented %d tasks, want %d", runtime.summary.Total, expectedTasks)
	}
	return runtime.summary, nil
}

func prepareLongMemEvalQAReader(opts LongMemEvalQAOptions) (*longMemEvalQAReaderRuntime, map[string]LongMemEvalRetrievalRecordResult, string, error) {
	if opts.ReaderProvider == nil {
		return nil, nil, "", fmt.Errorf("LongMemEval QA reader provider is required")
	}
	opts, contract, retrieval, sourcePath, err := prepareLongMemEvalQAInputs(opts)
	if err != nil {
		return nil, nil, "", err
	}
	sleeper := opts.RetrySleeper
	if sleeper == nil {
		sleeper = sleepLongMemEvalQARetry
	}
	return &longMemEvalQAReaderRuntime{
		opts:     opts,
		contract: contract,
		store:    artifact.NewLocalStore(opts.ArtifactRoot),
		sleep:    sleeper,
		summary:  LongMemEvalQAReaderSummary{RunID: contract.RunID},
	}, retrieval, sourcePath, nil
}

func prepareLongMemEvalQAInputs(opts LongMemEvalQAOptions) (LongMemEvalQAOptions, longMemEvalQACheckpointContract, map[string]LongMemEvalRetrievalRecordResult, string, error) {
	if err := benchmark.ValidateLongMemEvalQAExecution(opts.Qualification, opts.Execution); err != nil {
		return LongMemEvalQAOptions{}, longMemEvalQACheckpointContract{}, nil, "", err
	}
	if strings.TrimSpace(opts.SourceDatasetPath) == "" || strings.TrimSpace(opts.RetrievalResultsPath) == "" {
		return LongMemEvalQAOptions{}, longMemEvalQACheckpointContract{}, nil, "", fmt.Errorf("LongMemEval QA source dataset and retrieval results are required")
	}
	if strings.TrimSpace(opts.ArtifactRoot) == "" {
		opts.ArtifactRoot = "./artifacts"
	}
	runID := strings.TrimSpace(opts.RunID)
	frozenRunID := strings.TrimSpace(opts.Execution.RunID)
	if runID == "" {
		runID = frozenRunID
	}
	if frozenRunID != "" && runID != frozenRunID {
		return LongMemEvalQAOptions{}, longMemEvalQACheckpointContract{}, nil, "", fmt.Errorf("LongMemEval QA run ID %q differs from execution %q", runID, opts.Execution.RunID)
	}
	if err := validateLongMemEvalRetrievalSegment(runID, "run ID"); err != nil {
		return LongMemEvalQAOptions{}, longMemEvalQACheckpointContract{}, nil, "", err
	}
	implementationRevision := strings.TrimSpace(opts.ImplementationRevision)
	if implementationRevision == "" {
		implementationRevision = strings.TrimSpace(opts.Execution.ImplementationRev)
	}
	if implementationRevision == "" {
		implementationRevision = buildVCSRevision()
	}
	if opts.Execution.ImplementationRev != "" && implementationRevision != opts.Execution.ImplementationRev {
		return LongMemEvalQAOptions{}, longMemEvalQACheckpointContract{}, nil, "", fmt.Errorf("LongMemEval QA implementation revision %q differs from execution %q", implementationRevision, opts.Execution.ImplementationRev)
	}
	sourcePath, err := filepath.Abs(opts.SourceDatasetPath)
	if err != nil {
		return LongMemEvalQAOptions{}, longMemEvalQACheckpointContract{}, nil, "", err
	}
	if err := benchmark.VerifyFileSHA256(sourcePath, opts.Qualification.Dataset.SHA256); err != nil {
		return LongMemEvalQAOptions{}, longMemEvalQACheckpointContract{}, nil, "", fmt.Errorf("verify LongMemEval-S source: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return LongMemEvalQAOptions{}, longMemEvalQACheckpointContract{}, nil, "", err
	}
	if info.Size() != opts.Qualification.Dataset.SizeBytes {
		return LongMemEvalQAOptions{}, longMemEvalQACheckpointContract{}, nil, "", fmt.Errorf("LongMemEval-S source size is %d, want %d", info.Size(), opts.Qualification.Dataset.SizeBytes)
	}
	retrieval, err := LoadLongMemEvalQARetrieval(opts.RetrievalResultsPath, opts.Execution)
	if err != nil {
		return LongMemEvalQAOptions{}, longMemEvalQACheckpointContract{}, nil, "", err
	}
	contract := longMemEvalQACheckpointContract{
		RunID:                  runID,
		ImplementationRevision: implementationRevision,
		DatasetSHA256:          opts.Execution.DatasetSHA256,
		RecordSetSHA256:        opts.Execution.RecordSetSHA256,
		RetrievalSHA256:        opts.Execution.RetrievalInput.SHA256,
		RetrievalRunID:         opts.Execution.RetrievalInput.RunID,
		RetrievalRevision:      opts.Execution.RetrievalInput.ImplementationRevision,
		K:                      opts.Execution.RetrievalInput.K,
		Reader:                 *opts.Execution.Reader,
	}
	opts.RunID = runID
	opts.ImplementationRevision = implementationRevision
	return opts, contract, retrieval, sourcePath, nil
}

func (runtime *longMemEvalQAReaderRuntime) processTask(ctx context.Context, task LongMemEvalQATask) error {
	path, err := longMemEvalQACheckpointPath(runtime.opts.ArtifactRoot, runtime.contract.RunID, task.RecordID, task.Condition)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		if !runtime.opts.Resume {
			return fmt.Errorf("checkpoint already exists for record %q condition %q; use resume", task.RecordID, task.Condition)
		}
		checkpoint, err := loadLongMemEvalQACheckpoint(path)
		if err != nil {
			return err
		}
		if err := validateLongMemEvalQACheckpoint(checkpoint, task, runtime.contract); err != nil {
			return err
		}
		if runtime.addSummary(checkpoint, true) {
			return fmt.Errorf("reader terminal failure limit %d reached", runtime.contract.Reader.MaxTerminalFailures)
		}
		return nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}

	checkpoint, err := runtime.executeTask(ctx, task)
	if err != nil {
		return err
	}
	if err := validateLongMemEvalQACheckpoint(checkpoint, task, runtime.contract); err != nil {
		return err
	}
	if err := writeLongMemEvalQACheckpoint(path, checkpoint); err != nil {
		return err
	}
	if runtime.addSummary(checkpoint, false) {
		return fmt.Errorf("reader terminal failure limit %d reached", runtime.contract.Reader.MaxTerminalFailures)
	}
	return nil
}

func (runtime *longMemEvalQAReaderRuntime) executeTask(ctx context.Context, task LongMemEvalQATask) (LongMemEvalQACheckpoint, error) {
	checkpoint := LongMemEvalQACheckpoint{
		SchemaVersion:           longMemEvalQACheckpointSchema,
		RunID:                   runtime.contract.RunID,
		ImplementationRevision:  runtime.contract.ImplementationRevision,
		DatasetSHA256:           runtime.contract.DatasetSHA256,
		RecordSetSHA256:         runtime.contract.RecordSetSHA256,
		RetrievalSHA256:         runtime.contract.RetrievalSHA256,
		RetrievalRunID:          runtime.contract.RetrievalRunID,
		RetrievalRevision:       runtime.contract.RetrievalRevision,
		K:                       runtime.contract.K,
		Reader:                  runtime.contract.Reader,
		PromptSHA256:            task.PromptSHA256,
		ContextSHA256:           task.ContextSHA256,
		RecordID:                task.RecordID,
		QuestionType:            task.QuestionType,
		Abstention:              task.Abstention,
		Condition:               task.Condition,
		RetrievalClassification: task.RetrievalClassification,
		RankedOccurrenceKeys:    append([]string(nil), task.RankedOccurrenceKeys...),
		RankedSessionIDs:        append([]string(nil), task.RankedSessionIDs...),
	}
	for attemptNumber := 1; attemptNumber <= runtime.contract.Reader.MaxAttempts; attemptNumber++ {
		if err := ctx.Err(); err != nil {
			return LongMemEvalQACheckpoint{}, err
		}
		started := time.Now().UTC()
		attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(runtime.contract.Reader.TimeoutSeconds)*time.Second)
		response, generateErr := runtime.opts.ReaderProvider.Generate(attemptCtx, provider.GenerateRequest{
			Model:         runtime.contract.Reader.Model,
			System:        task.SystemPrompt,
			Prompt:        task.Question,
			ContextPacket: task.ContextPacket,
			MaxTokens:     runtime.contract.Reader.MaxOutputTokens,
		})
		attemptContextErr := attemptCtx.Err()
		cancel()
		if err := ctx.Err(); err != nil {
			return LongMemEvalQACheckpoint{}, err
		}
		attempt := LongMemEvalQAAttempt{
			Number:         attemptNumber,
			StartedAt:      started.Format(time.RFC3339Nano),
			DurationMillis: time.Since(started).Milliseconds(),
			ProviderModel:  strings.TrimSpace(response.Model),
			Usage:          cloneLongMemEvalQAUsage(response.Usage),
		}
		if len(response.RawArtifact) != 0 {
			key := filepath.ToSlash(filepath.Join("benchmarks", runtime.contract.RunID, "raw", "reader", task.RecordID, task.Condition, fmt.Sprintf("attempt-%03d.json", attemptNumber)))
			stored, err := runtime.store.Put(ctx, key, response.RawArtifact)
			if err != nil {
				return LongMemEvalQACheckpoint{}, err
			}
			attempt.RawArtifactURI = stored.URI
			attempt.RawArtifactSHA256 = stored.SHA256
			attempt.RawArtifactBytes = stored.ByteSize
		}
		output := strings.TrimSpace(response.Output)
		if generateErr == nil && attemptContextErr == nil && output != "" {
			attempt.Status = longMemEvalQAAttemptCompleted
			checkpoint.Attempts = append(checkpoint.Attempts, attempt)
			checkpoint.ReaderStatus = longMemEvalQAReaderCompleted
			checkpoint.Response = output
			checkpoint.ProviderModel = strings.TrimSpace(response.Model)
			if checkpoint.ProviderModel == "" {
				checkpoint.ProviderModel = runtime.contract.Reader.Model
			}
			score := benchmark.ScoreAnswer(task.Record, output)
			checkpoint.Score = &score
			return checkpoint, nil
		}
		attempt.Status = longMemEvalQAAttemptFailed
		switch {
		case attemptContextErr != nil:
			attempt.Error = truncateLongMemEvalQAError(attemptContextErr.Error())
			attempt.Retryable = longMemEvalQABoolPointer(true)
		case generateErr != nil:
			attempt.Error = truncateLongMemEvalQAError(generateErr.Error())
			attempt.Retryable = longMemEvalQABoolPointer(provider.ShouldRetry(generateErr))
		default:
			attempt.Error = "provider returned empty output"
			attempt.Retryable = longMemEvalQABoolPointer(true)
		}
		checkpoint.Attempts = append(checkpoint.Attempts, attempt)
		if attempt.Retryable != nil && !*attempt.Retryable {
			break
		}
		if attemptNumber < runtime.contract.Reader.MaxAttempts {
			delay := time.Second << (attemptNumber - 1)
			if err := runtime.sleep(ctx, delay); err != nil {
				return LongMemEvalQACheckpoint{}, err
			}
		}
	}
	checkpoint.ReaderStatus = longMemEvalQAReaderFailed
	return checkpoint, nil
}

func (runtime *longMemEvalQAReaderRuntime) addSummary(checkpoint LongMemEvalQACheckpoint, resumed bool) bool {
	runtime.summaryMu.Lock()
	defer runtime.summaryMu.Unlock()
	runtime.summary.Total++
	if resumed {
		runtime.summary.Resumed++
	} else {
		runtime.summary.ProviderCalls += len(checkpoint.Attempts)
	}
	if checkpoint.ReaderStatus == longMemEvalQAReaderCompleted {
		runtime.summary.Completed++
	} else {
		runtime.summary.Failed++
	}
	limit := runtime.contract.Reader.MaxTerminalFailures
	return limit > 0 && runtime.summary.Failed >= limit
}

func sleepLongMemEvalQARetry(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func cloneLongMemEvalQAUsage(usage *provider.TokenUsage) *provider.TokenUsage {
	if usage == nil {
		return nil
	}
	copy := *usage
	return &copy
}

func longMemEvalQABoolPointer(value bool) *bool {
	return &value
}

func truncateLongMemEvalQAError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 1000 {
		return message
	}
	message = message[:1000]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message
}
