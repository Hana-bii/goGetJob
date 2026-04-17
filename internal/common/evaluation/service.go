package evaluation

import (
	"context"
	"fmt"
	"strings"

	"goGetJob/internal/common/ai"
)

const maxReferenceContextChars = 6000

type Options struct {
	Model        ai.ChatModel
	PromptLoader *ai.PromptLoader
	BatchSize    int
}

type Service struct {
	model        ai.ChatModel
	promptLoader *ai.PromptLoader
	batchSize    int
}

type batchReportDTO struct {
	OverallScore        int               `json:"overallScore"`
	OverallFeedback     string            `json:"overallFeedback"`
	Strengths           []string          `json:"strengths"`
	Improvements        []string          `json:"improvements"`
	QuestionEvaluations []questionEvalDTO `json:"questionEvaluations"`
}

type questionEvalDTO struct {
	QuestionIndex   int      `json:"questionIndex"`
	Score           int      `json:"score"`
	Feedback        string   `json:"feedback"`
	ReferenceAnswer string   `json:"referenceAnswer"`
	KeyPoints       []string `json:"keyPoints"`
}

type summaryDTO struct {
	OverallFeedback string   `json:"overallFeedback"`
	Strengths       []string `json:"strengths"`
	Improvements    []string `json:"improvements"`
}

type batchResult struct {
	start  int
	end    int
	report *batchReportDTO
}

func NewService(options Options) *Service {
	loader := options.PromptLoader
	if loader == nil {
		loader = ai.NewPromptLoader("")
	}
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = 8
	}
	return &Service{model: options.Model, promptLoader: loader, batchSize: batchSize}
}

func (s *Service) Evaluate(ctx context.Context, sessionID string, qaRecords []QaRecord, resumeText, referenceContext string) (Report, error) {
	if s == nil || s.model == nil {
		return Report{}, fmt.Errorf("evaluation model is required")
	}
	resumeText = truncate(resumeText, 3000)
	referenceContext = truncate(strings.TrimSpace(referenceContext), maxReferenceContextChars)
	batches := s.evaluateInBatches(ctx, qaRecords, resumeText, referenceContext)
	evaluations := mergeQuestionEvaluations(batches)
	fallbackFeedback := mergeFeedback(batches)
	fallbackStrengths := mergeItems(batches, true)
	fallbackImprovements := mergeItems(batches, false)
	summary := s.summarize(ctx, qaRecords, evaluations, resumeText, referenceContext, fallbackFeedback, fallbackStrengths, fallbackImprovements)
	return buildReport(sessionID, qaRecords, evaluations, summary), nil
}

func (s *Service) evaluateInBatches(ctx context.Context, qaRecords []QaRecord, resumeText, referenceContext string) []batchResult {
	results := []batchResult{}
	for start := 0; start < len(qaRecords); start += s.batchSize {
		end := start + s.batchSize
		if end > len(qaRecords) {
			end = len(qaRecords)
		}
		report, err := s.evaluateBatch(ctx, qaRecords[start:end], resumeText, referenceContext)
		if err != nil {
			report = nil
		}
		results = append(results, batchResult{start: start, end: end, report: report})
	}
	return results
}

func (s *Service) evaluateBatch(ctx context.Context, batch []QaRecord, resumeText, referenceContext string) (*batchReportDTO, error) {
	system, err := s.promptLoader.Load("interview-evaluation-system.st")
	if err != nil {
		return nil, err
	}
	user, err := s.promptLoader.Render("interview-evaluation-user.st", map[string]string{
		"resumeText":       resumeText,
		"qaRecords":        buildQARecords(batch),
		"referenceContext": nonEmpty(referenceContext, "无"),
	})
	if err != nil {
		return nil, err
	}
	var report batchReportDTO
	err = ai.InvokeStructured(ctx, s.model, system+"\n\n"+user, &report, ai.StructuredOptions{MaxAttempts: 1})
	return &report, err
}

func (s *Service) summarize(ctx context.Context, qaRecords []QaRecord, evaluations []questionEvalDTO, resumeText, referenceContext, fallbackFeedback string, fallbackStrengths, fallbackImprovements []string) summaryDTO {
	system, err := s.promptLoader.Load("interview-evaluation-summary-system.st")
	if err != nil {
		return summaryDTO{OverallFeedback: fallbackFeedback, Strengths: fallbackStrengths, Improvements: fallbackImprovements}
	}
	user, err := s.promptLoader.Render("interview-evaluation-summary-user.st", map[string]string{
		"resumeText":              resumeText,
		"referenceContext":        nonEmpty(referenceContext, "无"),
		"categorySummary":         buildCategorySummary(qaRecords, evaluations),
		"questionHighlights":      buildQuestionHighlights(qaRecords, evaluations),
		"fallbackOverallFeedback": fallbackFeedback,
		"fallbackStrengths":       strings.Join(fallbackStrengths, "\n"),
		"fallbackImprovements":    strings.Join(fallbackImprovements, "\n"),
	})
	if err != nil {
		return summaryDTO{OverallFeedback: fallbackFeedback, Strengths: fallbackStrengths, Improvements: fallbackImprovements}
	}
	var summary summaryDTO
	if err := ai.InvokeStructured(ctx, s.model, system+"\n\n"+user, &summary, ai.StructuredOptions{MaxAttempts: 1}); err != nil {
		return summaryDTO{OverallFeedback: fallbackFeedback, Strengths: fallbackStrengths, Improvements: fallbackImprovements}
	}
	if strings.TrimSpace(summary.OverallFeedback) == "" {
		summary.OverallFeedback = fallbackFeedback
	}
	if len(summary.Strengths) == 0 {
		summary.Strengths = fallbackStrengths
	}
	if len(summary.Improvements) == 0 {
		summary.Improvements = fallbackImprovements
	}
	return summary
}

func buildQARecords(batch []QaRecord) string {
	var builder strings.Builder
	for _, record := range batch {
		builder.WriteString(fmt.Sprintf("问题%d [%s]: %s\n", record.QuestionIndex+1, record.Category, record.Question))
		if record.UserAnswer == "" {
			builder.WriteString("回答: (未回答)\n\n")
		} else {
			builder.WriteString("回答: " + record.UserAnswer + "\n\n")
		}
	}
	return builder.String()
}

func mergeQuestionEvaluations(results []batchResult) []questionEvalDTO {
	merged := []questionEvalDTO{}
	for _, result := range results {
		current := []questionEvalDTO{}
		if result.report != nil {
			current = result.report.QuestionEvaluations
		}
		for i := result.start; i < result.end; i++ {
			relative := i - result.start
			if relative < len(current) {
				merged = append(merged, current[relative])
			} else {
				merged = append(merged, questionEvalDTO{QuestionIndex: i, Score: 0, Feedback: "该题未成功生成评估结果，系统按 0 分处理。"})
			}
		}
	}
	return merged
}

func mergeFeedback(results []batchResult) string {
	parts := []string{}
	for _, result := range results {
		if result.report != nil && strings.TrimSpace(result.report.OverallFeedback) != "" {
			parts = append(parts, result.report.OverallFeedback)
		}
	}
	if len(parts) == 0 {
		return "本次面试已完成分批评估，但未生成有效综合评语。"
	}
	return strings.Join(parts, "\n\n")
}

func mergeItems(results []batchResult, strengths bool) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, result := range results {
		if result.report == nil {
			continue
		}
		items := result.report.Improvements
		if strengths {
			items = result.report.Strengths
		}
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item != "" && !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	if len(out) == 0 && !strengths {
		return []string{"建议补充回答细节并结合项目经历展开。"}
	}
	return out
}

func buildReport(sessionID string, qaRecords []QaRecord, evaluations []questionEvalDTO, summary summaryDTO) Report {
	questionDetails := []QuestionEvaluation{}
	referenceAnswers := []ReferenceAnswer{}
	categoryScores := map[string][]int{}
	for i, record := range qaRecords {
		eval := questionEvalDTO{QuestionIndex: record.QuestionIndex}
		if i < len(evaluations) {
			eval = evaluations[i]
		}
		score := eval.Score
		if strings.TrimSpace(record.UserAnswer) == "" {
			score = 0
		}
		questionDetails = append(questionDetails, QuestionEvaluation{QuestionIndex: record.QuestionIndex, Question: record.Question, Category: record.Category, UserAnswer: record.UserAnswer, Score: score, Feedback: eval.Feedback})
		referenceAnswers = append(referenceAnswers, ReferenceAnswer{QuestionIndex: record.QuestionIndex, Question: record.Question, ReferenceAnswer: eval.ReferenceAnswer, KeyPoints: eval.KeyPoints})
		categoryScores[record.Category] = append(categoryScores[record.Category], score)
	}
	reportCategories := []CategoryScore{}
	for category, scores := range categoryScores {
		reportCategories = append(reportCategories, CategoryScore{Category: category, Score: average(scores), QuestionCount: len(scores)})
	}
	return Report{SessionID: sessionID, TotalQuestions: len(qaRecords), OverallScore: averageQuestionScores(questionDetails), CategoryScores: reportCategories, QuestionDetails: questionDetails, OverallFeedback: summary.OverallFeedback, Strengths: summary.Strengths, Improvements: summary.Improvements, ReferenceAnswers: referenceAnswers}
}

func buildCategorySummary(qaRecords []QaRecord, evaluations []questionEvalDTO) string {
	report := buildReport("", qaRecords, evaluations, summaryDTO{})
	lines := make([]string, 0, len(report.CategoryScores))
	for _, category := range report.CategoryScores {
		lines = append(lines, fmt.Sprintf("- %s: 平均分 %d, 题数 %d", category.Category, category.Score, category.QuestionCount))
	}
	return strings.Join(lines, "\n")
}

func buildQuestionHighlights(qaRecords []QaRecord, evaluations []questionEvalDTO) string {
	lines := []string{}
	for i, record := range qaRecords {
		score := 0
		feedback := ""
		if i < len(evaluations) {
			score = evaluations[i].Score
			feedback = evaluations[i].Feedback
		}
		lines = append(lines, fmt.Sprintf("- Q%d | %s | 分数:%d | 反馈:%s", record.QuestionIndex+1, truncate(record.Question, 50), score, truncate(feedback, 80)))
	}
	return strings.Join(lines, "\n")
}

func average(scores []int) int {
	if len(scores) == 0 {
		return 0
	}
	total := 0
	for _, score := range scores {
		total += score
	}
	return total / len(scores)
}

func averageQuestionScores(details []QuestionEvaluation) int {
	if len(details) == 0 {
		return 0
	}
	scores := make([]int, 0, len(details))
	for _, detail := range details {
		scores = append(scores, detail.Score)
	}
	return average(scores)
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
