package benchmark

import (
	"fmt"
	"math"
	"strings"
)

type SessionRetrievalMetric struct {
	K                 int     `json:"k"`
	RecallAny         float64 `json:"recall_any"`
	RecallAll         float64 `json:"recall_all"`
	NDCG              float64 `json:"ndcg_any"`
	FirstRelevantRank int     `json:"first_relevant_rank"`
	ReciprocalRank    float64 `json:"reciprocal_rank"`
}

type RetrievalAggregate struct {
	Count         int     `json:"count"`
	MeanRecallAny float64 `json:"mean_recall_any"`
	MeanRecallAll float64 `json:"mean_recall_all"`
	MeanNDCG      float64 `json:"mean_ndcg_any"`
	MeanMRR       float64 `json:"mean_mrr"`
}

func EvaluateSessionRetrieval(ranked, relevant []string, k int) (SessionRetrievalMetric, error) {
	if k <= 0 {
		return SessionRetrievalMetric{}, fmt.Errorf("retrieval K must be positive")
	}
	relevantSet, err := sessionIDSet(relevant, "relevant")
	if err != nil {
		return SessionRetrievalMetric{}, err
	}
	if len(relevantSet) == 0 {
		return SessionRetrievalMetric{}, fmt.Errorf("at least one relevant session is required")
	}
	if err := validateSessionIDs(ranked, "ranked"); err != nil {
		return SessionRetrievalMetric{}, err
	}

	limit := k
	if limit > len(ranked) {
		limit = len(ranked)
	}
	recalled := make(map[string]struct{}, limit)
	actualDCG := 0.0
	firstRelevantRank := 0
	for index, sessionID := range ranked[:limit] {
		if _, relevant := relevantSet[sessionID]; !relevant {
			continue
		}
		recalled[sessionID] = struct{}{}
		actualDCG += retrievalDiscount(index)
		if firstRelevantRank == 0 {
			firstRelevantRank = index + 1
		}
	}

	recallAny := 0.0
	if len(recalled) > 0 {
		recallAny = 1
	}
	recallAll := 0.0
	if len(recalled) == len(relevantSet) {
		recallAll = 1
	}
	idealCount := len(relevantSet)
	if idealCount > k {
		idealCount = k
	}
	idealDCG := 0.0
	for index := 0; index < idealCount; index++ {
		idealDCG += retrievalDiscount(index)
	}
	ndcg := 0.0
	if idealDCG > 0 {
		ndcg = actualDCG / idealDCG
	}
	reciprocalRank := 0.0
	if firstRelevantRank > 0 {
		reciprocalRank = 1 / float64(firstRelevantRank)
	}
	return SessionRetrievalMetric{
		K:                 k,
		RecallAny:         recallAny,
		RecallAll:         recallAll,
		NDCG:              ndcg,
		FirstRelevantRank: firstRelevantRank,
		ReciprocalRank:    reciprocalRank,
	}, nil
}

func AggregateSessionRetrieval(metrics []SessionRetrievalMetric) RetrievalAggregate {
	aggregate := RetrievalAggregate{Count: len(metrics)}
	if len(metrics) == 0 {
		return aggregate
	}
	for _, metric := range metrics {
		aggregate.MeanRecallAny += metric.RecallAny
		aggregate.MeanRecallAll += metric.RecallAll
		aggregate.MeanNDCG += metric.NDCG
		aggregate.MeanMRR += metric.ReciprocalRank
	}
	count := float64(len(metrics))
	aggregate.MeanRecallAny = roundRetrievalMetric(aggregate.MeanRecallAny / count)
	aggregate.MeanRecallAll = roundRetrievalMetric(aggregate.MeanRecallAll / count)
	aggregate.MeanNDCG = roundRetrievalMetric(aggregate.MeanNDCG / count)
	aggregate.MeanMRR = roundRetrievalMetric(aggregate.MeanMRR / count)
	return aggregate
}

func sessionIDSet(ids []string, label string) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, fmt.Errorf("%s session IDs cannot contain an empty value", label)
		}
		result[id] = struct{}{}
	}
	return result, nil
}

func validateSessionIDs(ids []string, label string) error {
	for _, rawID := range ids {
		if strings.TrimSpace(rawID) == "" {
			return fmt.Errorf("%s session IDs cannot contain an empty value", label)
		}
	}
	return nil
}

func retrievalDiscount(index int) float64 {
	if index == 0 {
		return 1
	}
	return 1 / math.Log2(float64(index+1))
}

func roundRetrievalMetric(value float64) float64 {
	return math.Round(value*10000) / 10000
}
