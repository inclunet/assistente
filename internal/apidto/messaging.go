package apidto

// ConversationChannel é o canal/contato vinculados a uma conversa (borda Wails).
// Um único struct evita o descarte do segundo retorno que o gerador do Wails
// faz em métodos Go com (string, string, error).
type ConversationChannel struct {
	Channel   string `json:"channel"`
	ContactID string `json:"contactId"`
}
