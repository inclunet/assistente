package contextprovider

import "context"

type Volatility string

const (
	VolatilityStable      Volatility = "stable"
	VolatilityLowDynamic  Volatility = "low_dynamic"
	VolatilitySlowDynamic Volatility = "slow_dynamic"
	VolatilityMidDynamic  Volatility = "mid_dynamic"
	VolatilityRolling     Volatility = "rolling_dynamic"
	VolatilityFastDynamic Volatility = "fast_dynamic"
	VolatilityTurnDynamic Volatility = "turn_dynamic"
)

type Surface struct {
	Type    string
	Title   string
	State   map[string]any
	Context map[string]any
}

type Tab struct {
	Title     string
	Type      string
	ContentID string
	IsActive  bool
	State     map[string]any
}

type LinkedTaskList struct {
	ID          string
	Title       string
	Description string
	Tasks       []LinkedTask
}

type LinkedTask struct {
	ID         string
	Title      string
	Status     string
	StatusIcon string
}

type BuildRequest struct {
	ConversationID      string
	WorkspaceID         string
	ProjectID           string
	WorkspaceName       string
	WorkspaceProfile    string
	TabCount            int
	ActiveTabTitle      string
	ActiveTabType       string
	Tabs                []Tab
	CurrentUserText     string
	ConversationSummary string
	SlashSkillContent   string
	Surface             *Surface
	EnabledSkills       []string
	DisableSkills       bool
	DisableOnDemand     bool
	ToolCallingEnabled  bool
	EnabledTools        []string
	ProviderBudgets     map[string]int
	ProviderEnabled     map[string]bool
	ProviderSettings    map[string]map[string]any

	// TaskListContextEnabled carries the chat skill policy decision into the
	// tasklist provider. It should only be true when tasklist-manager is enabled
	// for the active profile and global skill disabling does not apply.
	TaskListContextEnabled bool
	LinkedTaskLists        []LinkedTaskList
}

func (r BuildRequest) Budget(provider string, fallback int) int {
	if r.ProviderBudgets == nil {
		return fallback
	}
	if budget := r.ProviderBudgets[provider]; budget > 0 {
		return budget
	}
	return fallback
}

func (r BuildRequest) Enabled(provider string) bool {
	if r.ProviderEnabled == nil {
		return true
	}
	enabled, ok := r.ProviderEnabled[provider]
	if !ok {
		return true
	}
	return enabled
}

func (r BuildRequest) Settings(provider string) map[string]any {
	if r.ProviderSettings == nil {
		return nil
	}
	return r.ProviderSettings[provider]
}

type Block struct {
	Provider   string
	Name       string
	Volatility Volatility
	Priority   int
	Content    string
}

type Provider interface {
	Name() string
	Build(ctx context.Context, req BuildRequest) ([]Block, error)
}

type ProviderMetadata struct {
	Name             string `json:"name"`
	DisplayName      string `json:"display_name"`
	Description      string `json:"description"`
	DefaultEnabled   bool   `json:"default_enabled"`
	DefaultBudget    int    `json:"default_budget"`
	SupportsSettings bool   `json:"supports_settings"`
}

type MetadataProvider interface {
	Metadata() ProviderMetadata
}
