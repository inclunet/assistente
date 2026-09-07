package apidto

// CustomActionView é a projeção de uma custom action para a UI (AEP-0067).
// Não expõe templates/condições — apenas o necessário para renderizar o item.
type CustomActionView struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Icon     string `json:"icon,omitempty"`
	Danger   bool   `json:"danger,omitempty"`
	Confirm  string `json:"confirm,omitempty"`
	HasEvent bool   `json:"hasEvent"`
	HasLink  bool   `json:"hasLink"`
}
