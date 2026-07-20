package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vermory/internal/artifact"
	"vermory/internal/benchmark"
	"vermory/internal/provider"
)

const (
	longMemEvalQAJudgeCompleted          = "judge_completed"
	longMemEvalQAJudgeFailed             = "judge_failed"
	longMemEvalQAJudgeInvalid            = "judge_invalid"
	longMemEvalQAJudgeNotRunReaderFailed = "not_run_reader_failed"
	longMemEvalQAAttemptInvalid          = "invalid"
)

type LongMemEvalQAJudgeSummary struct {
	RunID         string `json:"run_id"`
	Total         int    `json:"total"`
	Judged        int    `json:"judged"`
	Failed        int    `json:"failed"`
	Invalid       int    `json:"invalid"`
	NotRun        int    `json:"not_run"`
	Resumed       int    `json:"resumed"`
	ProviderCalls int    `json:"provider_calls"`
}

type longMemEvalQAJudgeTask struct {
	record benchmark.LongMemEvalRecord
	task   LongMemEvalQATask
}

type longMemEvalQAJudgeRuntime struct {
	opts      LongMemEvalQAOptions
	contract  longMemEvalQACheckpointContract
	store     *artifact.LocalStore
	sleep     func(context.Context, time.Duration) error
	summaryMu sync.Mutex
	summary   LongMemEvalQAJudgeSummary
}

func RunLongMemEvalQAJudge(ctx context.Context, opts LongMemEvalQAOptions) (LongMemEvalQAJudgeSummary, error) {
	if opts.JudgeProvider == nil {
		return LongMemEvalQAJudgeSummary{}, fmt.Errorf("LongMemEval QA judge provider is required")
	}
	opts, contract, retrieval, sourcePath, err := prepareLongMemEvalQAInputs(opts)
	if err != nil {
		return LongMemEvalQAJudgeSummary{}, err
	}
	sourceSummary, err := benchmark.ScanLongMemEval(sourcePath, nil)
	if err != nil {
		return LongMemEvalQAJudgeSummary{}, err
	}
	if err := validateLongMemEvalRetrievalSummary(opts.Qualification, opts.Execution, sourceSummary); err != nil {
		return LongMemEvalQAJudgeSummary{}, err
	}
	if len(retrieval) != opts.Qualification.Dataset.RecordCount {
		return LongMemEvalQAJudgeSummary{}, fmt.Errorf("retrieval input contains %d records, want %d", len(retrieval), opts.Qualification.Dataset.RecordCount)
	}
	sleeper := opts.RetrySleeper
	if sleeper == nil {
		sleeper = sleepLongMemEvalQARetry
	}
	runtime := &longMemEvalQAJudgeRuntime{
		opts:     opts,
		contract: contract,
		store:    artifact.NewLocalStore(opts.ArtifactRoot),
		sleep:    sleeper,
		summary:  LongMemEvalQAJudgeSummary{RunID: contract.RunID},
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	tasks := make(chan longMemEvalQAJudgeTask, opts.Execution.Judge.Workers*2)
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
	for index := 0; index < opts.Execution.Judge.Workers; index++ {
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
		judgeRecord := benchmark.LongMemEvalRecord{
			QuestionID:   record.QuestionID,
			QuestionType: record.QuestionType,
			Question:     record.Question,
			Answer:       record.Answer,
		}
		for _, task := range built {
			select {
			case tasks <- longMemEvalQAJudgeTask{record: judgeRecord, task: task}:
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
		return runtime.summary, fmt.Errorf("judge represented %d tasks, want %d", runtime.summary.Total, expectedTasks)
	}
	return runtime.summary, nil
}

func (runtime *longMemEvalQAJudgeRuntime) processTask(ctx context.Context, task longMemEvalQAJudgeTask) error {
	path, err := longMemEvalQACheckpointPath(runtime.opts.ArtifactRoot, runtime.contract.RunID, task.task.RecordID, task.task.Condition)
	if err != nil {
		return err
	}
	checkpoint, err := loadLongMemEvalQACheckpoint(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("reader checkpoint is missing for record %q condition %q", task.task.RecordID, task.task.Condition)
		}
		return err
	}
	if err := validateLongMemEvalQACheckpoint(checkpoint, task.task, runtime.contract); err != nil {
		return err
	}
	if checkpoint.Judge != nil {
		if !runtime.opts.Resume {
			return fmt.Errorf("judge state already exists for record %q condition %q; use resume", task.task.RecordID, task.task.Condition)
		}
		expectedPromptSHA256, err := longMemEvalQAJudgePromptSHA(task.record, checkpoint)
		if err != nil {
			return err
		}
		if err := validateLongMemEvalQAJudgeState(*checkpoint.Judge, checkpoint.ReaderStatus, *runtime.opts.Execution.Judge, expectedPromptSHA256); err != nil {
			return err
		}
		if runtime.addSummary(*checkpoint.Judge, true) {
			return fmt.Errorf("judge terminal failure limit %d reached", runtime.opts.Execution.Judge.MaxTerminalFailures)
		}
		return nil
	}
	if checkpoint.ReaderStatus == longMemEvalQAReaderFailed {
		checkpoint.Judge = &LongMemEvalQAJudgeState{
			Config: *runtime.opts.Execution.Judge,
			Status: longMemEvalQAJudgeNotRunReaderFailed,
		}
		if err := writeLongMemEvalQACheckpoint(path, checkpoint); err != nil {
			return err
		}
		if runtime.addSummary(*checkpoint.Judge, false) {
			return fmt.Errorf("judge terminal failure limit %d reached", runtime.opts.Execution.Judge.MaxTerminalFailures)
		}
		return nil
	}
	judge, err := runtime.executeTask(ctx, task.record, checkpoint)
	if err != nil {
		return err
	}
	expectedPromptSHA256, err := longMemEvalQAJudgePromptSHA(task.record, checkpoint)
	if err != nil {
		return err
	}
	if err := validateLongMemEvalQAJudgeState(judge, checkpoint.ReaderStatus, *runtime.opts.Execution.Judge, expectedPromptSHA256); err != nil {
		return err
	}
	checkpoint.Judge = &judge
	if err := writeLongMemEvalQACheckpoint(path, checkpoint); err != nil {
		return err
	}
	if runtime.addSummary(judge, false) {
		return fmt.Errorf("judge terminal failure limit %d reached", runtime.opts.Execution.Judge.MaxTerminalFailures)
	}
	return nil
}

func (runtime *longMemEvalQAJudgeRuntime) executeTask(ctx context.Context, record benchmark.LongMemEvalRecord, checkpoint LongMemEvalQACheckpoint) (LongMemEvalQAJudgeState, error) {
	prompt, err := benchmark.LongMemEvalJudgePrompt(record, checkpoint.Response)
	if err != nil {
		return LongMemEvalQAJudgeState{}, err
	}
	promptDigest := sha256.Sum256([]byte(prompt))
	state := LongMemEvalQAJudgeState{
		Config:       *runtime.opts.Execution.Judge,
		PromptSHA256: hex.EncodeToString(promptDigest[:]),
	}
	hadInvalid := false
	for attemptNumber := 1; attemptNumber <= state.Config.MaxAttempts; attemptNumber++ {
		if err := ctx.Err(); err != nil {
			return LongMemEvalQAJudgeState{}, err
		}
		started := time.Now().UTC()
		attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(state.Config.TimeoutSeconds)*time.Second)
		response, generateErr := runtime.opts.JudgeProvider.Generate(attemptCtx, provider.GenerateRequest{
			Model:     state.Config.Model,
			Prompt:    prompt,
			MaxTokens: state.Config.MaxOutputTokens,
		})
		attemptContextErr := attemptCtx.Err()
		cancel()
		if err := ctx.Err(); err != nil {
			return LongMemEvalQAJudgeState{}, err
		}
		output := strings.TrimSpace(response.Output)
		attempt := LongMemEvalQAAttempt{
			Number:         attemptNumber,
			StartedAt:      started.Format(time.RFC3339Nano),
			DurationMillis: time.Since(started).Milliseconds(),
			Output:         output,
			ProviderModel:  strings.TrimSpace(response.Model),
			Usage:          cloneLongMemEvalQAUsage(response.Usage),
		}
		if len(response.RawArtifact) != 0 {
			key := filepath.ToSlash(filepath.Join("benchmarks", runtime.contract.RunID, "raw", "judge", checkpoint.RecordID, checkpoint.Condition, fmt.Sprintf("attempt-%03d.json", attemptNumber)))
			stored, err := runtime.store.Put(ctx, key, response.RawArtifact)
			if err != nil {
				return LongMemEvalQAJudgeState{}, err
			}
			attempt.RawArtifactURI = stored.URI
			attempt.RawArtifactSHA256 = stored.SHA256
			attempt.RawArtifactBytes = stored.ByteSize
		}
		if attemptContextErr == nil && generateErr == nil {
			correct, parseErr := benchmark.ParseLongMemEvalJudgeLabel(output)
			if parseErr == nil {
				attempt.Status = longMemEvalQAAttemptCompleted
				state.Attempts = append(state.Attempts, attempt)
				state.Status = longMemEvalQAJudgeCompleted
				state.Correct = &correct
				state.Output = output
				state.ProviderModel = strings.TrimSpace(response.Model)
				if state.ProviderModel == "" {
					state.ProviderModel = state.Config.Model
				}
				return state, nil
			}
			hadInvalid = true
			attempt.Status = longMemEvalQAAttemptInvalid
			attempt.Error = truncateLongMemEvalQAError(parseErr.Error())
			attempt.Retryable = longMemEvalQABoolPointer(true)
			state.Output = output
		} else {
			attempt.Status = longMemEvalQAAttemptFailed
			if attemptContextErr != nil {
				attempt.Error = truncateLongMemEvalQAError(attemptContextErr.Error())
				attempt.Retryable = longMemEvalQABoolPointer(true)
			} else {
				attempt.Error = truncateLongMemEvalQAError(generateErr.Error())
				attempt.Retryable = longMemEvalQABoolPointer(provider.ShouldRetry(generateErr))
			}
		}
		state.Attempts = append(state.Attempts, attempt)
		if attempt.Retryable != nil && !*attempt.Retryable {
			break
		}
		if attemptNumber < state.Config.MaxAttempts {
			delay := time.Second << (attemptNumber - 1)
			if err := runtime.sleep(ctx, delay); err != nil {
				return LongMemEvalQAJudgeState{}, err
			}
		}
	}
	if hadInvalid {
		state.Status = longMemEvalQAJudgeInvalid
	} else {
		state.Status = longMemEvalQAJudgeFailed
	}
	return state, nil
}

func validateLongMemEvalQAJudgeState(state LongMemEvalQAJudgeState, readerStatus string, config benchmark.ExecutionModelConfig, expectedPromptSHA256 string) error {
	if state.Config != config {
		return fmt.Errorf("checkpoint judge config differs from frozen judge config")
	}
	if readerStatus == longMemEvalQAReaderFailed {
		if state.Status != longMemEvalQAJudgeNotRunReaderFailed || state.PromptSHA256 != "" || len(state.Attempts) != 0 || state.Correct != nil {
			return fmt.Errorf("reader_failed checkpoint has invalid judge state %q", state.Status)
		}
		return nil
	}
	if readerStatus != longMemEvalQAReaderCompleted {
		return fmt.Errorf("unsupported reader status %q for judge", readerStatus)
	}
	if state.PromptSHA256 != expectedPromptSHA256 {
		return fmt.Errorf("checkpoint judge prompt_sha256 is %q, want %q", state.PromptSHA256, expectedPromptSHA256)
	}
	if len(state.Attempts) == 0 || len(state.Attempts) > config.MaxAttempts {
		return fmt.Errorf("checkpoint judge has %d attempts, maximum %d", len(state.Attempts), config.MaxAttempts)
	}
	switch state.Status {
	case longMemEvalQAJudgeCompleted:
		if state.Correct == nil || len(state.Attempts) == 0 || state.Attempts[len(state.Attempts)-1].Status != longMemEvalQAAttemptCompleted {
			return fmt.Errorf("completed judge state is incomplete")
		}
		label, err := benchmark.ParseLongMemEvalJudgeLabel(state.Output)
		if err != nil || label != *state.Correct {
			return fmt.Errorf("completed judge output and label disagree")
		}
		for _, attempt := range state.Attempts[:len(state.Attempts)-1] {
			if attempt.Status != longMemEvalQAAttemptFailed && attempt.Status != longMemEvalQAAttemptInvalid {
				return fmt.Errorf("completed judge contains invalid prior attempt %d", attempt.Number)
			}
		}
	case longMemEvalQAJudgeFailed:
		if state.Correct != nil || len(state.Attempts) > config.MaxAttempts {
			return fmt.Errorf("failed judge state is incomplete")
		}
		if len(state.Attempts) < config.MaxAttempts {
			last := state.Attempts[len(state.Attempts)-1]
			if last.Retryable == nil || *last.Retryable {
				return fmt.Errorf("failed judge stopped before max attempts without a non-retryable final attempt")
			}
		}
		for _, attempt := range state.Attempts {
			if attempt.Status != longMemEvalQAAttemptFailed {
				return fmt.Errorf("failed judge contains non-failed attempt %d", attempt.Number)
			}
		}
	case longMemEvalQAJudgeInvalid:
		if state.Correct != nil || len(state.Attempts) > config.MaxAttempts {
			return fmt.Errorf("invalid judge state is incomplete")
		}
		if len(state.Attempts) < config.MaxAttempts {
			last := state.Attempts[len(state.Attempts)-1]
			if last.Retryable == nil || *last.Retryable {
				return fmt.Errorf("invalid judge stopped before max attempts without a non-retryable final attempt")
			}
		}
		invalid := false
		for _, attempt := range state.Attempts {
			if attempt.Status == longMemEvalQAAttemptInvalid {
				invalid = true
			}
		}
		if !invalid {
			return fmt.Errorf("invalid judge state contains no invalid attempt")
		}
	default:
		return fmt.Errorf("checkpoint judge status is %q", state.Status)
	}
	for index, attempt := range state.Attempts {
		if attempt.Number != index+1 || len(attempt.Error) > 1000 {
			return fmt.Errorf("checkpoint judge attempt %d is invalid", index+1)
		}
	}
	return nil
}

func longMemEvalQAJudgePromptSHA(record benchmark.LongMemEvalRecord, checkpoint LongMemEvalQACheckpoint) (string, error) {
	if checkpoint.ReaderStatus == longMemEvalQAReaderFailed {
		return "", nil
	}
	prompt, err := benchmark.LongMemEvalJudgePrompt(record, checkpoint.Response)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(digest[:]), nil
}

func (runtime *longMemEvalQAJudgeRuntime) addSummary(state LongMemEvalQAJudgeState, resumed bool) bool {
	runtime.summaryMu.Lock()
	defer runtime.summaryMu.Unlock()
	runtime.summary.Total++
	if resumed {
		runtime.summary.Resumed++
	} else {
		runtime.summary.ProviderCalls += len(state.Attempts)
	}
	switch state.Status {
	case longMemEvalQAJudgeCompleted:
		runtime.summary.Judged++
	case longMemEvalQAJudgeFailed:
		runtime.summary.Failed++
	case longMemEvalQAJudgeInvalid:
		runtime.summary.Invalid++
	case longMemEvalQAJudgeNotRunReaderFailed:
		runtime.summary.NotRun++
	}
	limit := runtime.opts.Execution.Judge.MaxTerminalFailures
	terminal := runtime.summary.Failed + runtime.summary.Invalid + runtime.summary.NotRun
	return limit > 0 && terminal >= limit
}
