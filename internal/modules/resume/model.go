package resume

import (
	"time"

	commonmodel "goGetJob/internal/common/model"
)

const maxAnalyzeErrorLength = 500

type Resume struct {
	ID               uint                        `json:"id" gorm:"primaryKey"`
	FileHash         string                      `json:"fileHash" gorm:"size:64;uniqueIndex;not null"`
	OriginalFilename string                      `json:"originalFilename" gorm:"not null"`
	FileSize         int64                       `json:"fileSize"`
	ContentType      string                      `json:"contentType"`
	StorageKey       string                      `json:"storageKey" gorm:"size:500"`
	StorageURL       string                      `json:"storageUrl" gorm:"size:1000"`
	ResumeText       string                      `json:"resumeText" gorm:"type:text"`
	UploadedAt       time.Time                   `json:"uploadedAt" gorm:"not null"`
	LastAccessedAt   time.Time                   `json:"lastAccessedAt"`
	AccessCount      int                         `json:"accessCount"`
	AnalyzeStatus    commonmodel.AsyncTaskStatus `json:"analyzeStatus" gorm:"size:20"`
	AnalyzeError     string                      `json:"analyzeError" gorm:"size:500"`
	Analyses         []ResumeAnalysis            `json:"-" gorm:"foreignKey:ResumeID"`
}

func (r *Resume) BeforeCreate() error {
	now := time.Now()
	if r.UploadedAt.IsZero() {
		r.UploadedAt = now
	}
	if r.LastAccessedAt.IsZero() {
		r.LastAccessedAt = r.UploadedAt
	}
	if r.AccessCount == 0 {
		r.AccessCount = 1
	}
	if r.AnalyzeStatus == "" {
		r.AnalyzeStatus = commonmodel.AsyncTaskStatusPending
	}
	return nil
}

func (r *Resume) Touch() {
	r.AccessCount++
	r.LastAccessedAt = time.Now()
}

type ResumeAnalysis struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	ResumeID        uint      `json:"resumeId" gorm:"index;not null"`
	OverallScore    int       `json:"overallScore"`
	ContentScore    int       `json:"contentScore"`
	StructureScore  int       `json:"structureScore"`
	SkillMatchScore int       `json:"skillMatchScore"`
	ExpressionScore int       `json:"expressionScore"`
	ProjectScore    int       `json:"projectScore"`
	Summary         string    `json:"summary" gorm:"type:text"`
	StrengthsJSON   string    `json:"strengthsJson" gorm:"type:text"`
	SuggestionsJSON string    `json:"suggestionsJson" gorm:"type:text"`
	AnalyzedAt      time.Time `json:"analyzedAt" gorm:"not null"`
}

func (a *ResumeAnalysis) BeforeCreate() error {
	if a.AnalyzedAt.IsZero() {
		a.AnalyzedAt = time.Now()
	}
	return nil
}

type ScoreDetail struct {
	ContentScore    int `json:"contentScore"`
	StructureScore  int `json:"structureScore"`
	SkillMatchScore int `json:"skillMatchScore"`
	ExpressionScore int `json:"expressionScore"`
	ProjectScore    int `json:"projectScore"`
}

type Suggestion struct {
	Category       string `json:"category"`
	Priority       string `json:"priority"`
	Issue          string `json:"issue"`
	Recommendation string `json:"recommendation"`
}

type AnalysisResult struct {
	OverallScore int          `json:"overallScore"`
	ScoreDetail  ScoreDetail  `json:"scoreDetail"`
	Summary      string       `json:"summary"`
	Strengths    []string     `json:"strengths"`
	Suggestions  []Suggestion `json:"suggestions"`
	OriginalText string       `json:"originalText"`
}

type UploadInput struct {
	Filename    string
	ContentType string
	Data        []byte
}

type UploadResult struct {
	Resume    Resume          `json:"resume"`
	Storage   StorageSummary  `json:"storage"`
	Analysis  *AnalysisResult `json:"analysis,omitempty"`
	Duplicate bool            `json:"duplicate"`
}

type StorageSummary struct {
	FileKey  string `json:"fileKey"`
	FileURL  string `json:"fileUrl"`
	ResumeID uint   `json:"resumeId"`
}

type ResumeListItem struct {
	ID               uint                        `json:"id"`
	OriginalFilename string                      `json:"originalFilename"`
	FileSize         int64                       `json:"fileSize"`
	UploadedAt       time.Time                   `json:"uploadedAt"`
	AccessCount      int                         `json:"accessCount"`
	LatestScore      *int                        `json:"latestScore,omitempty"`
	LastAnalyzedAt   *time.Time                  `json:"lastAnalyzedAt,omitempty"`
	AnalyzeStatus    commonmodel.AsyncTaskStatus `json:"analyzeStatus"`
	AnalyzeError     string                      `json:"analyzeError,omitempty"`
}

type ResumeDetail struct {
	Resume
	AnalysisHistory []AnalysisResult `json:"analysisHistory"`
}
