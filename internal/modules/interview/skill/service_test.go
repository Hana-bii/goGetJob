package skill

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServiceLoadsSkillMetadataAndReferences(t *testing.T) {
	service, err := NewService(Options{Root: "../../../skills"})
	require.NoError(t, err)

	skills := service.All()
	require.NotEmpty(t, skills)
	got, err := service.Get("java-backend")
	require.NoError(t, err)
	require.Equal(t, "java-backend", got.ID)
	require.NotEmpty(t, got.Categories)
	require.NotEmpty(t, got.Persona)
}

func TestCalculateAllocationHonorsPriority(t *testing.T) {
	service, err := NewService(Options{Root: "../../../skills"})
	require.NoError(t, err)
	categories := []Category{
		{Key: "base", Label: "Base", Priority: PriorityAlwaysOne},
		{Key: "core", Label: "Core", Priority: PriorityCore},
		{Key: "normal", Label: "Normal", Priority: PriorityNormal},
	}

	got := service.CalculateAllocation(categories, 5)

	require.Equal(t, 1, got["base"])
	require.GreaterOrEqual(t, got["core"], got["normal"])
	require.Equal(t, 5, got["base"]+got["core"]+got["normal"])
}

func TestBuildCustomSkillMapsKnownReferencesAndSafeSection(t *testing.T) {
	service, err := NewService(Options{Root: "../../../skills"})
	require.NoError(t, err)
	custom := service.BuildCustom([]Category{
		{Key: "mysql", Label: "MySQL", Priority: PriorityCore, Ref: "../secret", Shared: true},
	}, "a long enough jd")

	require.Equal(t, CustomSkillID, custom.ID)
	section := service.BuildReferenceSectionSafe(custom.ID, map[string]int{"mysql": 1})

	require.NotContains(t, section, "secret")
	require.NotEmpty(t, section)
}

func TestBuildReferenceSectionLoadsSkillLocalReference(t *testing.T) {
	service, err := NewService(Options{Root: "../../../skills"})
	require.NoError(t, err)

	section := service.BuildReferenceSectionSafe("ai-agent-dev", map[string]int{"AGENT_BASIS": 1})

	require.Contains(t, section, "Agent")
	require.NotContains(t, section, "未配置")
}
