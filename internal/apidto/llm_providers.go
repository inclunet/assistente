package apidto

// CreateLLMProviderRequest é o payload Wails para criar um provedor LLM.
type CreateLLMProviderRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
	APIFormat    string `json:"api_format,omitempty"`
	// ACPCommand e ACPArgs endereçam o agente de código quando APIFormat é
	// acp: é o que substitui BaseURL e APIKey, que ali não existem
	// (AEP-0084 D12).
	//
	// ACPEnv fica de fora de propósito. Variável de ambiente de processo é onde
	// token costuma parar, e não há tela que a edite: expô-la na fronteira só
	// criaria um caminho para segredo entrar sem que ninguém o veja. Quem
	// precisa dela usa a importação de configuração, que já a aceita.
	ACPCommand string   `json:"acp_command,omitempty"`
	ACPArgs    []string `json:"acp_args,omitempty"`
	// ACPAgentID é o agente do registro que a tela escolheu no catálogo
	// (AEP-0086 D11). Vazio é agente apontado à mão, que segue valendo.
	ACPAgentID string `json:"acp_agent_id,omitempty"`
	// ACPCredentialEnv diz quais variáveis do ambiente do agente recebem uma
	// credencial do cofre, e de qual entrada dele (AEP-0086 D12). Ao contrário
	// do ACPEnv, isto atravessa a fronteira: o que passa aqui é o nome da
	// entrada, não o segredo — ele continua saindo do cofre só na hora de subir
	// o processo, e nunca volta para a tela.
	ACPCredentialEnv map[string]string `json:"acp_credential_env,omitempty"`
}

// TestLLMProviderRequest é o payload Wails para testar um provedor LLM.
type TestLLMProviderRequest struct {
	Type       string `json:"type"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
}

// UpdateLLMProviderRequest é o payload Wails para atualizar um provedor LLM.
type UpdateLLMProviderRequest struct {
	Name         string `json:"name,omitempty"`
	Type         string `json:"type,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
	APIFormat    string `json:"api_format,omitempty"`
	// ACPCommand segue a convenção dos demais campos daqui: vazio é "não
	// mexer".
	ACPCommand string `json:"acp_command,omitempty"`
	// ACPArgs é ponteiro porque, aqui, lista vazia é edição legítima — tirar
	// todos os argumentos do agente —, e "vazio é não mexer" tornaria isso
	// impossível.
	ACPArgs *[]string `json:"acp_args,omitempty"`
	// ACPAgentID troca qual agente do registro este provedor é. É ponteiro
	// pela razão do ACPArgs: vazio aqui é edição de verdade, porque agente
	// apontado à mão é caminho válido (AEP-0086 D3) e é para onde volta quem
	// precisa desvincular o provedor do catálogo. Nulo é "não mexer".
	ACPAgentID *string `json:"acp_agent_id,omitempty"`
	// ACPCredentialEnv troca as variáveis que recebem credencial do cofre
	// (AEP-0086 D12). É ponteiro pela razão do ACPArgs: mapa vazio aqui é
	// desligar a passagem, que é a edição que alguém faz ao tirar a credencial
	// de um agente; nulo é "não mexer".
	ACPCredentialEnv *map[string]string `json:"acp_credential_env,omitempty"`
}
