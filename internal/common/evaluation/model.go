package evaluation

type QaRecord struct {
	QuestionIndex int    `json:"questionIndex"`
	Question      string `json:"question"`
	Category      string `json:"category"`
	UserAnswer    string `json:"userAnswer"`
}

type Report struct {
	SessionID        string               `json:"sessionId"`
	TotalQuestions   int                  `json:"totalQuestions"`
	OverallScore     int                  `json:"overallScore"`
	CategoryScores   []CategoryScore      `json:"categoryScores"`
	QuestionDetails  []QuestionEvaluation `json:"questionDetails"`
	OverallFeedback  string               `json:"overallFeedback"`
	Strengths        []string             `json:"strengths"`
	Improvements     []string             `json:"improvements"`
	ReferenceAnswers []ReferenceAnswer    `json:"referenceAnswers"`
}

type CategoryScore struct {
	Category      string `json:"category"`
	Score         int    `json:"score"`
	QuestionCount int    `json:"questionCount"`
}

type QuestionEvaluation struct {
	QuestionIndex int    `json:"questionIndex"`
	Question      string `json:"question"`
	Category      string `json:"category"`
	UserAnswer    string `json:"userAnswer"`
	Score         int    `json:"score"`
	Feedback      string `json:"feedback"`
}

type ReferenceAnswer struct {
	QuestionIndex   int      `json:"questionIndex"`
	Question        string   `json:"question"`
	ReferenceAnswer string   `json:"referenceAnswer"`
	KeyPoints       []string `json:"keyPoints"`
}
