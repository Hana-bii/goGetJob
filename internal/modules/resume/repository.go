package resume

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("resume not found")

type Repository interface {
	CreateResume(ctx context.Context, resume *Resume) error
	UpdateResume(ctx context.Context, resume *Resume) error
	FindResumeByID(ctx context.Context, id uint) (*Resume, error)
	FindResumeByHash(ctx context.Context, hash string) (*Resume, error)
	ListResumes(ctx context.Context) ([]Resume, error)
	DeleteResume(ctx context.Context, id uint) error
	CreateAnalysis(ctx context.Context, analysis *ResumeAnalysis) error
	LatestAnalysis(ctx context.Context, resumeID uint) (*ResumeAnalysis, error)
	ListAnalyses(ctx context.Context, resumeID uint) ([]ResumeAnalysis, error)
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (r *GormRepository) AutoMigrate() error {
	if r == nil || r.db == nil {
		return fmt.Errorf("gorm db is required")
	}
	return r.db.AutoMigrate(&Resume{}, &ResumeAnalysis{})
}

func (r *GormRepository) CreateResume(ctx context.Context, resume *Resume) error {
	return r.require().WithContext(ctx).Create(resume).Error
}

func (r *GormRepository) UpdateResume(ctx context.Context, resume *Resume) error {
	return r.require().WithContext(ctx).Save(resume).Error
}

func (r *GormRepository) FindResumeByID(ctx context.Context, id uint) (*Resume, error) {
	var resume Resume
	err := r.require().WithContext(ctx).First(&resume, id).Error
	return resumeResult(&resume, err)
}

func (r *GormRepository) FindResumeByHash(ctx context.Context, hash string) (*Resume, error) {
	var resume Resume
	err := r.require().WithContext(ctx).Where("file_hash = ?", hash).First(&resume).Error
	return resumeResult(&resume, err)
}

func (r *GormRepository) ListResumes(ctx context.Context) ([]Resume, error) {
	var resumes []Resume
	err := r.require().WithContext(ctx).Order("uploaded_at desc").Find(&resumes).Error
	return resumes, err
}

func (r *GormRepository) DeleteResume(ctx context.Context, id uint) error {
	db := r.require().WithContext(ctx)
	if err := db.Where("resume_id = ?", id).Delete(&ResumeAnalysis{}).Error; err != nil {
		return err
	}
	result := db.Delete(&Resume{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) CreateAnalysis(ctx context.Context, analysis *ResumeAnalysis) error {
	return r.require().WithContext(ctx).Create(analysis).Error
}

func (r *GormRepository) LatestAnalysis(ctx context.Context, resumeID uint) (*ResumeAnalysis, error) {
	var analysis ResumeAnalysis
	err := r.require().WithContext(ctx).Where("resume_id = ?", resumeID).Order("analyzed_at desc").First(&analysis).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &analysis, err
}

func (r *GormRepository) ListAnalyses(ctx context.Context, resumeID uint) ([]ResumeAnalysis, error) {
	var analyses []ResumeAnalysis
	err := r.require().WithContext(ctx).Where("resume_id = ?", resumeID).Order("analyzed_at desc").Find(&analyses).Error
	return analyses, err
}

func (r *GormRepository) require() *gorm.DB {
	if r == nil || r.db == nil {
		panic("gorm db is required")
	}
	return r.db
}

func resumeResult(resume *Resume, err error) (*Resume, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return resume, err
}

type MemoryRepository struct {
	mu       sync.Mutex
	nextID   uint
	nextAID  uint
	resumes  map[uint]Resume
	byHash   map[string]uint
	analyses map[uint][]ResumeAnalysis
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		nextID:   1,
		nextAID:  1,
		resumes:  map[uint]Resume{},
		byHash:   map[string]uint{},
		analyses: map[uint][]ResumeAnalysis{},
	}
}

func (r *MemoryRepository) CreateResume(_ context.Context, resume *Resume) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if resume.ID == 0 {
		resume.ID = r.nextID
		r.nextID++
	}
	if err := resume.BeforeCreate(); err != nil {
		return err
	}
	r.resumes[resume.ID] = *resume
	r.byHash[resume.FileHash] = resume.ID
	return nil
}

func (r *MemoryRepository) UpdateResume(_ context.Context, resume *Resume) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.resumes[resume.ID]; !ok {
		return ErrNotFound
	}
	r.resumes[resume.ID] = *resume
	r.byHash[resume.FileHash] = resume.ID
	return nil
}

func (r *MemoryRepository) FindResumeByID(_ context.Context, id uint) (*Resume, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resume, ok := r.resumes[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &resume, nil
}

func (r *MemoryRepository) FindResumeByHash(_ context.Context, hash string) (*Resume, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byHash[hash]
	if !ok {
		return nil, ErrNotFound
	}
	resume := r.resumes[id]
	return &resume, nil
}

func (r *MemoryRepository) ListResumes(_ context.Context) ([]Resume, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resumes := make([]Resume, 0, len(r.resumes))
	for _, resume := range r.resumes {
		resumes = append(resumes, resume)
	}
	return resumes, nil
}

func (r *MemoryRepository) DeleteResume(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	resume, ok := r.resumes[id]
	if !ok {
		return ErrNotFound
	}
	delete(r.byHash, resume.FileHash)
	delete(r.resumes, id)
	delete(r.analyses, id)
	return nil
}

func (r *MemoryRepository) CreateAnalysis(_ context.Context, analysis *ResumeAnalysis) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.resumes[analysis.ResumeID]; !ok {
		return ErrNotFound
	}
	if analysis.ID == 0 {
		analysis.ID = r.nextAID
		r.nextAID++
	}
	if err := analysis.BeforeCreate(); err != nil {
		return err
	}
	r.analyses[analysis.ResumeID] = append(r.analyses[analysis.ResumeID], *analysis)
	return nil
}

func (r *MemoryRepository) LatestAnalysis(_ context.Context, resumeID uint) (*ResumeAnalysis, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.analyses[resumeID]
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	latest := list[len(list)-1]
	return &latest, nil
}

func (r *MemoryRepository) ListAnalyses(_ context.Context, resumeID uint) ([]ResumeAnalysis, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := append([]ResumeAnalysis(nil), r.analyses[resumeID]...)
	return list, nil
}
