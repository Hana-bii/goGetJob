package resume

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"goGetJob/internal/common/ai"
)

type AIAnalysisOptions struct {
	Model        ai.ChatModel
	PromptLoader *ai.PromptLoader
	SystemPrompt string
	UserPrompt   string
	MaxAttempts  int
}

type AIAnalysisService struct {
	model        ai.ChatModel
	promptLoader *ai.PromptLoader
	systemPrompt string
	userPrompt   string
	maxAttempts  int
}

func NewAIAnalysisService(options AIAnalysisOptions) *AIAnalysisService {
	loader := options.PromptLoader
	if loader == nil {
		loader = ai.NewPromptLoader("")
	}
	systemPrompt := options.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "resume-analysis-system.st"
	}
	userPrompt := options.UserPrompt
	if userPrompt == "" {
		userPrompt = "resume-analysis-user.st"
	}
	maxAttempts := options.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 2
	}
	return &AIAnalysisService{
		model:        options.Model,
		promptLoader: loader,
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
		maxAttempts:  maxAttempts,
	}
}

func (s *AIAnalysisService) Analyze(ctx context.Context, resumeText string) (AnalysisResult, error) {
	if s == nil || s.model == nil {
		return AnalysisResult{}, fmt.Errorf("analysis model is required")
	}
	system, err := s.promptLoader.Load(s.systemPrompt)
	if err != nil {
		return AnalysisResult{}, err
	}
	user, err := s.promptLoader.Render(s.userPrompt, map[string]string{"resumeText": resumeText})
	if err != nil {
		return AnalysisResult{}, err
	}
	prompt := system + "\n\n" + user
	var result AnalysisResult
	if err := ai.InvokeStructured(ctx, s.model, prompt, &result, ai.StructuredOptions{
		MaxAttempts:       s.maxAttempts,
		InjectLastError:   true,
		RepairInstruction: "Return strict JSON matching the resume analysis schema only.",
	}); err != nil {
		return AnalysisResult{}, err
	}
	result.OriginalText = resumeText
	return result, nil
}

func analysisToEntity(resumeID uint, result AnalysisResult) *ResumeAnalysis {
	strengths, _ := json.Marshal(nilSlice(result.Strengths))
	suggestions, _ := json.Marshal(nilSlice(result.Suggestions))
	return &ResumeAnalysis{
		ResumeID:        resumeID,
		OverallScore:    result.OverallScore,
		ContentScore:    result.ScoreDetail.ContentScore,
		StructureScore:  result.ScoreDetail.StructureScore,
		SkillMatchScore: result.ScoreDetail.SkillMatchScore,
		ExpressionScore: result.ScoreDetail.ExpressionScore,
		ProjectScore:    result.ScoreDetail.ProjectScore,
		Summary:         result.Summary,
		StrengthsJSON:   string(strengths),
		SuggestionsJSON: string(suggestions),
	}
}

func analysisFromEntity(entity *ResumeAnalysis, originalText string) (AnalysisResult, error) {
	if entity == nil {
		return AnalysisResult{}, ErrNotFound
	}
	var strengths []string
	if entity.StrengthsJSON != "" {
		if err := json.Unmarshal([]byte(entity.StrengthsJSON), &strengths); err != nil {
			return AnalysisResult{}, err
		}
	}
	var suggestions []Suggestion
	if entity.SuggestionsJSON != "" {
		if err := json.Unmarshal([]byte(entity.SuggestionsJSON), &suggestions); err != nil {
			return AnalysisResult{}, err
		}
	}
	return AnalysisResult{
		OverallScore: entity.OverallScore,
		ScoreDetail: ScoreDetail{
			ContentScore:    entity.ContentScore,
			StructureScore:  entity.StructureScore,
			SkillMatchScore: entity.SkillMatchScore,
			ExpressionScore: entity.ExpressionScore,
			ProjectScore:    entity.ProjectScore,
		},
		Summary:      entity.Summary,
		Strengths:    strengths,
		Suggestions:  suggestions,
		OriginalText: originalText,
	}, nil
}

func latestAnalysisResult(ctx context.Context, repo Repository, resume *Resume) (*AnalysisResult, error) {
	analysis, err := repo.LatestAnalysis(ctx, resume.ID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result, err := analysisFromEntity(analysis, resume.ResumeText)
	return &result, err
}

func nilSlice[T any](value []T) []T {
	if value == nil {
		return []T{}
	}
	return value
}
