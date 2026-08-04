package acp

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"assistente/internal/logging"
)

// HealthState é o resultado de sondar um provider de agente de código. São três
// estados, e não dois, porque "instalado mas sem login" tem instrução própria e
// resolve-se fora do app, no CLI do agente (AEP-0084 D12). Tratá-lo como falha
// de conexão mandaria a pessoa conferir comando e caminho que já estão certos.
type HealthState string

const (
	// HealthOnline é o agente que subiu, se apresentou e aceitou abrir sessão.
	HealthOnline HealthState = "online"

	// HealthUnauthenticated é o agente que subiu e se apresentou, mas recusou
	// abrir sessão por falta de login.
	HealthUnauthenticated HealthState = "unauthenticated"

	// HealthOffline é o agente que não subiu ou não se apresentou: comando
	// errado, binário quebrado, processo travado.
	HealthOffline HealthState = "offline"
)

// HealthReport é o que a sonda descobriu. Erro vem como texto já achatado
// porque parte dele nasce no agente, que é fonte não confiável (AEP-0084 D11).
type HealthReport struct {
	State HealthState

	// AgentName e AgentVersion são a identificação que o agente deu no
	// handshake. Servem de prova de que se falou com o programa esperado.
	AgentName    string
	AgentVersion string

	// AuthMethods são os métodos de login anunciados. Só interessam quando o
	// estado é HealthUnauthenticated: são eles que dizem como resolver.
	AuthMethods []AuthMethod

	// WorkDir é o diretório com que a sonda abriu a sessão — o mesmo que um
	// turno usaria (AEP-0084 D5).
	WorkDir string

	// Latency é quanto a sonda inteira levou: subir o processo (quando ainda
	// não estava de pé), o handshake e a sessão.
	Latency time.Duration

	// Error é o motivo, quando há um. Vazio em HealthOnline.
	//
	// Ele é saneado como rótulo, e não só achatado em uma linha: parte do texto
	// vem do agente (D11), a tela o mostra e o leitor de telas o lê. Achatar
	// deixava passar o resto de uma sequência de cor ("[31m") como se fosse
	// texto, e um erro de mil caracteres faria o anúncio recitar despejo de pilha
	// em vez de dizer o que fazer.
	Error string
}

// Unauthenticated diz se falta login. Existe para quem só precisa decidir entre
// "dá para usar" e "explique o login" não ter de comparar constante de estado.
func (r HealthReport) Unauthenticated() bool { return r.State == HealthUnauthenticated }

// Probe sonda o provider: sobe o agente se preciso, faz o handshake e tenta
// abrir uma sessão. A sessão faz parte da sonda de propósito — o handshake do
// Cursor termina bem mesmo sem login, e é `session/new` que responde
// `auth_required`. Sem ela, "saudável" incluiria um agente que não atende
// nenhum turno.
//
// A sessão da sonda é encerrada em seguida: ela existe para perguntar, não para
// conversar. O processo, ao contrário, fica de pé — é o mesmo que o primeiro
// turno usaria, e derrubá-lo aqui faria a pessoa pagar o spawn duas vezes
// (AEP-0084 D3).
func (m *Manager) Probe(ctx context.Context, spec ProviderSpec) HealthReport {
	start := time.Now()
	client, err := m.Client(spec)
	if err != nil {
		return HealthReport{State: HealthOffline, Error: SanitizeLabel(err.Error()), Latency: time.Since(start)}
	}
	return m.probe(ctx, spec, client, start)
}

// ProbeCandidate sonda uma configuração que ainda não é um provider: é o que a
// tela de cadastro chama para testar comando e argumentos antes de salvar.
//
// Aqui o processo é descartável, ao contrário do Probe. Não há provider a que
// ele pertença, e deixar de pé um agente de uma configuração que talvez nem
// seja salva acumularia processos a cada clique em "testar".
func (m *Manager) ProbeCandidate(ctx context.Context, spec ProviderSpec) HealthReport {
	start := time.Now()
	if err := spec.validate(); err != nil {
		return HealthReport{State: HealthOffline, Error: SanitizeLabel(err.Error()), Latency: time.Since(start)}
	}

	client, err := m.dial(Config{
		Command:       spec.Command,
		Args:          slices.Clone(spec.Args),
		Env:           maps.Clone(spec.Env),
		WorkDir:       m.processWorkDir(),
		ClientName:    m.clientName,
		ClientVersion: m.clientVersion,
	}, m.handler)
	if err != nil {
		return HealthReport{State: HealthOffline, Error: SanitizeLabel(err.Error()), Latency: time.Since(start)}
	}
	defer func() {
		if err := client.Close(); err != nil {
			logging.Warnf(ctx, managerComponent,
				"[ACP] erro ao encerrar o agente da sondagem de configuração: %v", err)
		}
	}()

	return m.probe(ctx, spec, client, start)
}

// probe é a sondagem em si, igual para o provider salvo e para a configuração
// em teste: quem chama decide de onde vem o cliente e quem o encerra.
func (m *Manager) probe(ctx context.Context, spec ProviderSpec, client Client, start time.Time) HealthReport {
	report := HealthReport{State: HealthOffline}

	caps, err := client.Capabilities(ctx)
	if err != nil {
		report.State = stateForError(err)
		report.Error = SanitizeLabel(err.Error())
		report.Latency = time.Since(start)
		return report
	}
	report.AgentName = SanitizeLabel(caps.AgentName)
	report.AgentVersion = SanitizeLabel(caps.AgentVersion)
	report.AuthMethods = sanitizedAuthMethods(caps.AuthMethods)

	dir, err := m.currentDir()
	if err != nil {
		report.Error = SanitizeLabel(err.Error())
		report.Latency = time.Since(start)
		return report
	}
	report.WorkDir = dir

	session, err := client.NewSession(ctx, dir)
	if err != nil {
		report.State = stateForError(err)
		report.Error = SanitizeLabel(err.Error())
		report.Latency = time.Since(start)
		return report
	}

	m.closeProbeSession(ctx, spec, session)
	report.State = HealthOnline
	report.Latency = time.Since(start)
	return report
}

// sanitizedAuthMethods trata nome e descrição dos métodos de login como o que
// eles são: texto vindo do agente (AEP-0084 D11). Eles chegam à tela junto com o
// estado sem login, e o mesmo cuidado do nome do agente vale aqui — escape de
// terminal, caractere de controle ou parágrafo inteiro no lugar de um rótulo. O
// ID sai intacto de propósito: ele identifica o método para o protocolo, não é
// para ler.
func sanitizedAuthMethods(methods []AuthMethod) []AuthMethod {
	if len(methods) == 0 {
		return nil
	}
	out := make([]AuthMethod, 0, len(methods))
	for _, method := range methods {
		method.Name = SanitizeLabel(method.Name)
		method.Description = SanitizeLabel(method.Description)
		out = append(out, method)
	}
	return out
}

// stateForError separa "faça login" de "não conectou". É a única distinção que
// a tela precisa fazer para escolher entre explicar o login e mandar conferir o
// comando.
func stateForError(err error) HealthState {
	if errors.Is(err, ErrNotAuthenticated) {
		return HealthUnauthenticated
	}
	return HealthOffline
}

// closeProbeSession despede a sessão da sonda. Falhar aqui não muda o
// diagnóstico — o agente respondeu, que é o que se estava perguntando —, mas
// fica no log: uma sessão que não fecha vai acumulando no agente.
func (m *Manager) closeProbeSession(ctx context.Context, spec ProviderSpec, session Session) {
	if session == nil {
		return
	}
	if err := session.Close(ctx); err != nil {
		logging.Warnf(ctx, managerComponent,
			"[ACP] erro ao encerrar a sessão de sondagem do provider %q: %v", spec.ID, err)
	}
}
