package interviewschedule

import (
	"context"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidInput = fmt.Errorf("invalid interview schedule input")

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, request CreateInterviewRequest) (InterviewScheduleDTO, error) {
	if err := s.require(); err != nil {
		return InterviewScheduleDTO{}, err
	}
	entity, err := buildEntity(request)
	if err != nil {
		return InterviewScheduleDTO{}, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return InterviewScheduleDTO{}, err
	}
	return entity.DTO(), nil
}

func (s *Service) GetByID(ctx context.Context, id uint) (InterviewScheduleDTO, error) {
	if err := s.require(); err != nil {
		return InterviewScheduleDTO{}, err
	}
	entity, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return InterviewScheduleDTO{}, err
	}
	return entity.DTO(), nil
}

func (s *Service) List(ctx context.Context, status string, start, end *time.Time) ([]InterviewScheduleDTO, error) {
	if err := s.require(); err != nil {
		return nil, err
	}
	if start != nil && end != nil {
		items, err := s.repo.ListByInterviewTimeBetween(ctx, *start, *end)
		if err != nil {
			return nil, err
		}
		return toDTOs(items), nil
	}
	if start != nil || end != nil {
		return nil, fmt.Errorf("%w: start and end must be provided together", ErrInvalidInput)
	}
	if strings.TrimSpace(status) != "" {
		normalized, err := normalizeInterviewStatus(status)
		if err != nil {
			return nil, err
		}
		items, err := s.repo.ListByStatus(ctx, normalized)
		if err != nil {
			return nil, err
		}
		return toDTOs(items), nil
	}
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	return toDTOs(items), nil
}

func (s *Service) Update(ctx context.Context, id uint, request CreateInterviewRequest) (InterviewScheduleDTO, error) {
	if err := s.require(); err != nil {
		return InterviewScheduleDTO{}, err
	}
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return InterviewScheduleDTO{}, err
	}
	updated, err := buildEntity(request)
	if err != nil {
		return InterviewScheduleDTO{}, err
	}
	existing.CompanyName = updated.CompanyName
	existing.Position = updated.Position
	existing.InterviewTime = updated.InterviewTime
	existing.InterviewType = updated.InterviewType
	existing.MeetingLink = updated.MeetingLink
	existing.RoundNumber = updated.RoundNumber
	existing.Interviewer = updated.Interviewer
	existing.Notes = updated.Notes
	if err := s.repo.Update(ctx, existing); err != nil {
		return InterviewScheduleDTO{}, err
	}
	return existing.DTO(), nil
}

func (s *Service) Delete(ctx context.Context, id uint) error {
	if err := s.require(); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) UpdateStatus(ctx context.Context, id uint, status InterviewStatus) (InterviewScheduleDTO, error) {
	if err := s.require(); err != nil {
		return InterviewScheduleDTO{}, err
	}
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return InterviewScheduleDTO{}, err
	}
	existing.Status = status
	if err := s.repo.Update(ctx, existing); err != nil {
		return InterviewScheduleDTO{}, err
	}
	return existing.DTO(), nil
}

func (s *Service) require() error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("schedule repository is required")
	}
	return nil
}

func buildEntity(request CreateInterviewRequest) (*InterviewSchedule, error) {
	normalized := request.normalize()
	if normalized.CompanyName == "" {
		return nil, fmt.Errorf("%w: company name is required", ErrInvalidInput)
	}
	if normalized.Position == "" {
		return nil, fmt.Errorf("%w: position is required", ErrInvalidInput)
	}
	interviewTime, err := parseScheduleTime(normalized.InterviewTime)
	if err != nil {
		return nil, err
	}
	interviewType, err := normalizeInterviewType(normalized.InterviewType)
	if err != nil {
		return nil, err
	}
	return &InterviewSchedule{
		CompanyName:   normalized.CompanyName,
		Position:      normalized.Position,
		InterviewTime: interviewTime,
		InterviewType: interviewType,
		MeetingLink:   normalized.MeetingLink,
		RoundNumber:   normalized.RoundNumber,
		Interviewer:   normalized.Interviewer,
		Notes:         normalized.Notes,
		Status:        InterviewStatusPending,
	}, nil
}

func toDTOs(items []InterviewSchedule) []InterviewScheduleDTO {
	out := make([]InterviewScheduleDTO, 0, len(items))
	for _, item := range items {
		out = append(out, item.DTO())
	}
	return out
}
