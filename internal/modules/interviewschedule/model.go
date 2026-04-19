package interviewschedule

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultInterviewType = "VIDEO"
	defaultRoundNumber   = 1
	datetimeLayout       = "2006-01-02T15:04:05"
)

var supportedInterviewTypes = map[string]struct{}{
	string(InterviewTypeOnsite): {},
	string(InterviewTypeVideo):  {},
	string(InterviewTypePhone):  {},
}

type InterviewStatus string

const (
	InterviewStatusPending     InterviewStatus = "PENDING"
	InterviewStatusCompleted   InterviewStatus = "COMPLETED"
	InterviewStatusCancelled   InterviewStatus = "CANCELLED"
	InterviewStatusRescheduled InterviewStatus = "RESCHEDULED"
)

type InterviewType string

const (
	InterviewTypeOnsite InterviewType = "ONSITE"
	InterviewTypeVideo  InterviewType = "VIDEO"
	InterviewTypePhone  InterviewType = "PHONE"
)

type InterviewSchedule struct {
	ID            uint            `gorm:"primaryKey"`
	CompanyName   string          `gorm:"size:200;not null;index"`
	Position      string          `gorm:"size:200;not null"`
	InterviewTime time.Time       `gorm:"not null;index"`
	InterviewType string          `gorm:"size:20;not null"`
	MeetingLink   string          `gorm:"type:text"`
	RoundNumber   int             `gorm:"not null;default:1"`
	Interviewer   string          `gorm:"size:200"`
	Notes         string          `gorm:"type:text"`
	Status        InterviewStatus `gorm:"size:20;not null;index"`
	CreatedAt     time.Time       `gorm:"not null"`
	UpdatedAt     time.Time       `gorm:"not null"`
}

func (s *InterviewSchedule) BeforeCreate() error {
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = s.CreatedAt
	}
	if s.Status == "" {
		s.Status = InterviewStatusPending
	}
	normalized, err := normalizeInterviewType(s.InterviewType)
	if err != nil {
		return err
	}
	s.InterviewType = normalized
	if s.RoundNumber <= 0 {
		s.RoundNumber = defaultRoundNumber
	}
	s.CompanyName = strings.TrimSpace(s.CompanyName)
	s.Position = strings.TrimSpace(s.Position)
	s.MeetingLink = strings.TrimSpace(s.MeetingLink)
	s.Interviewer = strings.TrimSpace(s.Interviewer)
	s.Notes = strings.TrimSpace(s.Notes)
	return nil
}

func (s *InterviewSchedule) BeforeUpdate() error {
	s.UpdatedAt = time.Now()
	normalized, err := normalizeInterviewType(s.InterviewType)
	if err != nil {
		return err
	}
	s.InterviewType = normalized
	if s.RoundNumber <= 0 {
		s.RoundNumber = defaultRoundNumber
	}
	s.CompanyName = strings.TrimSpace(s.CompanyName)
	s.Position = strings.TrimSpace(s.Position)
	s.MeetingLink = strings.TrimSpace(s.MeetingLink)
	s.Interviewer = strings.TrimSpace(s.Interviewer)
	s.Notes = strings.TrimSpace(s.Notes)
	return nil
}

type InterviewScheduleDTO struct {
	ID            uint            `json:"id"`
	CompanyName   string          `json:"companyName"`
	Position      string          `json:"position"`
	InterviewTime string          `json:"interviewTime"`
	InterviewType string          `json:"interviewType"`
	MeetingLink   string          `json:"meetingLink,omitempty"`
	RoundNumber   int             `json:"roundNumber"`
	Interviewer   string          `json:"interviewer,omitempty"`
	Notes         string          `json:"notes,omitempty"`
	Status        InterviewStatus `json:"status"`
	CreatedAt     string          `json:"createdAt"`
	UpdatedAt     string          `json:"updatedAt"`
}

type CreateInterviewRequest struct {
	CompanyName   string `json:"companyName"`
	Position      string `json:"position"`
	InterviewTime string `json:"interviewTime"`
	InterviewType string `json:"interviewType,omitempty"`
	MeetingLink   string `json:"meetingLink,omitempty"`
	RoundNumber   int    `json:"roundNumber,omitempty"`
	Interviewer   string `json:"interviewer,omitempty"`
	Notes         string `json:"notes,omitempty"`
}

type ParseRequest struct {
	RawText string `json:"rawText"`
	Source  string `json:"source,omitempty"`
}

type ParseResponse struct {
	Success     bool                    `json:"success"`
	Data        *CreateInterviewRequest `json:"data"`
	Confidence  float64                 `json:"confidence"`
	ParseMethod string                  `json:"parseMethod"`
	Log         string                  `json:"log"`
}

func (s InterviewSchedule) DTO() InterviewScheduleDTO {
	return InterviewScheduleDTO{
		ID:            s.ID,
		CompanyName:   s.CompanyName,
		Position:      s.Position,
		InterviewTime: formatScheduleTime(s.InterviewTime),
		InterviewType: s.InterviewType,
		MeetingLink:   s.MeetingLink,
		RoundNumber:   s.RoundNumber,
		Interviewer:   s.Interviewer,
		Notes:         s.Notes,
		Status:        s.Status,
		CreatedAt:     formatScheduleTime(s.CreatedAt),
		UpdatedAt:     formatScheduleTime(s.UpdatedAt),
	}
}

func (r CreateInterviewRequest) normalize() CreateInterviewRequest {
	r.CompanyName = strings.TrimSpace(r.CompanyName)
	r.Position = strings.TrimSpace(r.Position)
	r.InterviewType = strings.TrimSpace(r.InterviewType)
	r.MeetingLink = strings.TrimSpace(r.MeetingLink)
	r.Interviewer = strings.TrimSpace(r.Interviewer)
	r.Notes = strings.TrimSpace(r.Notes)
	if r.RoundNumber <= 0 {
		r.RoundNumber = defaultRoundNumber
	}
	return r
}

func normalizeInterviewType(raw string) (string, error) {
	trimmed := strings.ToUpper(strings.TrimSpace(raw))
	if trimmed == "" {
		return string(InterviewTypeVideo), nil
	}
	if _, ok := supportedInterviewTypes[trimmed]; ok {
		return trimmed, nil
	}
	return "", fmt.Errorf("%w: unsupported interview type %q", ErrInvalidInput, raw)
}

func normalizeInterviewStatus(raw string) (InterviewStatus, error) {
	trimmed := InterviewStatus(strings.ToUpper(strings.TrimSpace(raw)))
	switch trimmed {
	case "", InterviewStatusPending, InterviewStatusCompleted, InterviewStatusCancelled, InterviewStatusRescheduled:
		return trimmed, nil
	default:
		return "", fmt.Errorf("%w: invalid interview status %q", ErrInvalidInput, raw)
	}
}

func formatScheduleTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format(datetimeLayout)
}

func parseScheduleTime(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("%w: interview time is required", ErrInvalidInput)
	}
	layouts := []string{
		datetimeLayout,
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		time.RFC3339,
		time.RFC3339Nano,
		"2006年1月2日 15:04:05",
		"2006年1月2日 15:04",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: invalid interview time %q", ErrInvalidInput, raw)
}
