package eval

import "testing"

func TestScoreOutputTracksIncludesAndForbiddenPhrases(t *testing.T) {
	task := Task{
		ID:             "self-case-stale-context",
		MustInclude:    []string{"ContextMesh", "Bluebridge Cup"},
		MustNotInclude: []string{"based on MemOS"},
	}
	score := ScoreOutput(task, "ContextMesh is a Bluebridge Cup project, not based on MemOS.")
	if score.MustIncludeHitRate != 1 {
		t.Fatalf("expected hit rate 1, got %f", score.MustIncludeHitRate)
	}
	if len(score.MustNotIncludeViolations) != 1 {
		t.Fatalf("expected one forbidden violation, got %d", len(score.MustNotIncludeViolations))
	}
}

func TestScoreOutputAddsStructuredAcceptanceMetrics(t *testing.T) {
	task := Task{
		ID:             "workspace-continuity",
		MustInclude:    []string{"workspace", "continuity"},
		MustNotInclude: []string{"hallucinated"},
	}

	score := ScoreOutput(task, "The workspace continuity view is grounded in repository state.")

	if score.Continuation != 1 {
		t.Fatalf("expected continuation 1, got %f", score.Continuation)
	}
	if score.Groundedness != 1 {
		t.Fatalf("expected groundedness 1, got %f", score.Groundedness)
	}
	if score.TargetFitness != 1 {
		t.Fatalf("expected target fitness 1, got %f", score.TargetFitness)
	}
}

func TestScoreOutputUsesConservativeProxyMetricsForPartialMatches(t *testing.T) {
	task := Task{
		ID:             "workspace-continuity-partial",
		MustInclude:    []string{"workspace", "continuity"},
		MustNotInclude: []string{"hallucinated", "fabricated"},
	}

	score := ScoreOutput(task, "The workspace summary is hallucinated.")

	if score.MustIncludeHitRate != 0.5 {
		t.Fatalf("expected hit rate 0.5, got %f", score.MustIncludeHitRate)
	}
	if len(score.MissingIncludes) != 1 {
		t.Fatalf("expected one missing include, got %d", len(score.MissingIncludes))
	}
	if score.Groundedness != 0.5 {
		t.Fatalf("expected groundedness proxy 0.5, got %f", score.Groundedness)
	}
	if len(score.MustNotIncludeViolations) != 1 {
		t.Fatalf("expected one forbidden violation, got %d", len(score.MustNotIncludeViolations))
	}
	if score.TargetFitness != 0.5 {
		t.Fatalf("expected conservative target fitness 0.5, got %f", score.TargetFitness)
	}
}

func TestScoreOutputWithoutForbiddenChecksDefaultsGroundednessProxy(t *testing.T) {
	task := Task{
		ID:          "workspace-continuity-no-forbidden",
		MustInclude: []string{"workspace", "continuity"},
	}

	score := ScoreOutput(task, "The workspace summary is current.")

	if score.MustIncludeHitRate != 0.5 {
		t.Fatalf("expected hit rate 0.5, got %f", score.MustIncludeHitRate)
	}
	if score.Groundedness != 1 {
		t.Fatalf("expected groundedness proxy 1 with no forbidden checks, got %f", score.Groundedness)
	}
	if score.TargetFitness != 0.5 {
		t.Fatalf("expected target fitness to stay conservative at 0.5, got %f", score.TargetFitness)
	}
}

func TestScoreOutputAcceptsDeclaredAlternativeFactPhrasings(t *testing.T) {
	task := Task{
		ID: "conversation-fact-aliases",
		MustIncludeAny: [][]string{
			{"Seattle", "西雅图"},
			{"one-bedroom", "一居"},
			{"pet-friendly", "能养", "可养", "宠物友好"},
		},
	}

	score := ScoreOutput(task, "当前主线是西雅图租房：一居优先，且房源必须可养中型犬。")

	if score.MustIncludeHitRate != 1 {
		t.Fatalf("expected declared fact aliases to satisfy all requirements, got %#v", score)
	}
	if len(score.MissingIncludes) != 0 {
		t.Fatalf("expected no missing requirements, got %#v", score.MissingIncludes)
	}
}
