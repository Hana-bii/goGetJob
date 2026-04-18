package knowledgebase

import (
	"time"

	commonmodel "goGetJob/internal/common/model"
)

const (
	maxVectorErrorLength = 500
	defaultNoResult      = "没有找到相关信息。"
)

type KnowledgeBase struct {
	ID               uint                        `json:"id" gorm:"primaryKey"`
	Name             string                      `json:"name" gorm:"not null"`
	Category         string                      `json:"category" gorm:"size:100;index"`
	FileHash         string                      `json:"fileHash" gorm:"size:64;uniqueIndex;not null"`
	OriginalFilename string                      `json:"originalFilename"`
	FileSize         int64                       `json:"fileSize"`
	ContentType      string                      `json:"contentType"`
	StorageKey       string                      `json:"storageKey" gorm:"size:500"`
	StorageURL       string                      `json:"storageUrl" gorm:"size:1000"`
	Content          string                      `json:"content" gorm:"type:text"`
	UploadedAt       time.Time                   `json:"uploadedAt"`
	LastAccessedAt   time.Time                   `json:"lastAccessedAt"`
	AccessCount      int                         `json:"accessCount"`
	QuestionCount    int                         `json:"questionCount"`
	VectorStatus     commonmodel.AsyncTaskStatus `json:"vectorStatus" gorm:"size:20;index"`
	VectorError      string                      `json:"vectorError" gorm:"size:500"`
	ChunkCount       int                         `json:"chunkCount"`
}

func (k *KnowledgeBase) BeforeCreate() error {
	now := time.Now()
	if k.UploadedAt.IsZero() {
		k.UploadedAt = now
	}
	if k.LastAccessedAt.IsZero() {
		k.LastAccessedAt = k.UploadedAt
	}
	if k.VectorStatus == "" {
		k.VectorStatus = commonmodel.AsyncTaskStatusPending
	}
	return nil
}

func (k *KnowledgeBase) Touch() {
	k.AccessCount++
	k.LastAccessedAt = time.Now()
}

type KnowledgeBaseListItem struct {
	ID               uint                        `json:"id"`
	Name             string                      `json:"name"`
	Category         string                      `json:"category,omitempty"`
	OriginalFilename string                      `json:"originalFilename"`
	FileSize         int64                       `json:"fileSize"`
	ContentType      string                      `json:"contentType"`
	ContentLength    int                         `json:"contentLength"`
	StorageKey       string                      `json:"storageKey,omitempty"`
	StorageURL       string                      `json:"storageUrl,omitempty"`
	UploadedAt       time.Time                   `json:"uploadedAt"`
	LastAccessedAt   time.Time                   `json:"lastAccessedAt"`
	AccessCount      int                         `json:"accessCount"`
	QuestionCount    int                         `json:"questionCount"`
	VectorStatus     commonmodel.AsyncTaskStatus `json:"vectorStatus"`
	VectorError      string                      `json:"vectorError,omitempty"`
	ChunkCount       int                         `json:"chunkCount"`
}

type KnowledgeBaseStats struct {
	TotalCount         int `json:"totalCount"`
	TotalQuestionCount int `json:"totalQuestionCount"`
	TotalAccessCount   int `json:"totalAccessCount"`
	CompletedCount     int `json:"completedCount"`
	ProcessingCount    int `json:"processingCount"`
}

type UploadResult struct {
	KnowledgeBase KnowledgeBaseUploadSummary `json:"knowledgeBase"`
	Storage       StorageSummary             `json:"storage"`
	Duplicate     bool                       `json:"duplicate"`
}

type KnowledgeBaseUploadSummary struct {
	ID            uint                        `json:"id"`
	Name          string                      `json:"name"`
	Category      string                      `json:"category,omitempty"`
	FileSize      int64                       `json:"fileSize"`
	ContentLength int                         `json:"contentLength"`
	VectorStatus  commonmodel.AsyncTaskStatus `json:"vectorStatus"`
}

type StorageSummary struct {
	FileKey string `json:"fileKey"`
	FileURL string `json:"fileUrl"`
}

type UploadInput struct {
	Filename    string
	ContentType string
	Data        []byte
	Name        string
	Category    string
}

type QueryRequest struct {
	KnowledgeBaseIDs []uint `json:"knowledgeBaseIds"`
	Question         string `json:"question"`
}

type QueryResponse struct {
	Answer            string `json:"answer"`
	KnowledgeBaseID   uint   `json:"knowledgeBaseId"`
	KnowledgeBaseName string `json:"knowledgeBaseName"`
}

type RagChatSession struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	SessionID      string    `json:"sessionId" gorm:"size:36;uniqueIndex;not null"`
	Title          string    `json:"title"`
	KnowledgeBases string    `json:"knowledgeBaseIds" gorm:"type:text"`
	MessageCount   int       `json:"messageCount"`
	IsPinned       bool      `json:"isPinned"`
	Status         string    `json:"status" gorm:"size:20"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type RagChatMessage struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	SessionID    string    `json:"sessionId" gorm:"size:36;index"`
	Type         string    `json:"type" gorm:"size:20"`
	Content      string    `json:"content" gorm:"type:text"`
	MessageOrder int       `json:"messageOrder"`
	Completed    bool      `json:"completed"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type CreateRagChatSessionRequest struct {
	KnowledgeBaseIDs []uint `json:"knowledgeBaseIds"`
	Title            string `json:"title"`
}

type UpdateRagChatTitleRequest struct {
	Title string `json:"title"`
}

type UpdateRagChatKnowledgeBasesRequest struct {
	KnowledgeBaseIDs []uint `json:"knowledgeBaseIds"`
}

type RagChatMessageRequest struct {
	Question string `json:"question"`
}

type RagChatSessionDetail struct {
	Session  RagChatSession   `json:"session"`
	Messages []RagChatMessage `json:"messages"`
}
