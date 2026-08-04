package app

import (
	"assistente/internal/acp"
)

// ACPAgentSetup é o que a tela de provedores precisa para configurar um agente
// de código: onde ele está nesta máquina e sobre qual diretório ele vai agir.
//
// Os dois lados vêm juntos de propósito. Configurar um agente é decidir o
// comando e saber onde ele mexe em arquivos; separá-los em duas chamadas só
// faria a tela pedir em dois passos o que ela sempre mostra junto.
type ACPAgentSetup struct {
	// Found diz se há agente instalado. Falso não é erro: é o estado que a tela
	// precisa explicar, com o que fazer para resolver (AEP-0084 Fase 3).
	Found bool `json:"found"`

	// Command e Args sobem o agente em modo ACP, já no formato que o provider
	// guarda.
	Command string   `json:"command"`
	Args    []string `json:"args"`

	// Version e Source dizem qual instalação respondeu, quando dá para saber.
	Version string `json:"version,omitempty"`
	Source  string `json:"source,omitempty"`

	// Searched são os lugares consultados. Só interessa quando não se achou
	// nada, e é o que transforma "não encontrado" em algo verificável.
	Searched []string `json:"searched,omitempty"`

	// WorkDir é o diretório sobre o qual o agente age (AEP-0084 D5): o
	// workspace ativo, ou o diretório de onde o app foi iniciado quando não há
	// workspace. Fica visível porque é onde o agente vai editar arquivos, e
	// esconder isso seria esconder o alcance do que a pessoa está autorizando.
	WorkDir string `json:"work_dir,omitempty"`
}

// DetectACPAgent procura na máquina o agente de código pedido e devolve, junto,
// o diretório sobre o qual ele vai agir.
//
// Exige sessão como o resto da API de provedores: a tela que chama só existe
// depois do login, e um sondador de sistema de arquivos aberto antes disso é
// superfície que não precisa existir.
func (a *App) DetectACPAgent(kind string) (ACPAgentSetup, error) {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return ACPAgentSetup{}, err
	}

	install, err := acp.DetectAgent(acp.AgentKind(kind))
	if err != nil {
		return ACPAgentSetup{}, err
	}

	setup := ACPAgentSetup{
		Found:    install.Found,
		Command:  install.Command,
		Args:     install.Args,
		Version:  install.Version,
		Source:   install.Source,
		Searched: install.Searched,
	}
	if setup.Args == nil {
		// Lista sempre presente: `null` faria a tela distinguir "sem
		// argumentos" de "campo ausente" antes de preencher o formulário.
		setup.Args = []string{}
	}
	// Falha aqui não invalida a detecção: sem diretório a tela deixa de mostrar
	// um dado, mas o comando encontrado continua valendo.
	if dir, err := a.acpWorkDir(); err == nil {
		setup.WorkDir = dir
	}
	return setup, nil
}
