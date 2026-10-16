package apidto

// CommandCatalogFilter é o filtro da Command Palette/API de catálogo.
type CommandCatalogFilter struct {
	Locale string `json:"locale"`
	Query  string `json:"query"`
	Source string `json:"source"`
}

// CommandCatalogItem é a projeção segura de apresentação de um comando.
type CommandCatalogItem struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	Category            string   `json:"category"`
	Aliases             []string `json:"aliases"`
	Icon                string   `json:"icon"`
	Effect              string   `json:"effect"`
	Risk                string   `json:"risk"`
	Decision            string   `json:"decision"`
	Available           bool     `json:"available"`
	AvailabilityStatus  string   `json:"availabilityStatus"`
	AvailabilityReason  string   `json:"availabilityReason"`
	ReadinessReason     string   `json:"readinessReason,omitempty"`
	AllowedSources      []string `json:"allowedSources"`
	Scopes              []string `json:"scopes"`
	PresentationVersion string   `json:"presentationVersion"`
}

// CommandCatalogDetail acrescenta schemas e flags de contrato para tela de detalhe.
type CommandCatalogDetail struct {
	CommandCatalogItem
	ArgumentsSchema            any  `json:"argumentsSchema,omitempty"`
	ResultSchema               any  `json:"resultSchema,omitempty"`
	HasMutableTarget           bool `json:"hasMutableTarget"`
	MutatesEffectiveCapability bool `json:"mutatesEffectiveCapability"`
	ContextNone                bool `json:"contextNone"`
}
