package voiceinterview

import (
	"context"
	"fmt"
	"strings"

	"goGetJob/internal/common/ai"
)

type PromptServiceOptions struct {
	Model        ai.ChatModel
	PromptLoader *ai.PromptLoader
}

type PromptService struct {
	model        ai.ChatModel
	promptLoader *ai.PromptLoader
}

type PromptInput struct {
	ResumeText    string
	JDText        string
	SkillID       string
	Difficulty    string
	QuestionCount int
	Phase         string
}

type promptQuestionListDTO struct {
	Questions []PromptQuestion `json:"questions"`
}

func NewPromptService(options PromptServiceOptions) *PromptService {
	loader := options.PromptLoader
	if loader == nil {
		loader = ai.NewPromptLoader("internal/prompts")
	}
	return &PromptService{model: options.Model, promptLoader: loader}
}

func (s *PromptService) GeneratePrompts(ctx context.Context, input PromptInput) ([]PromptQuestion, error) {
	count := input.QuestionCount
	if count <= 0 {
		count = DefaultQuestionCount
	}
	if s == nil || s.model == nil {
		return fallbackPromptQuestions(count, input.Phase), nil
	}
	prompt := s.buildQuestionPrompt(input, count)
	var dto promptQuestionListDTO
	if err := ai.InvokeStructured(ctx, s.model, prompt, &dto, ai.StructuredOptions{MaxAttempts: 2, InjectLastError: true, RepairInstruction: "Return strict JSON only matching the voice interview question schema."}); err != nil {
		return fallbackPromptQuestions(count, input.Phase), nil
	}
	out := sanitizePromptQuestions(dto.Questions, count, input.Phase)
	if len(out) == 0 {
		return fallbackPromptQuestions(count, input.Phase), nil
	}
	return out, nil
}

func (s *PromptService) BuildReplyPrompt(session VoiceSession, messages []VoiceMessage) string {
	var b strings.Builder
	b.WriteString("You are conducting a concise voice interview. Current phase: ")
	b.WriteString(nonEmpty(session.CurrentPhase, DefaultPhase))
	b.WriteString(". Reply with one short interviewer message and ask at most one question.\n\nDialogue:\n")
	for _, message := range messages {
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		b.WriteString(string(message.Role))
		b.WriteString(": ")
		b.WriteString(message.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func (s *PromptService) buildQuestionPrompt(input PromptInput, count int) string {
	return fmt.Sprintf(`Generate %d voice interview prompts as strict JSON: {"questions":[{"index":0,"question":"","category":"","phase":""}]}.
Difficulty: %s
Skill: %s
Phase: %s
Resume: %s
JD: %s`, count, nonEmpty(input.Difficulty, DefaultDifficulty), input.SkillID, nonEmpty(input.Phase, DefaultPhase), input.ResumeText, input.JDText)
}

func fallbackPromptQuestions(count int, phase string) []PromptQuestion {
	if count <= 0 {
		count = DefaultQuestionCount
	}
	phase = nonEmpty(phase, DefaultPhase)
	questions := make([]PromptQuestion, 0, count)
	for i := 0; i < count; i++ {
		questions = append(questions, PromptQuestion{Index: i, Question: fallbackQuestion(i), Category: "General", Phase: phase})
	}
	return questions
}

func fallbackQuestion(index int) string {
	switch index {
	case 0:
		return "Please introduce one recent project and your main responsibility."
	case 1:
		return "What technical challenge did you solve in that project?"
	case 2:
		return "How did you verify the solution worked reliably?"
	default:
		return fmt.Sprintf("Please share another example that shows your backend engineering ability, topic %d.", index+1)
	}
}

func sanitizePromptQuestions(questions []PromptQuestion, limit int, phase string) []PromptQuestion {
	out := []PromptQuestion{}
	for _, question := range questions {
		if strings.TrimSpace(question.Question) == "" {
			continue
		}
		question.Index = len(out)
		question.Category = nonEmpty(question.Category, "General")
		question.Phase = nonEmpty(question.Phase, phase)
		out = append(out, question)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
