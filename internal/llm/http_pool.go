package llm

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

var (
	// sharedTransport é o transport compartilhado para reutilização de conexões
	sharedTransport *http.Transport

	// SharedHTTPClient é um cliente HTTP otimizado para reutilização de conexões
	// Deve ser usado por todos os componentes que fazem requisições HTTP
	SharedHTTPClient *http.Client

	// responseHeaderTimeout armazena o timeout configurado (padrão: 180s)
	responseHeaderTimeout = 180 * time.Second
)

func init() {
	initTransport(180) // Inicializa com valor padrão
}

// initTransport inicializa o transport com o timeout especificado
func initTransport(timeoutSeconds int) {
	// #region agent log
	fmt.Printf("🔧 [HTTP_POOL] initTransport chamado - input=%d\n", timeoutSeconds)
	// #endregion
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	responseHeaderTimeout = time.Duration(timeoutSeconds) * time.Second
	// #region agent log
	fmt.Printf("🔧 [HTTP_POOL] ResponseHeaderTimeout definido para %v\n", responseHeaderTimeout)
	// #endregion

	sharedTransport = &http.Transport{
		// Connection pooling
		MaxIdleConns:        100,              // Máximo de conexões idle no pool
		MaxIdleConnsPerHost: 20,               // Máximo por host (importante para APIs LLM)
		MaxConnsPerHost:     100,              // Máximo de conexões por host
		IdleConnTimeout:     90 * time.Second, // Tempo máximo idle antes de fechar

		// Timeouts de conexão
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second, // Timeout para estabelecer conexão
			KeepAlive: 30 * time.Second, // Intervalo de keep-alive
		}).DialContext,

		// TLS handshake timeout
		TLSHandshakeTimeout: 10 * time.Second,

		// Timeouts de resposta
		ResponseHeaderTimeout: responseHeaderTimeout, // Tempo máximo para receber headers
		ExpectContinueTimeout: 1 * time.Second,

		// Compressão
		DisableCompression: false, // Permite gzip para respostas menores
	}

	SharedHTTPClient = &http.Client{
		Transport: sharedTransport,
		// Nota: Timeout global não é definido aqui para permitir
		// que cada chamada defina seu próprio timeout via context
	}
}

// ConfigureResponseTimeout configura o timeout de resposta em segundos
// Deve ser chamado durante a inicialização da aplicação após carregar a config
func ConfigureResponseTimeout(timeoutSeconds int) {
	// #region agent log
	fmt.Printf("🔧 [HTTP_POOL] ConfigureResponseTimeout chamado com %d segundos\n", timeoutSeconds)
	// #endregion
	initTransport(timeoutSeconds)
}

// NewHTTPClientWithTimeout cria um cliente que usa o transport compartilhado
// mas com um timeout específico (para casos que precisam de timeout diferente)
func NewHTTPClientWithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: sharedTransport,
		Timeout:   timeout,
	}
}

// GetSharedTransport retorna o transport compartilhado para uso em clientes customizados
func GetSharedTransport() http.RoundTripper {
	return sharedTransport
}

// GetResponseHeaderTimeout retorna o timeout configurado para receber headers
func GetResponseHeaderTimeout() time.Duration {
	return responseHeaderTimeout
}
