package model

type AsyncTaskStatus string

const (
	AsyncTaskStatusPending    AsyncTaskStatus = "PENDING"
	AsyncTaskStatusProcessing AsyncTaskStatus = "PROCESSING"
	AsyncTaskStatusCompleted  AsyncTaskStatus = "COMPLETED"
	AsyncTaskStatusFailed     AsyncTaskStatus = "FAILED"
)
