package interviewschedule

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"goGetJob/internal/common/ai"
)

type ParseService struct {
	model ai.ChatModel
}

func NewParseService(model ai.ChatModel) *ParseService {
	return &ParseService{model: model}
}

func (s *ParseService) Parse(ctx context.Context, request ParseRequest) (ParseResponse, error) {
	rawText := strings.TrimSpace(request.RawText)
	if rawText == "" {
		return ParseResponse{
			Success:     false,
			Confidence:  0,
			ParseMethod: "none",
			Log:         "input text is empty",
		}, nil
	}

	if parsed, ok := s.parseByRegex(rawText, request.Source); ok {
		return ParseResponse{
			Success:     true,
			Data:        parsed,
			Confidence:  0.95,
			ParseMethod: "rule",
			Log:         "regex parsing succeeded",
		}, nil
	}

	if s.model == nil {
		return ParseResponse{
			Success:     false,
			Confidence:  0,
			ParseMethod: "none",
			Log:         "regex parsing failed and AI model is unavailable",
		}, nil
	}

	parsed, err := s.parseWithAI(ctx, rawText, request.Source)
	if err != nil {
		return ParseResponse{
			Success:     false,
			Confidence:  0,
			ParseMethod: "none",
			Log:         err.Error(),
		}, nil
	}

	return ParseResponse{
		Success:     true,
		Data:        parsed,
		Confidence:  0.8,
		ParseMethod: "ai",
		Log:         "AI parsing succeeded",
	}, nil
}

type aiParseResult struct {
	CompanyName   string `json:"companyName"`
	Position      string `json:"position"`
	InterviewTime string `json:"interviewTime"`
	InterviewType string `json:"interviewType"`
	MeetingLink   string `json:"meetingLink"`
	RoundNumber   int    `json:"roundNumber"`
	Interviewer   string `json:"interviewer"`
	Notes         string `json:"notes"`
}

func (s *ParseService) parseWithAI(ctx context.Context, rawText, source string) (*CreateInterviewRequest, error) {
	prompt := strings.TrimSpace(fmt.Sprintf(`
Extract interview schedule details from the invitation text below.

Rules:
- Return strict JSON only.
- Required fields: companyName, position, interviewTime.
- interviewTime must use the format 2006-01-02T15:04:05.
- Default interviewType to VIDEO if the text does not say otherwise.
- roundNumber should default to 1 when missing.
- Keep meetingLink, interviewer, and notes if present.
- source is %q.

Invitation text:
%s
`, source, rawText))

	var decoded aiParseResult
	if err := ai.InvokeStructured(ctx, s.model, prompt, &decoded, ai.StructuredOptions{
		MaxAttempts:       2,
		InjectLastError:   true,
		RepairInstruction: "Return strict JSON only with keys companyName, position, interviewTime, interviewType, meetingLink, roundNumber, interviewer, notes.",
	}); err != nil {
		return nil, err
	}

	request, err := aiResultToRequest(decoded)
	if err != nil {
		return nil, err
	}
	return request, nil
}

func aiResultToRequest(decoded aiParseResult) (*CreateInterviewRequest, error) {
	request := &CreateInterviewRequest{
		CompanyName:   strings.TrimSpace(decoded.CompanyName),
		Position:      strings.TrimSpace(decoded.Position),
		InterviewTime: strings.TrimSpace(decoded.InterviewTime),
		MeetingLink:   strings.TrimSpace(decoded.MeetingLink),
		RoundNumber:   decoded.RoundNumber,
		Interviewer:   strings.TrimSpace(decoded.Interviewer),
		Notes:         strings.TrimSpace(decoded.Notes),
	}
	if request.RoundNumber <= 0 {
		request.RoundNumber = defaultRoundNumber
	}
	if request.CompanyName == "" || request.Position == "" || request.InterviewTime == "" {
		return nil, fmt.Errorf("%w: AI output missing required schedule fields", ErrInvalidInput)
	}
	parsedTime, err := parseScheduleTime(request.InterviewTime)
	if err != nil {
		return nil, err
	}
	interviewType, err := normalizeInterviewType(decoded.InterviewType)
	if err != nil {
		return nil, err
	}
	request.InterviewType = interviewType
	request.InterviewTime = formatScheduleTime(parsedTime)
	return request, nil
}

func (s *ParseService) parseByRegex(rawText, source string) (*CreateInterviewRequest, bool) {
	request := &CreateInterviewRequest{
		InterviewType: defaultInterviewType,
		RoundNumber:   defaultRoundNumber,
	}

	request.CompanyName = firstMatch(rawText, []string{
		`(?im)(?:公司|公司名称|company(?: name)?|company)\s*[:：]\s*([^\n,，;；]{1,80})`,
		`(?im)(?:面试公司|企业)\s*[:：]\s*([^\n,，;；]{1,80})`,
	})
	request.Position = firstMatch(rawText, []string{
		`(?im)(?:岗位|职位|position|job title|role)\s*[:：]\s*([^\n,，;；]{1,120})`,
		`(?im)(?:应聘岗位|面试岗位)\s*[:：]\s*([^\n,，;；]{1,120})`,
	})
	request.InterviewTime = firstTime(rawText)
	request.MeetingLink = firstMatch(rawText, []string{
		`https?://meeting\.feishu\.cn/[^\s"']+`,
		`https?://zoom\.us/j/[^\s"']+`,
		`https?://meeting\.tencent\.com/[^\s"']+`,
		`https?://[^\s"']+`,
	})
	request.Interviewer = firstMatch(rawText, []string{
		`(?im)(?:面试官|interviewer)\s*[:：]\s*([^\n,，;；]{1,80})`,
	})
	request.Notes = firstMatch(rawText, []string{
		`(?im)(?:备注|notes?)\s*[:：]\s*([^\n]+)`,
	})

	if round := firstMatch(rawText, []string{`(?i)(?:第\s*)?([一二三四五六七八九十\d]+)\s*(?:轮|round)`}); round != "" {
		if parsed, ok := parseRoundNumber(round); ok {
			request.RoundNumber = parsed
		}
	}

	if request.InterviewTime != "" {
		if parsedTime, err := parseScheduleTime(request.InterviewTime); err == nil {
			request.InterviewTime = formatScheduleTime(parsedTime)
		}
	}
	if request.InterviewType == "" {
		request.InterviewType = inferInterviewType(rawText)
	}
	if request.MeetingLink == "" {
		request.MeetingLink = inferMeetingLink(rawText)
	}

	if !isParsedRequestValid(request) {
		return nil, false
	}
	return request, true
}

func isParsedRequestValid(request *CreateInterviewRequest) bool {
	if request == nil {
		return false
	}
	if strings.TrimSpace(request.CompanyName) == "" {
		return false
	}
	if strings.TrimSpace(request.Position) == "" {
		return false
	}
	if strings.TrimSpace(request.InterviewTime) == "" {
		return false
	}
	if _, err := parseScheduleTime(request.InterviewTime); err != nil {
		return false
	}
	return true
}

func inferInterviewType(rawText string) string {
	lower := strings.ToLower(rawText)
	switch {
	case strings.Contains(lower, "onsite") || strings.Contains(rawText, "现场") || strings.Contains(rawText, "线下"):
		return string(InterviewTypeOnsite)
	case strings.Contains(lower, "phone") || strings.Contains(rawText, "电话"):
		return string(InterviewTypePhone)
	default:
		return string(InterviewTypeVideo)
	}
}

func inferMeetingLink(rawText string) string {
	for _, pattern := range []string{
		`https?://meeting\.feishu\.cn/[^\s"']+`,
		`https?://zoom\.us/j/[^\s"']+`,
		`https?://meeting\.tencent\.com/[^\s"']+`,
		`https?://[^\s"']+`,
	} {
		if value := firstMatch(rawText, []string{pattern}); value != "" {
			return value
		}
	}
	return ""
}

func firstMatch(rawText string, patterns []string) string {
	for _, expr := range patterns {
		re := regexp.MustCompile(expr)
		match := re.FindStringSubmatch(rawText)
		if len(match) > 1 {
			return strings.TrimSpace(match[1])
		}
		if len(match) == 1 && match[0] != "" {
			return strings.TrimSpace(match[0])
		}
	}
	return ""
}

func firstTime(rawText string) string {
	patterns := []string{
		`(?im)(?:时间|面试时间|interview time|time)\s*[:：]\s*([^\n,，;；]{1,80})`,
		`(\d{4}[-/]\d{2}[-/]\d{2}[T\s]\d{2}:\d{2}(?::\d{2})?)`,
		`(\d{4}年\d{1,2}月\d{1,2}日\s*\d{1,2}:\d{2}(?::\d{2})?)`,
	}
	return strings.TrimSpace(firstMatch(rawText, patterns))
}

func parseRoundNumber(raw string) (int, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	if value, err := strconv.Atoi(trimmed); err == nil {
		return value, true
	}
	if len([]rune(trimmed)) == 1 {
		switch []rune(trimmed)[0] {
		case '一':
			return 1, true
		case '二':
			return 2, true
		case '三':
			return 3, true
		case '四':
			return 4, true
		case '五':
			return 5, true
		case '六':
			return 6, true
		case '七':
			return 7, true
		case '八':
			return 8, true
		case '九':
			return 9, true
		case '十':
			return 10, true
		}
	}
	return 0, false
}
