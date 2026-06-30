package app

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"assistente/internal/auth"
	"assistente/internal/config"
	"assistente/internal/httpapi"
)

func (a *App) startHTTPAPI() error {
	authCfg, err := config.LoadAuthConfig()
	if err != nil {
		return fmt.Errorf("carregar auth.json: %w", err)
	}
	if !authCfg.HTTP.Enabled {
		return nil
	}
	if err := httpapi.ValidateBindSecurity(authCfg.HTTP.BindAddress, authCfg.HTTP.TLSEnabled, authCfg.HTTP.DevInsecure); err != nil {
		return err
	}

	// B17: DevInsecure faz HTTP plain. Se config caiu em produção por
	// engano (env var não setada, copy-paste), credenciais e JWTs
	// trafegam em texto. Aborta hard quando ASSISTENTE_ENV=production
	// e grita no log quando bind não é loopback (LAN exposta).
	if err := guardDevInsecure(authCfg); err != nil {
		return err
	}

	var external *auth.ExternalAuthenticator
	if authCfg.Mode == "external" {
		external = auth.NewExternalAuthenticator(auth.ExternalAuthConfig{
			Issuer:            authCfg.External.Issuer,
			Audience:          authCfg.External.Audience,
			JWKSURL:           authCfg.External.JWKSURL,
			AllowedAlgorithms: authCfg.External.AllowedAlgorithms,
			RequiredScopes:    authCfg.External.RequiredScopes,
			RoleClaim:         authCfg.External.RoleClaim,
		})
	}

	handler := httpapi.New(httpapi.Config{
		Vault:    a.vaultSvc,
		IDs:      a.identitySvc,
		Sessions: a.currentSessionService,
		Mode:     authCfg.Mode,
		External: external,
	}).Handler()

	listener, err := net.Listen("tcp", authCfg.HTTP.BindAddress)
	if err != nil {
		// M25: detecta porta em uso e dá uma mensagem útil — antes vinha
		// só "listen tcp 127.0.0.1:17652: bind: address already in use"
		// que é genérico para o usuário final.
		if isAddrInUseErr(err) {
			return fmt.Errorf("iniciar HTTP API em %s: porta já está em uso (outra instância do assistente rodando ou outro processo escutando)", authCfg.HTTP.BindAddress)
		}
		return fmt.Errorf("iniciar HTTP API em %s: %w", authCfg.HTTP.BindAddress, err)
	}

	// B15: timeouts conservadores para evitar Slowloris e clientes lentos
	// presos. Auth API processa requests pequenos (login/refresh/JWKS),
	// então 30s para read/write é folga grande sem deixar atacante
	// segurar conexão indefinidamente.
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	a.httpAPIServer = server

	go func() {
		var serveErr error
		if authCfg.HTTP.TLSEnabled {
			serveErr = server.ServeTLS(listener, authCfg.HTTP.TLSCertFile, authCfg.HTTP.TLSKeyFile)
		} else {
			serveErr = server.Serve(listener)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logging.Errorf(context.Background(), "app.app-httpapi", "[httpapi] servidor encerrado com erro: %v", serveErr)
		}
	}()
	logging.Warnf(context.Background(), "app.app-httpapi", "[httpapi] escutando em %s (mode=%s tls=%v)", listener.Addr().String(), authCfg.Mode, authCfg.HTTP.TLSEnabled)
	return nil
}

// guardDevInsecure aplica heurísticas para evitar que dev_insecure=true
// vaze para produção sem aviso (B17 do review do AEP-0052):
//
//   - ASSISTENTE_ENV=production: aborta hard. Não há caso de uso legítimo.
//   - Bind não-loopback com dev_insecure: log explícito com WARNING.
//     Loopback (127.0.0.1/localhost) já é coberto por ValidateBindSecurity.
func guardDevInsecure(cfg *config.AuthConfig) error {
	if !cfg.HTTP.DevInsecure {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ASSISTENTE_ENV")), "production") {
		return errors.New("HTTP API: dev_insecure=true não é permitido com ASSISTENTE_ENV=production")
	}
	host, _, err := net.SplitHostPort(cfg.HTTP.BindAddress)
	if err != nil {
		host = cfg.HTTP.BindAddress
	}
	ip := net.ParseIP(host)
	loopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !loopback {
		logging.Warnf(context.Background(), "app.app-httpapi", "[httpapi] WARNING: dev_insecure=true em bind não-loopback %q — credenciais e tokens trafegam em texto", cfg.HTTP.BindAddress)
	}
	return nil
}

// isAddrInUseErr reconhece o erro de porta já alocada em Linux/macOS/Windows
// para que o caller possa dar uma mensagem amigável (M25 do review).
func isAddrInUseErr(err error) bool {
	var sysErr *net.OpError
	if !errors.As(err, &sysErr) {
		return false
	}
	if errors.Is(sysErr.Err, syscall.EADDRINUSE) {
		return true
	}
	// Windows: WSAEADDRINUSE (10048). syscall.EADDRINUSE no Windows
	// é reexportado como 10048, mas em Linux/macOS é 98/48 — errors.Is
	// acima já cobre. O fallback de string é um seguro extra.
	return strings.Contains(strings.ToLower(err.Error()), "address already in use") ||
		strings.Contains(strings.ToLower(err.Error()), "only one usage of each socket address")
}
