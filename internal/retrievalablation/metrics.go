package retrievalablation

import (
	"math"
	"sort"
	"time"
)

func ScoreQuery(query Query, results []RankedResult) QueryMetrics {
	relevant := make(map[string]struct{}, len(query.RelevantRecordIDs))
	for _, recordID := range query.RelevantRecordIDs {
		relevant[recordID] = struct{}{}
	}
	forbidden := make(map[string]struct{}, len(query.ForbiddenRecordIDs))
	for _, recordID := range query.ForbiddenRecordIDs {
		forbidden[recordID] = struct{}{}
	}
	foundRelevant := make(map[string]struct{}, len(relevant))
	foundForbidden := make(map[string]struct{}, len(forbidden))
	foundIneligible := make(map[string]struct{})
	metrics := QueryMetrics{}
	dcg := 0.0
	for index, result := range results {
		rank := index + 1
		if _, exists := relevant[result.RecordID]; exists {
			if _, counted := foundRelevant[result.RecordID]; !counted {
				foundRelevant[result.RecordID] = struct{}{}
				dcg += 1 / math.Log2(float64(rank+1))
				if metrics.MRR == 0 {
					metrics.MRR = 1 / float64(rank)
				}
			}
		}
		if _, exists := forbidden[result.RecordID]; exists {
			foundForbidden[result.RecordID] = struct{}{}
		}
		if !result.Eligible {
			foundIneligible[result.RecordID] = struct{}{}
		}
	}
	if len(results) > 0 {
		if _, exists := relevant[results[0].RecordID]; exists {
			metrics.HitAt1 = 1
		}
	}
	metrics.RecallAtK = float64(len(foundRelevant)) / float64(len(relevant))
	idcg := 0.0
	idealCount := len(relevant)
	idealLimit := query.Limit
	if idealLimit <= 0 {
		idealLimit = len(results)
	}
	if idealLimit < idealCount {
		idealCount = idealLimit
	}
	for index := 0; index < idealCount; index++ {
		idcg += 1 / math.Log2(float64(index+2))
	}
	if idcg > 0 {
		metrics.NDCGAtK = dcg / idcg
	}
	metrics.ForbiddenCount = len(foundForbidden)
	metrics.IneligibleCount = len(foundIneligible)
	return metrics
}

func AggregateMetrics(reports []QueryReport) Aggregate {
	aggregate := Aggregate{QueryCount: len(reports)}
	if len(reports) == 0 {
		return aggregate
	}
	durations := make([]time.Duration, 0, len(reports))
	for _, report := range reports {
		aggregate.HitAt1 += report.Metrics.HitAt1
		aggregate.RecallAtK += report.Metrics.RecallAtK
		aggregate.MRR += report.Metrics.MRR
		aggregate.NDCGAtK += report.Metrics.NDCGAtK
		aggregate.ForbiddenCount += report.Metrics.ForbiddenCount
		aggregate.IneligibleCount += report.Metrics.IneligibleCount
		durations = append(durations, report.Duration)
	}
	count := float64(len(reports))
	aggregate.HitAt1 /= count
	aggregate.RecallAtK /= count
	aggregate.MRR /= count
	aggregate.NDCGAtK /= count
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	aggregate.SearchP50 = durationPercentile(durations, 0.50)
	aggregate.SearchP95 = durationPercentile(durations, 0.95)
	return aggregate
}

func AggregateCohorts(reports []QueryReport) map[string]Aggregate {
	grouped := make(map[string][]QueryReport)
	for _, report := range reports {
		for _, cohort := range report.Cohorts {
			grouped[cohort] = append(grouped[cohort], report)
		}
	}
	result := make(map[string]Aggregate, len(grouped))
	for cohort, cohortReports := range grouped {
		result[cohort] = AggregateMetrics(cohortReports)
	}
	return result
}

func durationPercentile(values []time.Duration, percentile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * percentile)
	return values[index]
}
