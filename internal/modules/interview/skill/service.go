package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	maxReferenceSectionChars = 12000
	maxSingleReferenceChars  = 3000
)

var frontMatterPattern = regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n?(.*)$`)

type Options struct {
	Root string
}

type Service struct {
	root             string
	skills           map[string]Skill
	categoryRefIndex map[string]refMapping
	referenceCache   map[string]string
}

type refMapping struct {
	ref     string
	shared  bool
	skillID string
}

func NewService(options Options) (*Service, error) {
	root := options.Root
	if root == "" {
		root = "internal/skills"
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	s := &Service{
		root:             root,
		skills:           map[string]Skill{},
		categoryRefIndex: map[string]refMapping{},
		referenceCache:   map[string]string{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) load() error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return fmt.Errorf("read skills root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		skillID := entry.Name()
		loaded, err := s.loadSkill(skillID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(loaded.Name) == "" {
			continue
		}
		s.skills[skillID] = loaded
		for _, category := range loaded.Categories {
			if category.Key != "" && category.Ref != "" {
				if _, exists := s.categoryRefIndex[category.Key]; !exists {
					s.categoryRefIndex[category.Key] = refMapping{ref: category.Ref, shared: category.Shared, skillID: skillID}
				}
			}
		}
	}
	return nil
}

func (s *Service) loadSkill(skillID string) (Skill, error) {
	raw, err := os.ReadFile(filepath.Join(s.root, skillID, "SKILL.md"))
	if err != nil {
		return Skill{}, fmt.Errorf("read skill %s: %w", skillID, err)
	}
	match := frontMatterPattern.FindStringSubmatch(string(raw))
	if len(match) != 3 {
		return Skill{}, fmt.Errorf("skill %s missing front matter", skillID)
	}
	var fm frontMatter
	if err := yaml.Unmarshal([]byte(match[1]), &fm); err != nil {
		return Skill{}, fmt.Errorf("parse skill front matter %s: %w", skillID, err)
	}
	var meta metaFile
	if rawMeta, err := os.ReadFile(filepath.Join(s.root, skillID, "skill.meta.yml")); err == nil {
		if err := yaml.Unmarshal(rawMeta, &meta); err != nil {
			return Skill{}, fmt.Errorf("parse skill meta %s: %w", skillID, err)
		}
	}
	name := meta.DisplayName
	if name == "" {
		name = fm.Name
	}
	return Skill{
		ID:          skillID,
		Name:        name,
		Description: fm.Description,
		Categories:  normalizeCategories(meta.Categories),
		IsPreset:    true,
		Persona:     strings.TrimSpace(match[2]),
		Display:     meta.Display,
	}, nil
}

func normalizeCategories(categories []Category) []Category {
	out := make([]Category, 0, len(categories))
	for _, category := range categories {
		if category.Priority == "" {
			category.Priority = PriorityNormal
		}
		out = append(out, category)
	}
	return out
}

func (s *Service) All() []Skill {
	ids := make([]string, 0, len(s.skills))
	for id := range s.skills {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Skill, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.skills[id])
	}
	return out
}

func (s *Service) Get(id string) (Skill, error) {
	skill, ok := s.skills[id]
	if !ok {
		return Skill{}, fmt.Errorf("skill %q not found", id)
	}
	return skill, nil
}

func (s *Service) BuildCustom(categories []Category, jdText string) Skill {
	out := make([]Category, 0, len(categories))
	for _, category := range categories {
		if mapped, ok := s.categoryRefIndex[category.Key]; ok {
			category.Ref = mapped.ref
			category.Shared = mapped.shared
		}
		if category.Priority == "" {
			category.Priority = PriorityNormal
		}
		out = append(out, category)
	}
	return Skill{ID: CustomSkillID, Name: "自定义面试（JD 解析）", Description: "基于职位描述提取的面试方向", Categories: out, SourceJD: jdText}
}

func (s *Service) CalculateAllocation(categories []Category, totalQuestions int) map[string]int {
	allocation := map[string]int{}
	var always, core, normal []Category
	for _, category := range categories {
		switch category.Priority {
		case PriorityAlwaysOne:
			always = append(always, category)
		case PriorityCore:
			core = append(core, category)
		default:
			normal = append(normal, category)
		}
	}
	remaining := totalQuestions
	for _, group := range [][]Category{always, core, normal} {
		for _, category := range group {
			if remaining <= 0 {
				break
			}
			allocation[category.Key]++
			remaining--
		}
	}
	for remaining > 0 && (len(core)+len(normal)) > 0 {
		for _, category := range append(append([]Category{}, core...), normal...) {
			if remaining <= 0 {
				break
			}
			allocation[category.Key]++
			remaining--
		}
	}
	for _, category := range categories {
		if _, ok := allocation[category.Key]; !ok {
			allocation[category.Key] = 0
		}
	}
	return allocation
}

func (s *Service) BuildReferenceSectionSafe(skillID string, allocation map[string]int) string {
	skill, err := s.Get(skillID)
	if err != nil && skillID != CustomSkillID {
		return ""
	}
	if skillID == CustomSkillID {
		skill = Skill{ID: CustomSkillID}
		for key, mapped := range s.categoryRefIndex {
			skill.Categories = append(skill.Categories, Category{Key: key, Label: key, Ref: mapped.ref, Shared: mapped.shared})
		}
	}
	return s.buildReferenceSection(skill, allocation, maxReferenceSectionChars)
}

func (s *Service) buildReferenceSection(skill Skill, allocation map[string]int, maxChars int) string {
	var builder strings.Builder
	for _, category := range skill.Categories {
		if allocation != nil && allocation[category.Key] <= 0 {
			continue
		}
		if category.Ref == "" || !safeReference(category.Ref) {
			continue
		}
		content := s.loadReference(skill.ID, category)
		if strings.TrimSpace(content) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("### ")
		builder.WriteString(category.Label)
		builder.WriteByte('\n')
		builder.WriteString(content)
		if builder.Len() > maxChars {
			out := truncateUTF8(builder.String(), maxChars)
			return out + "\n...(references truncated)"
		}
	}
	if builder.Len() == 0 {
		return "未配置 references。"
	}
	return builder.String()
}

func (s *Service) loadReference(skillID string, category Category) string {
	candidates := []string{}
	effectiveSkillID := skillID
	if skillID == CustomSkillID {
		if mapped, ok := s.categoryRefIndex[category.Key]; ok {
			effectiveSkillID = mapped.skillID
		}
	}
	if category.Shared {
		candidates = append(candidates, filepath.Join(s.root, "_shared", "references", category.Ref))
	}
	if effectiveSkillID != "" && effectiveSkillID != CustomSkillID {
		candidates = append(candidates,
			filepath.Join(s.root, effectiveSkillID, "references", category.Ref),
			filepath.Join(s.root, effectiveSkillID, category.Ref),
		)
	}
	if !category.Shared {
		candidates = append(candidates, filepath.Join(s.root, "_shared", "references", category.Ref))
	}
	for _, path := range candidates {
		if cached, ok := s.referenceCache[path]; ok {
			return cached
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(raw))
		if len(content) > maxSingleReferenceChars {
			content = truncateUTF8(content, maxSingleReferenceChars) + "\n...(single reference truncated)"
		}
		s.referenceCache[path] = content
		return content
	}
	return ""
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		_, size := utf8.DecodeLastRuneInString(value[:end])
		if size <= 1 {
			end--
		} else {
			end -= size
		}
	}
	return strings.TrimSpace(value[:end])
}

func safeReference(ref string) bool {
	return !strings.Contains(ref, "..") && !strings.HasPrefix(ref, "/") && !strings.HasPrefix(ref, `\`)
}
