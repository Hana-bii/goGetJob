package interview

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"goGetJob/internal/common/ai"
	"goGetJob/internal/modules/interview/skill"
)

const (
	defaultQuestionType = "GENERAL"
	maxFollowUpCount    = 2
	resumeQuestionRatio = 0.6
)

var difficultyDescriptions = map[string]string{
	"junior": "Campus or 0-1 years. Focus on fundamentals and simple application.",
	"mid":    "1-3 years. Focus on principles and practical experience.",
	"senior": "3+ years. Focus on architecture design and deep optimization.",
}

type QuestionServiceOptions struct {
	Model         ai.ChatModel
	ResumeModel   ai.ChatModel
	SkillService  *skill.Service
	PromptLoader  *ai.PromptLoader
	FollowUpCount int
	MaxAttempts   int
}

type QuestionService struct {
	model         ai.ChatModel
	resumeModel   ai.ChatModel
	skillService  *skill.Service
	promptLoader  *ai.PromptLoader
	followUpCount int
	maxAttempts   int
}

type GenerateQuestionsInput struct {
	SkillID             string
	Difficulty          string
	ResumeText          string
	QuestionCount       int
	HistoricalQuestions []HistoricalQuestion
	CustomCategories    []skill.Category
	JDText              string
}

type questionListDTO struct {
	Questions []questionDTO `json:"questions"`
}

type questionDTO struct {
	Question     string   `json:"question"`
	Type         string   `json:"type"`
	Category     string   `json:"category"`
	TopicSummary string   `json:"topicSummary"`
	FollowUps    []string `json:"followUps"`
}

func NewQuestionService(options QuestionServiceOptions) *QuestionService {
	loader := options.PromptLoader
	if loader == nil {
		loader = ai.NewPromptLoader("internal/prompts")
	}
	resumeModel := options.ResumeModel
	if resumeModel == nil {
		resumeModel = options.Model
	}
	followUps := options.FollowUpCount
	if followUps < 0 {
		followUps = 0
	}
	if followUps > maxFollowUpCount {
		followUps = maxFollowUpCount
	}
	maxAttempts := options.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 2
	}
	return &QuestionService{
		model:         options.Model,
		resumeModel:   resumeModel,
		skillService:  options.SkillService,
		promptLoader:  loader,
		followUpCount: followUps,
		maxAttempts:   maxAttempts,
	}
}

func (s *QuestionService) Generate(ctx context.Context, input GenerateQuestionsInput) ([]Question, error) {
	if s == nil || s.skillService == nil {
		return nil, fmt.Errorf("question skill service is required")
	}
	count := input.QuestionCount
	if count <= 0 {
		count = DefaultQuestionCount
	}
	skillDef, err := s.resolveSkill(input)
	if err != nil {
		return nil, err
	}
	difficulty := difficultyDescriptions[nonEmpty(input.Difficulty, DefaultDifficulty)]
	if difficulty == "" {
		difficulty = difficultyDescriptions[DefaultDifficulty]
	}
	history := buildHistoricalSection(input.HistoricalQuestions)
	if strings.TrimSpace(input.ResumeText) == "" {
		return s.generateDirectionOnly(ctx, s.model, skillDef, difficulty, count, history)
	}

	resumeCount := int(math.Round(float64(count) * resumeQuestionRatio))
	if resumeCount < 1 {
		resumeCount = 1
	}
	if resumeCount > count {
		resumeCount = count
	}
	directionCount := count - resumeCount

	var wg sync.WaitGroup
	var resumeQuestions, directionQuestions []Question
	var resumeErr, directionErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		resumeQuestions, resumeErr = s.generateResumeQuestions(ctx, skillDef, difficulty, input.ResumeText, resumeCount, history)
	}()
	if directionCount > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			directionQuestions, directionErr = s.generateDirectionOnly(ctx, s.model, skillDef, difficulty, directionCount, history)
		}()
	}
	wg.Wait()

	if resumeErr != nil {
		return s.generateDirectionOnly(ctx, s.model, skillDef, difficulty, count, history)
	}
	if directionErr != nil {
		if len(resumeQuestions) == 0 {
			return s.generateFallbackQuestions(skillDef, count), nil
		}
		return resumeQuestions, nil
	}
	if len(resumeQuestions) == 0 && len(directionQuestions) == 0 {
		return s.generateFallbackQuestions(skillDef, count), nil
	}
	return mergeQuestionBatches(resumeQuestions, directionQuestions), nil
}

func (s *QuestionService) generateResumeQuestions(ctx context.Context, skillDef skill.Skill, difficulty, resumeText string, count int, history string) ([]Question, error) {
	if s.resumeModel == nil {
		return nil, fmt.Errorf("resume question model is required")
	}
	system, err := s.promptLoader.Load("interview-question-resume-system.st")
	if err != nil {
		return nil, err
	}
	user, err := s.promptLoader.Render("interview-question-resume-user.st", map[string]string{
		"questionCount":         fmt.Sprint(count),
		"followUpCount":         fmt.Sprint(s.followUpCount),
		"skillName":             skillDef.Name,
		"skillDescription":      skillDef.Description,
		"difficultyDescription": difficulty,
		"resumeText":            resumeText,
		"historicalSection":     history,
	})
	if err != nil {
		return nil, err
	}
	return s.invokeQuestions(ctx, s.resumeModel, system, user, count)
}

func (s *QuestionService) generateDirectionOnly(ctx context.Context, model ai.ChatModel, skillDef skill.Skill, difficulty string, count int, history string) ([]Question, error) {
	if model == nil {
		return s.generateFallbackQuestions(skillDef, count), nil
	}
	allocation := s.skillService.CalculateAllocation(skillDef.Categories, count)
	system, err := s.promptLoader.Load("interview-question-skill-system.st")
	if err != nil {
		return nil, err
	}
	system += genericModeSystemAppend() + "\n\n"
	user, err := s.promptLoader.Render("interview-question-skill-user.st", map[string]string{
		"questionCount":         fmt.Sprint(count),
		"followUpCount":         fmt.Sprint(s.followUpCount),
		"difficultyDescription": difficulty,
		"skillName":             skillDef.Name,
		"skillDescription":      skillDef.Description,
		"skillToolCommand":      skillDef.ID,
		"allocationTable":       buildAllocationDescription(allocation, skillDef.Categories),
		"historicalSection":     history,
		"referenceSection":      s.skillService.BuildReferenceSectionSafe(skillDef.ID, allocation),
		"jdSection":             buildJDSection(skillDef.SourceJD),
	})
	if err != nil {
		return nil, err
	}
	questions, err := s.invokeQuestions(ctx, model, system, user, count)
	if err != nil || countMainQuestions(questions) == 0 {
		return s.generateFallbackQuestions(skillDef, count), nil
	}
	return questions, nil
}

func (s *QuestionService) invokeQuestions(ctx context.Context, model ai.ChatModel, system, user string, count int) ([]Question, error) {
	var dto questionListDTO
	prompt := system + "\n\n" + questionFormatInstruction() + "\n\n" + user
	if err := ai.InvokeStructured(ctx, model, prompt, &dto, ai.StructuredOptions{
		MaxAttempts:       s.maxAttempts,
		InjectLastError:   true,
		RepairInstruction: "Return strict JSON only matching the requested interview question schema.",
	}); err != nil {
		return nil, err
	}
	return capToMainCount(s.convertToQuestions(dto), count), nil
}

func (s *QuestionService) convertToQuestions(dto questionListDTO) []Question {
	questions := []Question{}
	index := 0
	for _, item := range dto.Questions {
		if strings.TrimSpace(item.Question) == "" {
			continue
		}
		qtype := strings.ToUpper(nonEmpty(item.Type, defaultQuestionType))
		mainIndex := index
		questions = append(questions, NewQuestion(index, item.Question, qtype, item.Category, item.TopicSummary, false, nil))
		index++
		for _, followUp := range sanitizeFollowUps(item.FollowUps, s.followUpCount) {
			parent := mainIndex
			questions = append(questions, NewQuestion(index, followUp, qtype, buildFollowUpCategory(item.Category, index-mainIndex), "", true, &parent))
			index++
		}
	}
	return questions
}

func (s *QuestionService) generateFallbackQuestions(skillDef skill.Skill, count int) []Question {
	if count <= 0 {
		count = DefaultQuestionCount
	}
	categories := skillDef.Categories
	if len(categories) == 0 {
		categories = []skill.Category{{Key: defaultQuestionType, Label: "General"}}
	}
	questions := []Question{}
	index := 0
	for i := 0; i < count; i++ {
		category := categories[i%len(categories)]
		label := nonEmpty(category.Label, category.Key)
		question := fmt.Sprintf("Please describe your understanding and practical experience in %s.", label)
		questions = append(questions, NewQuestion(index, question, nonEmpty(category.Key, defaultQuestionType), label, label, false, nil))
		mainIndex := index
		index++
		for j := 0; j < s.followUpCount; j++ {
			parent := mainIndex
			questions = append(questions, NewQuestion(index, defaultFollowUp(question, j+1), nonEmpty(category.Key, defaultQuestionType), buildFollowUpCategory(label, j+1), "", true, &parent))
			index++
		}
	}
	return questions
}

func (s *QuestionService) resolveSkill(input GenerateQuestionsInput) (skill.Skill, error) {
	skillID := nonEmpty(input.SkillID, DefaultSkillID)
	if skillID == skill.CustomSkillID && len(input.CustomCategories) > 0 {
		return s.skillService.BuildCustom(input.CustomCategories, input.JDText), nil
	}
	return s.skillService.Get(skillID)
}

func mergeQuestionBatches(first, second []Question) []Question {
	if len(second) == 0 {
		return first
	}
	if len(first) == 0 {
		return second
	}
	offset := len(first)
	merged := append([]Question(nil), first...)
	for _, q := range second {
		q.QuestionIndex += offset
		if q.ParentQuestionIndex != nil {
			parent := *q.ParentQuestionIndex + offset
			q.ParentQuestionIndex = &parent
		}
		merged = append(merged, q)
	}
	return merged
}

func capToMainCount(questions []Question, maxMain int) []Question {
	if maxMain <= 0 {
		return questions
	}
	mainSeen := 0
	out := []Question{}
	for _, question := range questions {
		if !question.IsFollowUp {
			mainSeen++
		}
		if mainSeen > maxMain {
			break
		}
		out = append(out, question)
	}
	return out
}

func countMainQuestions(questions []Question) int {
	count := 0
	for _, question := range questions {
		if !question.IsFollowUp {
			count++
		}
	}
	return count
}

func sanitizeFollowUps(followUps []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := []string{}
	for _, item := range followUps {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func buildHistoricalSection(history []HistoricalQuestion) string {
	if len(history) == 0 {
		return "No historical questions."
	}
	grouped := map[string][]string{}
	for _, item := range history {
		qtype := nonEmpty(item.Type, defaultQuestionType)
		summary := item.TopicSummary
		if summary == "" {
			summary = truncateRunes(item.Question, 30)
		}
		grouped[qtype] = append(grouped[qtype], summary)
	}
	lines := []string{"Historical topics to avoid repeating:"}
	for qtype, topics := range grouped {
		lines = append(lines, "- "+qtype+": "+strings.Join(topics, ", "))
	}
	return strings.Join(lines, "\n")
}

func buildAllocationDescription(allocation map[string]int, categories []skill.Category) string {
	lines := []string{}
	for _, category := range categories {
		count := allocation[category.Key]
		if count <= 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("| %s | %d | %s |", nonEmpty(category.Label, category.Key), count, category.Priority))
	}
	return strings.Join(lines, "\n")
}

func buildJDSection(jd string) string {
	if strings.TrimSpace(jd) == "" {
		return ""
	}
	return "Use the following JD requirements while generating questions:\n" + jd
}

func buildFollowUpCategory(category string, order int) string {
	return nonEmpty(category, "Follow-up") + fmt.Sprintf(" (follow-up %d)", order)
}

func defaultFollowUp(mainQuestion string, order int) string {
	if order == 1 {
		return "Please expand with a concrete project example for: " + mainQuestion
	}
	return "If this caused a production issue, how would you diagnose and fix it?"
}

func questionFormatInstruction() string {
	return `Return strict JSON with this shape:
{"questions":[{"question":"","type":"","category":"","topicSummary":"","followUps":[]}]}`
}

func genericModeSystemAppend() string {
	return `
# Generic interview mode
No candidate resume is available. Generate standard interview questions for the selected direction.
- Do not imply the candidate mentioned anything in a resume or project.
- Questions should directly test the selected technical direction.`
}
