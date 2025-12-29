package llm

import (
	"net"
	"net/http"
	"time"
)

// SharedHTTPClient é um cliente HTTP otimizado para reutilização de conexões
// Deve ser usado por todos os componentes que fazem requisições HTTP
var SharedHTTPClient = &http.Client{
	Transport: &http.Transport{
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
		ResponseHeaderTimeout: 60 * time.Second, // Tempo máximo para receber headers
		ExpectContinueTimeout: 1 * time.Second,

		// Compressão
		DisableCompression: false, // Permite gzip para respostas menores
	},
	// Nota: Timeout global não é definido aqui para permitir
	// que cada chamada defina seu próprio timeout via context
}

// NewHTTPClientWithTimeout cria um cliente que usa o transport compartilhado
// mas com um timeout específico (para casos que precisam de timeout diferente)
func NewHTTPClientWithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: SharedHTTPClient.Transport,
		Timeout:   timeout,
	}
}

// GetSharedTransport retorna o transport compartilhado para uso em clientes customizados
func GetSharedTransport() http.RoundTripper {
	return SharedHTTPClient.Transport
}
