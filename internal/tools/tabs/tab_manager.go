package tabs

// TabInfo contém as informações de uma aba necessárias para as tools.
type TabInfo struct {
	ID             uint
	Title          string
	IsActive       bool
	Position       int
	ConversationID *uint
}

// TabManager abstrai as operações de gerenciamento de abas,
// permitindo que as tools interajam com o sistema de abas sem acoplamento direto.
type TabManager interface {
	GetAllTabs() ([]TabInfo, error)
	GetActiveTab() (*TabInfo, error)
	UpdateTabTitle(id uint, title string) error
	CloseTab(id uint) error
}
