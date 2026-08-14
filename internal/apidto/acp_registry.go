package apidto

// Estados de um agente do catálogo nesta máquina (AEP-0086 Fase 2).
//
// São seis, e não os três que a Fase 2 enumera, porque três não cobrem o que o
// app de fato sabe. A Fase 2 lista "encontrado por detecção", "não encontrado" e
// "requisito ausente"; os outros três existem para não transformar ignorância em
// afirmação:
//
//   - a detecção sabe procurar dois agentes dos 38 (D1), e dizer "não
//     encontrado" para os outros 36 alegaria uma procura que não aconteceu;
//   - um agente distribuído só como binário pode não ter alvo para esta
//     plataforma — em Windows ARM isso é a regra, não a exceção;
//   - a procura pode não ter dado para ser concluída (permissão negada), e aí
//     "não encontrado" mandaria reinstalar o que talvez já esteja lá.
const (
	// ACPCatalogStateInstalled é o agente que está nesta máquina, seja porque o
	// app o instalou (D5), seja porque a detecção o achou instalado por fora
	// (D1). Qual dos dois foi está em InstalledByApp, porque o que dá para fazer
	// com um e com o outro é diferente.
	ACPCatalogStateInstalled = "installed"

	// ACPCatalogStateRequirementMissing é o runtime que o agente exige e não
	// está aqui (D7). Vence "não encontrado" porque é o que bloqueia primeiro:
	// sem Node, instalar o pacote npm não é uma opção.
	ACPCatalogStateRequirementMissing = "requirement_missing"

	// ACPCatalogStateNoPlatformTarget é o agente sem forma de chegar a esta
	// máquina: só binário, e sem alvo para este sistema e arquitetura.
	ACPCatalogStateNoPlatformTarget = "no_platform_target"

	// ACPCatalogStateNotInstalled é o agente que a detecção procurou e não
	// achou. Só vale para os agentes que a detecção conhece.
	ACPCatalogStateNotInstalled = "not_installed"

	// ACPCatalogStateNoDetection é o agente para o qual este app não tem
	// detecção. Não é "não instalado": o app não olhou, e não tem como olhar.
	ACPCatalogStateNoDetection = "no_detection"

	// ACPCatalogStateDetectionFailed é a procura que não pôde ser concluída.
	ACPCatalogStateDetectionFailed = "detection_failed"
)

// ACPCatalog é o catálogo do registro ACP pronto para a tela (AEP-0086 Fase 2).
//
// Ele já vem resolvido: o estado de cada agente nesta máquina, o runtime que ele
// exige e o que se sabe sobre a integridade do artefato saem de regras do AEP, e
// deixar a tela deduzi-las do documento cru seria reimplementar o AEP em
// TypeScript.
type ACPCatalog struct {
	// Version é o `version` do documento do registro, já validado.
	Version string `json:"version,omitempty"`

	// Agents é o catálogo inteiro, ordenado por nome. Vazio quando não houve
	// como carregá-lo — e aí ReasonCode diz por quê (D2).
	Agents []ACPCatalogAgent `json:"agents"`

	// FetchedAt é quando o catálogo foi coletado, em RFC 3339. Vazio quando não
	// há catálogo. A tela mostra a idade em texto, e é ela quem formata: só ela
	// sabe o idioma de quem lê.
	FetchedAt string `json:"fetched_at,omitempty"`

	// AgeSeconds é a idade do que está sendo servido.
	AgeSeconds int64 `json:"age_seconds"`

	// FromCache diz que o catálogo veio do que estava guardado em disco.
	FromCache bool `json:"from_cache"`

	// Stale diz que a idade passou do prazo de revalidação. O conteúdo continua
	// útil, e a revalidação já foi disparada em segundo plano (D2).
	Stale bool `json:"stale"`

	// ReasonCode explica, como vocabulário fechado, por que o catálogo está
	// vazio ou por que a última atualização não aconteceu. A frase é da tela,
	// nos três locales.
	ReasonCode string `json:"reason_code,omitempty"`

	// ReasonDetail é a parte variável do motivo, quando existe: o erro de
	// transporte, já saneado.
	ReasonDetail string `json:"reason_detail,omitempty"`

	// Platform é o alvo desta máquina no vocabulário do registro
	// (`windows-x86_64`). Vazio quando o registro não nomeia esta combinação de
	// sistema e arquitetura.
	Platform string `json:"platform,omitempty"`
}

// ACPCatalogAgent é uma linha do catálogo com o que a tela precisa dizer sobre
// ela. Todo o texto já passou pelo saneamento da fronteira (D9).
type ACPCatalogAgent struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	License     string   `json:"license,omitempty"`
	Website     string   `json:"website,omitempty"`
	Repository  string   `json:"repository,omitempty"`

	// Distributions são as formas de obter o agente: `binary`, `npx`, `uvx`.
	Distributions []string `json:"distributions"`

	// Runtime é o pré-requisito nomeado desta máquina — `node`, `uv` — ou vazio
	// quando o agente não exige nenhum (D7).
	Runtime string `json:"runtime,omitempty"`

	// RuntimeFound diz se esse pré-requisito está nesta máquina. Sem Runtime, é
	// sempre falso e não quer dizer nada: não há o que encontrar.
	RuntimeFound bool `json:"runtime_found"`

	// RuntimePath é o executável do runtime que atendeu. Ele fica visível porque
	// é a prova de qual instalação o app achou, que é o dado de quem tem duas
	// versões de Node na máquina.
	RuntimePath string `json:"runtime_path,omitempty"`

	// Integrity é o que se sabe sobre conferir o artefato binário nesta
	// plataforma (D4): `not_distributed`, `no_platform_target`, `digest` ou
	// `no_digest`.
	Integrity string `json:"integrity"`

	// State é o estado nesta máquina, entre as constantes ACPCatalogState*.
	State string `json:"state"`

	// StateDetail é o complemento técnico do estado, quando ele tem um: o
	// caminho da instalação encontrada, ou o motivo de a procura não ter dado.
	// Já saneado, e ele complementa a frase da tela em vez de substituí-la.
	StateDetail string `json:"state_detail,omitempty"`

	// DetectedVersion é a versão da instalação que a detecção achou, quando o
	// layout dela revela. Não é a versão do registro: são duas coisas, e é a
	// diferença entre as duas que a Fase 7 vai usar para avisar de atualização.
	DetectedVersion string `json:"detected_version,omitempty"`

	// InstalledByApp diz que quem pôs este agente na máquina foi este app (D5).
	// A distinção importa porque muda o que dá para fazer com ele: o que o app
	// instalou, ele sabe onde está, sabe atualizar e sabe remover; o que veio de
	// fora, ele apenas reconheceu.
	InstalledByApp bool `json:"installed_by_app,omitempty"`

	// InstalledVersion é a versão que o app instalou. Ela é separada da
	// detectada porque as duas têm destinos diferentes: é esta que a Fase 7
	// compara com a do registro, já que atualizar só faz sentido para o que o
	// app mesmo pôs ali.
	InstalledVersion string `json:"installed_version,omitempty"`

	// InstalledUnverified é a instalação cujo artefato o app não teve como
	// conferir contra digest publicado (D4). A marca acompanha o agente depois
	// de instalado, e não desaparece com o diálogo em que ela foi aceita: quem
	// abrir o catálogo semanas depois precisa saber o que aquele arquivo vale.
	InstalledUnverified bool `json:"installed_unverified,omitempty"`
}
