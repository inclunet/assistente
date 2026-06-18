package contextprovider

import "context"

type Volatility string

const (
	VolatilityStable      Volatility = "stable"
	VolatilitySlowDynamic Volatility = "slow_dynamic"
	VolatilityFastDynamic Volatility = "fast_dynamic"
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
	ConversationID   string
	WorkspaceID      string
	ProjectID        string
	WorkspaceName    string
	WorkspaceProfile string
	TabCount         int
	ActiveTabTitle   string
	ActiveTabType    string
	Tabs             []Tab
	CurrentUserText  string
	Surface          *Surface
	ProviderBudgets  map[string]int

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
