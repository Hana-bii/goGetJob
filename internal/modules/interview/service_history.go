package interview

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"goGetJob/internal/common/evaluation"
	"goGetJob/internal/infrastructure/export"
)

type HistoryService struct {
	repo        Repository
	pdfExporter export.PDFExporter
}

func NewHistoryService(repo Repository, pdfExporter export.PDFExporter) *HistoryService {
	return &HistoryService{repo: repo, pdfExporter: pdfExporter}
}

func (s *HistoryService) Detail(ctx context.Context, sessionID string) (InterviewDetail, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return InterviewDetail{}, err
	}
	questions, err := parseQuestions(session.QuestionsJSON)
	if err != nil {
		return InterviewDetail{}, err
	}
	answers, err := s.repo.ListAnswers(ctx, sessionID)
	if err != nil {
		return InterviewDetail{}, err
	}
	return buildDetail(session, questions, answers), nil
}

func (s *HistoryService) Export(ctx context.Context, sessionID string) ([]byte, string, error) {
	if s == nil || s.pdfExporter == nil {
		return nil, "", fmt.Errorf("pdf exporter is required")
	}
	detail, err := s.Detail(ctx, sessionID)
	if err != nil {
		return nil, "", err
	}
	report := export.Report{
		Title: "Interview Report " + detail.SessionID,
		Sections: []export.ReportSection{
			{Heading: "Score", Body: fmt.Sprintf("%d / 100", detail.OverallScore)},
			{Heading: "Feedback", Body: detail.OverallFeedback},
			{Heading: "Strengths", Body: strings.Join(detail.Strengths, "\n")},
			{Heading: "Improvements", Body: strings.Join(detail.Improvements, "\n")},
			{Heading: "Answers", Body: answerSummary(detail.Answers)},
		},
	}
	pdf, err := s.pdfExporter.ExportReport(ctx, report)
	if err != nil {
		return nil, "", err
	}
	return pdf, "interview-report-" + sessionID + ".pdf", nil
}

func buildDetail(session *Session, questions []Question, answers []Answer) InterviewDetail {
	answerByIndex := map[int]Answer{}
	for _, answer := range answers {
		answerByIndex[answer.QuestionIndex] = answer
	}
	return InterviewDetail{
		ID:               session.ID,
		SessionID:        session.SessionID,
		TotalQuestions:   session.TotalQuestions,
		Status:           session.Status,
		EvaluateStatus:   session.EvaluateStatus,
		EvaluateError:    session.EvaluateError,
		OverallScore:     session.OverallScore,
		OverallFeedback:  session.OverallFeedback,
		CreatedAt:        session.CreatedAt,
		CompletedAt:      session.CompletedAt,
		Questions:        questions,
		Strengths:        parseStringSlice(session.StrengthsJSON),
		Improvements:     parseStringSlice(session.ImprovementsJSON),
		ReferenceAnswers: parseReferenceAnswers(session.ReferenceAnswersJSON),
		Answers:          buildAnswerDetails(questions, answerByIndex),
	}
}

func buildAnswerDetails(questions []Question, answerByIndex map[int]Answer) []AnswerDetail {
	details := make([]AnswerDetail, 0, len(questions))
	for _, question := range questions {
		answer := answerByIndex[question.QuestionIndex]
		details = append(details, AnswerDetail{
			QuestionIndex:   question.QuestionIndex,
			Question:        nonEmpty(answer.Question, question.Question),
			Category:        nonEmpty(answer.Category, question.Category),
			UserAnswer:      answer.UserAnswer,
			Score:           answer.Score,
			Feedback:        answer.Feedback,
			ReferenceAnswer: answer.ReferenceAnswer,
			KeyPoints:       parseStringSlice(answer.KeyPointsJSON),
			AnsweredAt:      answer.AnsweredAt,
		})
	}
	return details
}

func answerSummary(answers []AnswerDetail) string {
	lines := []string{}
	for _, answer := range answers {
		lines = append(lines, fmt.Sprintf("Q%d: %s\nAnswer: %s\nScore: %d\nFeedback: %s",
			answer.QuestionIndex+1, answer.Question, answer.UserAnswer, answer.Score, answer.Feedback))
	}
	return strings.Join(lines, "\n\n")
}

func parseStringSlice(encoded string) []string {
	if strings.TrimSpace(encoded) == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(encoded), &out)
	return out
}

func parseReferenceAnswers(encoded string) []evaluation.ReferenceAnswer {
	if strings.TrimSpace(encoded) == "" {
		return nil
	}
	var out []evaluation.ReferenceAnswer
	_ = json.Unmarshal([]byte(encoded), &out)
	return out
}
