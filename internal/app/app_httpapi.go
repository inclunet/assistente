package app

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"

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

	var external *auth.ExternalAuthenticator
	if authCfg.Mode == "external" {
		external = auth.NewExternalAuthenticator(auth.ExternalAuthConfig{
			Issuer:            authCfg.External.Issuer,
			Audience:          authCfg.External.Audience,
			JWKSURL:           authCfg.External.JWKSURL,
			AllowedAlgorithms: authCfg.External.AllowedAlgorithms,
			RequiredScopes:    authCfg.External.RequiredScopes,
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
		return fmt.Errorf("iniciar HTTP API em %s: %w", authCfg.HTTP.BindAddress, err)
	}
	server := &http.Server{Handler: handler}
	a.httpAPIServer = server

	go func() {
		var serveErr error
		if authCfg.HTTP.TLSEnabled {
			serveErr = server.ServeTLS(listener, authCfg.HTTP.TLSCertFile, authCfg.HTTP.TLSKeyFile)
		} else {
			serveErr = server.Serve(listener)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("[httpapi] servidor encerrado com erro: %v", serveErr)
		}
	}()
	log.Printf("[httpapi] escutando em %s (mode=%s tls=%v)", listener.Addr().String(), authCfg.Mode, authCfg.HTTP.TLSEnabled)
	return nil
}
