package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"assistente/internal/app"
	"assistente/internal/database"

	"golang.org/x/term"
)

type providerAuthBackend interface {
	GetAuthStatus() (app.AuthStatus, error)
	SetupVault(string) (string, error)
	UnlockVault(string, string) error
	CreateAdminUser(app.CreateAdminRequest) (*database.User, error)
	Login(app.LoginRequest) (*app.AuthUser, error)
	RefreshAuth(app.RefreshRequest) (*app.AuthUser, error)
}

// prepareProviderSession usa o contrato de autenticação do desktop. Não escolhe
// usuário por inferência, nem cria contexto privilegiado para o cadastro.
func prepareProviderSession(svc providerAuthBackend, out io.Writer, reader *bufio.Reader, readPwd passwordReader) error {
	p := providerPrompt{reader, out}
	status, err := svc.GetAuthStatus()
	if err != nil {
		return err
	}
	if !status.VaultConfigured {
		secret, err := readMasterPassword(readPwd)
		if err != nil {
			return err
		}
		recovery, err := svc.SetupVault(secret)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "Guarde a chave de recuperação em local seguro: %s\n", recovery); err != nil {
			return err
		}
		if _, err := p.text("Pressione Enter após guardar a chave", "", false); err != nil {
			return err
		}
	} else if !status.VaultUnlocked {
		secret, err := readPwd("Senha mestre: ")
		if err != nil {
			return err
		}
		if err := svc.UnlockVault("master", secret); err != nil {
			return err
		}
	}
	// Reconsulta após setup/unlock: o backend é a autoridade sobre cofre e contas.
	status, err = svc.GetAuthStatus()
	if err != nil {
		return err
	}
	if !status.VaultUnlocked {
		return fmt.Errorf("cofre bloqueado")
	}
	if status.HasUsers {
		if user, err := svc.RefreshAuth(app.RefreshRequest{}); err == nil && user != nil {
			return nil
		}
	}
	username, err := p.text("Usuário", "", true)
	if err != nil {
		return err
	}
	password, err := readPwd("Senha da conta: ")
	if err != nil {
		return err
	}
	if !status.HasUsers {
		confirm, err := readPwd("Confirmar senha da conta: ")
		if err != nil {
			return err
		}
		if password != confirm {
			return fmt.Errorf("as senhas não coincidem")
		}
		if _, err := svc.CreateAdminUser(app.CreateAdminRequest{Username: username, Password: password}); err != nil {
			return err
		}
	}
	user, err := svc.Login(app.LoginRequest{Username: username, Password: password, ClientLabel: "CLI"})
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("autenticação não retornou uma sessão")
	}
	return nil
}

func runProviderCommand(svc cliApp, out io.Writer, setup bool) error {
	reader := bufio.NewReader(os.Stdin)
	// Um único buffer também para segredos quando stdin é pipe. Criar outro
	// bufio.Reader pode consumir antecipadamente e perder as próximas respostas.
	readPwd := passwordReader(readPassword)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		readPwd = pipePasswordReader(reader, out)
	}
	if err := prepareProviderSession(svc, out, reader, readPwd); err != nil {
		return err
	}
	if setup {
		return runSetup(svc, out, reader, readPwd)
	}
	return runProvidersAdd(svc, out, reader, readPwd)
}

func pipePasswordReader(reader *bufio.Reader, out io.Writer) passwordReader {
	return func(label string) (string, error) {
		if _, err := fmt.Fprint(out, label); err != nil {
			return "", err
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
	}
}
