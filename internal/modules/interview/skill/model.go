package skill

const CustomSkillID = "custom"

const (
	PriorityAlwaysOne = "ALWAYS_ONE"
	PriorityCore      = "CORE"
	PriorityNormal    = "NORMAL"
)

type Skill struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Categories  []Category `json:"categories"`
	IsPreset    bool       `json:"isPreset"`
	SourceJD    string     `json:"sourceJd,omitempty"`
	Persona     string     `json:"persona,omitempty"`
	Display     *Display   `json:"display,omitempty"`
}

type Category struct {
	Key      string `json:"key" yaml:"key"`
	Label    string `json:"label" yaml:"label"`
	Priority string `json:"priority" yaml:"priority"`
	Ref      string `json:"ref,omitempty" yaml:"ref"`
	Shared   bool   `json:"shared" yaml:"shared"`
}

type Display struct {
	Icon      string `json:"icon" yaml:"icon"`
	Gradient  string `json:"gradient" yaml:"gradient"`
	IconBg    string `json:"iconBg" yaml:"iconBg"`
	IconColor string `json:"iconColor" yaml:"iconColor"`
}

type metaFile struct {
	DisplayName string     `yaml:"displayName"`
	Display     *Display   `yaml:"display"`
	Categories  []Category `yaml:"categories"`
}

type frontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}
