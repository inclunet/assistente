package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// CallbackServer gerencia o servidor HTTP local para callbacks OAuth
type CallbackServer struct {
	server   *http.Server
	listener net.Listener
	port     int

	// Canais para comunicação
	resultChan chan *CallbackResult

	// Estado pendente
	mu           sync.Mutex
	pendingState string
	providerID   string
}

// NewCallbackServer cria um novo servidor de callback
func NewCallbackServer() *CallbackServer {
	return &CallbackServer{
		resultChan: make(chan *CallbackResult, 1),
	}
}

// Start inicia o servidor em uma porta disponível
func (s *CallbackServer) Start() error {
	// Encontra uma porta disponível
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("erro ao criar listener: %w", err)
	}

	s.listener = listener
	s.port = listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleCallback)
	mux.HandleFunc("/", s.handleRoot)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		s.server.Serve(listener)
	}()

	return nil
}

// Stop para o servidor
func (s *CallbackServer) Stop() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}
}

// GetRedirectURI retorna a URI de redirecionamento
func (s *CallbackServer) GetRedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", s.port)
}

// GetPort retorna a porta do servidor
func (s *CallbackServer) GetPort() int {
	return s.port
}

// SetPendingAuth define a autenticação pendente
func (s *CallbackServer) SetPendingAuth(providerID, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingState = state
	s.providerID = providerID
}

// WaitForCallback aguarda o callback OAuth
func (s *CallbackServer) WaitForCallback(timeout time.Duration) (*CallbackResult, error) {
	select {
	case result := <-s.resultChan:
		return result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout aguardando callback OAuth")
	}
}

// handleCallback processa o callback OAuth
func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Verifica erro
	if errCode := query.Get("error"); errCode != "" {
		errDesc := query.Get("error_description")
		s.resultChan <- &CallbackResult{
			Error: fmt.Sprintf("%s: %s", errCode, errDesc),
		}
		s.writeErrorPage(w, errCode, errDesc)
		return
	}

	code := query.Get("code")
	state := query.Get("state")

	// Valida state
	s.mu.Lock()
	expectedState := s.pendingState
	providerID := s.providerID
	s.mu.Unlock()

	if state != expectedState {
		s.resultChan <- &CallbackResult{
			Error: "state inválido - possível ataque CSRF",
		}
		s.writeErrorPage(w, "invalid_state", "State parameter mismatch")
		return
	}

	// Sucesso!
	s.resultChan <- &CallbackResult{
		Code:  code,
		State: state,
	}

	s.writeSuccessPage(w, providerID)
}

// handleRoot página inicial
func (s *CallbackServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
	<title>OAuth - Assistente</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; 
		       display: flex; align-items: center; justify-content: center; height: 100vh; 
		       margin: 0; background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%); color: white; }
		.container { text-align: center; padding: 40px; }
		h1 { font-size: 24px; margin-bottom: 16px; }
		p { color: #888; }
	</style>
</head>
<body>
	<div class="container">
		<h1>🔐 Servidor OAuth</h1>
		<p>Aguardando autorização...</p>
	</div>
</body>
</html>`)
}

// writeSuccessPage escreve página de sucesso
func (s *CallbackServer) writeSuccessPage(w http.ResponseWriter, providerID string) {
	provider := GetProvider(providerID)
	providerName := providerID
	providerIcon := "✅"
	if provider != nil {
		providerName = provider.Name
		providerIcon = provider.Icon
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
	<title>Autorização Concluída - Assistente</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; 
		       display: flex; align-items: center; justify-content: center; height: 100vh; 
		       margin: 0; background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 100%%); color: white; }
		.container { text-align: center; padding: 40px; background: rgba(255,255,255,0.1); 
		             border-radius: 16px; backdrop-filter: blur(10px); }
		.icon { font-size: 64px; margin-bottom: 16px; }
		h1 { font-size: 24px; margin-bottom: 8px; color: #4ade80; }
		p { color: #888; margin-bottom: 24px; }
		.close-hint { font-size: 14px; color: #666; }
	</style>
</head>
<body>
	<div class="container">
		<div class="icon">%s</div>
		<h1>Conectado com sucesso!</h1>
		<p>Sua conta %s foi autorizada.</p>
		<p class="close-hint">Você pode fechar esta janela e voltar ao Assistente.</p>
	</div>
	<script>
		// Tenta fechar a janela automaticamente após 3 segundos
		setTimeout(function() {
			window.close();
		}, 3000);
	</script>
</body>
</html>`, providerIcon, providerName)
}

// writeErrorPage escreve página de erro
func (s *CallbackServer) writeErrorPage(w http.ResponseWriter, errCode, errDesc string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
	<title>Erro de Autorização - Assistente</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; 
		       display: flex; align-items: center; justify-content: center; height: 100vh; 
		       margin: 0; background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 100%%); color: white; }
		.container { text-align: center; padding: 40px; background: rgba(255,255,255,0.1); 
		             border-radius: 16px; backdrop-filter: blur(10px); }
		.icon { font-size: 64px; margin-bottom: 16px; }
		h1 { font-size: 24px; margin-bottom: 8px; color: #f87171; }
		p { color: #888; }
		.error-code { font-family: monospace; background: rgba(0,0,0,0.3); padding: 8px 16px; 
		              border-radius: 8px; margin-top: 16px; }
	</style>
</head>
<body>
	<div class="container">
		<div class="icon">❌</div>
		<h1>Erro na Autorização</h1>
		<p>%s</p>
		<div class="error-code">%s</div>
	</div>
</body>
</html>`, errDesc, errCode)
}

// GenerateState gera um state aleatório para proteção CSRF
func GenerateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}




