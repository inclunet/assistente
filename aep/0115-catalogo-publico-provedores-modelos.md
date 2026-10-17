# AEP-0115 — Catálogo público de provedores e modelos

Status: In Progress — contrato aprovado; fases 1 (schema e asset inicial) e 2 (consumo, validação e cache backend) implementadas nos PRs empilhados; integração de provedores, modelos e vozes pendente

## Resumo

Esta AEP define um catálogo versionado, publicado como JSON no site Hugo do
Assistente, para fornecer ao aplicativo perfis de provedores, modelos, recursos,
limites conhecidos, rotas de proxy e vozes TTS. O catálogo elimina duplicação
de presets entre frontend e backend e melhora a configuração de conexões
conhecidas, preservando a autoridade das configurações locais do usuário.

O contrato separa o servidor escolhido pelo usuário, o formato/adaptador de API,
o modelo canônico e o ID exato que o endpoint aceita. Assim, proxies como
LiteLLM e agregadores como OpenRouter podem expor aliases sem que o aplicativo
adivinhe o modelo subjacente.

## Motivação

Hoje há presets duplicados em `frontend/src/config/providers.ts`,
`internal/providers/defaults.go` e documentação. URLs de cadastro e de chave
nem sempre são apresentadas como links úteis, e provedores conhecidos ainda
expõem campos de configuração que deveriam ser controlados pelo preset. O
catálogo permite atualizar informações úteis sem nova versão do aplicativo e
serve como contrato único para provedor, API, modelo e experiência de
configuração.

## Decisões

### Publicação e atualização

- O artefato inicial será `docs/static/json/assistente-catalog.json`, servido
  em `https://assistente.inclunet.com.br/json/assistente-catalog.json`.
- O documento inclui `schema_version`, `catalog_version` e `updated_at` para
  compatibilidade, ordenação e frescor. A procedência editorial não é requisito.
- O backend baixa, valida e mantém cache local do último catálogo válido. A
  interface continua utilizável sem rede e não bloqueia a abertura enquanto
  atualiza. Se o servidor suportar, usar ETag/Last-Modified para evitar
  downloads sem mudança.
- O catálogo é dado declarativo, nunca código: IDs de adaptadores, formatos,
  métodos de autenticação e rotas devem pertencer a allowlists implementadas no
  aplicativo. Um JSON remoto não pode instruir execução, encaminhamento
  arbitrário ou alteração de segurança.
- O aplicativo envia somente requisição de leitura do catálogo. Chaves,
  credenciais, endpoints personalizados, modelos privados e telemetria de uso
  não são publicados nem enviados ao site.

### Perfis de servidor e campos de configuração

Cada perfil tem `id` estável, nome exibido, família/grupo, `api_format`,
adaptador suportado, URL padrão e, quando aplicável, porta local padrão, modo
de autenticação, `signup_url` e `api_key_url`. Esses links são apresentados na
interface como links acessíveis e localizados.

O perfil também define uma política por campo, no mínimo `fixed`, `editable`,
`choice` ou `hidden`, além de valor padrão, obrigatoriedade e opções permitidas
quando pertinentes. O formulário aplica essa política. Em provedores conhecidos,
URL, formato da API, autenticação e demais parâmetros estruturais são fixos e
somente leitura por padrão. Uma exceção editável ou uma lista de escolhas só
existe quando declarada explicitamente pelo perfil e necessária para um caso
suportado; não há uma opção genérica para liberar campos bloqueados. Campos
`fixed` são enviados a partir do catálogo, sem controle editável na UI.
URLs do catálogo não podem carregar userinfo (usuário/senha embutidos); chaves
e credenciais nunca pertencem ao asset público. Perfis `hosted` exigem `https`;
perfis locais e de proxy podem declarar `http` quando o endpoint assim exigir.

Os perfis catalogados fornecem defaults e restrições de interface, não
substituem configurações já salvas. Atualizar o catálogo não reescreve endpoint,
formato, credenciais ou modelo de conexões existentes. Registros antigos só
recebem um perfil quando houver identidade explícita confiável; não inferir
provedor por hostname, marca, ID de modelo ou compatibilidade OpenAI. Os demais
continuam como configuração personalizada.

Perfis personalizados continuam disponíveis. Seu nome, URL, porta, opções e
credenciais ficam locais. O catálogo pode fornecer modelos genéricos de
configuração para formatos conhecidos, sem incluir os dados particulares do
usuário.

O ID estável do perfil (`servers[].id`) é persistido explicitamente na conexão
criada a partir daquele perfil. Ele não substitui nem se confunde com
`ProviderType`, família do servidor ou `api_format`; a conexão persiste cada
conceito em seu campo próprio conforme o contrato atual e a AEP-0097. A criação
grava o vínculo com o perfil e a família escolhida pelo preset, e o runtime
continua usando a identidade/configuração persistidas. IDs publicados não são
reutilizados para outro servidor. Se um perfil for descontinuado, seu ID e
metadados mínimos permanecem como entrada obsoleta/alias de migração para
preservar conexões existentes; não se reidentifica uma conexão por URL ou modelo.

As políticas de edição do catálogo se aplicam aos campos estruturais da
conexão. Elas não podem bloquear nem forçar capacidades de protocolo em runtime
que o contrato vigente define como editáveis, incluindo
`reasoning_content_mode` da AEP-0097. Para esses campos, o valor salvo pela
pessoa permanece soberano; o catálogo pode sugerir apenas um valor inicial
quando ainda não houver valor configurado.

### Modelos, recursos e rotas expostas

Cada modelo usa uma identidade canônica no catálogo, separada do ID remoto que
uma conexão envia. Metadados podem incluir fornecedor, nome, estado, modalidades
de entrada/saída, tarefas, ferramentas, saída estruturada, raciocínio, limites
de contexto/entrada/saída e capacidades TTS. Cada recurso usa `supported`,
`unsupported` ou `unknown`; limites só são preenchidos quando conhecidos e são
informativos, não um mecanismo para substituir validação do servidor.

Uma exposição associa explicitamente `(server_profile_id, api_format,
exposed_model_id)` ao modelo canônico, podendo declarar diferenças conhecidas de
capacidade. IDs são comparados integralmente. É proibido remover prefixos,
reescrever IDs ou inferir mapeamentos a partir de nomes parecidos. Se um alias
não puder ser associado com confiança, ele permanece utilizável e suas
capacidades são `unknown`.

O modelo que a conexão lista dinamicamente continua sendo a fonte do que aquela
conta/endereço pode acessar; o catálogo enriquece a seleção, não concede acesso.
Uma correspondência explícita só é aplicada à combinação exata de perfil de
servidor, formato e ID exposto.

Proxies e agregadores são perfis de servidor próprios, distintos do fornecedor
canônico do modelo. OpenRouter pode publicar IDs e metadados próprios; LiteLLM
pode expor `model_name` arbitrário configurado no gateway. O catálogo só associa
um alias LiteLLM ao modelo subjacente quando o administrador fornecer um
mapeamento explícito ou houver metadado confiável disponível. O aplicativo não
supõe que o alias revele o backend.

Vozes TTS são recursos associados ao modelo e à rota/API em que são válidas.
O catálogo distingue lista completa, parcial e desconhecida. Ausência de voz
não prova incompatibilidade. Voz inválida é diferente de campo `voice` não
suportado, conforme AEP-0113 e AEP-0038.
Cada exposição TTS declara `selection_mode`: `model_and_voice` quando o usuário
seleciona modelo e voz separadamente, ou `model_only` quando seleciona apenas o
modelo e `voice_id` deve permanecer vazio. O seletor oferece escolha de voz
somente no primeiro modo, conforme AEP-0038; completude da lista não altera
esse contrato.

### Envelope e formato dos registros

O formato v1 é JSON UTF-8. O envelope contém versão e coleções separadas para
servidores, modelos canônicos e exposições; cada exposição pode conter overrides
de capacidade e vozes aplicáveis àquela rota. A forma conceitual mínima é:

```json
{
  "schema_version": 1,
  "catalog_version": "2026-10-17.1",
  "updated_at": "2026-10-17T12:00:00Z",
  "servers": [{
    "id": "openrouter",
    "name": "OpenRouter",
    "provider_family": "openrouter",
    "api_format": "openai",
    "adapter_id": "openai",
    "base_url": "https://openrouter.ai/api/v1",
    "auth": {"mode": "bearer_api_key", "required": true},
    "signup_url": "https://openrouter.ai/signup",
    "api_key_url": "https://openrouter.ai/keys",
    "fields": {
      "base_url": {"policy": "fixed", "default": "https://openrouter.ai/api/v1"},
      "api_format": {"policy": "fixed", "default": "openai"},
      "auth.mode": {"policy": "fixed", "default": "bearer_api_key"}
    }
  }],
  "models": [{
    "id": "anthropic/claude-3.7-sonnet",
    "vendor": "Anthropic",
    "name": "Claude 3.7 Sonnet",
    "status": "active",
    "capabilities": {
      "input.text": "supported",
      "input.image": "supported",
      "output.text": "supported",
      "output.audio": "unsupported",
      "tools.function_calling": "supported",
      "reasoning": "unknown"
    },
    "limits": {"context_tokens": 200000, "max_output_tokens": 64000}
  }],
  "exposures": [{
    "server_profile_id": "openrouter",
    "api_format": "openai",
    "exposed_model_id": "anthropic/claude-3.7-sonnet",
    "canonical_model_id": "anthropic/claude-3.7-sonnet"
  }]
}
```

O exemplo é ilustrativo; o schema formal define enums, campos
obrigatórios/opcionais, tipos, limites de tamanho e URLs aceitas. O JSON Schema
valida a estrutura de cada registro. O backend decodifica o envelope em tipos
Go e valida a estrutura v1 e as mesmas invariantes relevantes antes de aceitar
qualquer documento remoto. Integridade entre coleções é uma validação semântica
complementar: IDs de servidor/modelo duplicados, referências inexistentes,
tuplas de exposição repetidas e defaults que não pertençam às opções declaradas
por campos `choice` invalidam o documento inteiro. O teste do pacote valida o
asset publicado e os casos semânticos; alterações no schema v1 devem atualizar
os tipos, o validador e esses casos juntos. `capabilities`
aceita somente os três estados definidos nesta AEP. Limites numéricos são inteiros
positivos. `voices.completeness` aceita `complete`, `partial` ou `unknown`; cada
item contém pelo menos ID e nome
localizável/fallback. Exposições são identificadas pela tupla servidor + formato
exato + ID do modelo exposto, que deve ser única no documento. `selection_mode`
aceita `model_and_voice` ou `model_only` e é obrigatório quando a exposição
oferece TTS.

### Relação com configuração e estado local

O catálogo descreve conhecimento de produto compartilhado. Configurações
efetivas da conexão e capacidades aprendidas por rejeições permanecem locais.
O catálogo não armazena erros observados por usuários, credenciais, prompts,
preços ou estado de compatibilidade individual. AEP-0113 continua responsável
pelo aprendizado local de campos não suportados.

`api_format` e família do servidor permanecem conceitos distintos conforme
AEP-0097. O catálogo escolhe valores iniciais e políticas de edição, mas não
altera conexões existentes nem sobrepõe a identidade explícita persistida.

## Fases

1. **Contrato e asset JSON — concluída neste PR.** Schema Draft 2020-12 validado
   contra o asset inicial publicado em `docs/static/json/`, cobrindo provedores
   hospedados, Ollama/LocalAI/llama.cpp, OpenRouter, LiteLLM, perfil
   personalizado, modelos, exposição por proxy e completude de vozes.
2. **Consumo e cache no backend — concluída no PR filho desta fase.** Buscar a
   URL fixa sem seguir redirects, validar estrutura e integridade semântica,
   persistir atomicamente a última versão válida com ETag/Last-Modified e expor
   snapshots tipados por API Wails. O cache é carregado localmente e a atualização
   ocorre em segundo plano; falhas mantêm o snapshot válido anterior.
3. **Configuração de provedores pelo catálogo — em andamento.** Persistir a
   associação explícita `server_profile_id` sem inferir perfis de conexões
   existentes; depois trocar os presets duplicados do frontend/backend por
   perfis catalogados, aplicar política fixa/editável por campo, manter
   customizados, mostrar links de cadastro/chave e preservar configurações
   salvas.
4. **Modelos, capacidades e proxies.** Associar IDs expostos apenas por
   mapeamento exato e integrar recursos/limites sem impedir aliases
   desconhecidos.
5. **Vozes TTS na API do catálogo.** Integrar a coleção de vozes do asset ao
   consumo tipado, associada à exposição de perfil, rota/API e modelo, incluindo
   estado de completude. Esta fase fornece os dados consumidos pela fase 4 do
   AEP-0113, que implementa a seleção e o comportamento de compatibilidade na
   interface. Assim, a dependência é unidirecional: AEP-0113 fase 4 depende das
   fases 1–5 desta AEP.
6. **Manutenção do catálogo.** Definir processo e cadência de atualização,
   validação e publicação. Sondagens de síntese potencialmente pagas podem ser
   usadas no processo editorial, com custo e escopo controlados; abrir a UI ou
   selecionar modelo não dispara sondagem paga automaticamente.

## Riscos

- Catálogo desatualizado pode sugerir capacidade ou limite incorreto; manter
  `unknown`, cache do último documento válido e validação na API real.
- Mudança de schema pode quebrar versões antigas; versionar schema e ignorar
  campos opcionais desconhecidos sem aceitar formatos incompatíveis.
- Um proxy pode usar alias sem revelar seu modelo; preservar o alias utilizável
  com capacidades desconhecidas até existir associação explícita.
- Políticas de campos muito rígidas podem impedir configuração necessária;
  cada campo e perfil deve declarar a política e permitir opções suportadas
  suficientes para o uso pretendido.
- Confiar em JSON remoto para selecionar comportamento executável ampliaria a
  superfície de ataque; tratar o catálogo como dados validados e usar
  adaptadores locais allowlisted.

## Critérios de aceitação

- [x] O asset público tem schema versionado, exemplos de provedor, modelo,
  exposição por proxy e estado de completude de vozes. Evidência: arquivos
  `docs/static/json/assistente-catalog.schema.json` e
  `docs/static/json/assistente-catalog.json`, servidos pelo Hugo sob `/json/`;
  validação Draft 2020-12 e conferência semântica das referências e unicidade
  deste asset registradas no PR. A validação semântica de catálogos baixados é
  coberta também pelo consumidor da fase 2.
- [x] A aplicação valida estrutura e integridade semântica do documento (IDs
  únicos, referências existentes, tuplas de exposição únicas e defaults de
  `choice`), mantém cache válido e continua a funcionar offline. Evidência:
  `internal/llmcatalog/{parser.go,validate.go,service.go,service_test.go}`;
  os testes cobrem asset publicado, rejeição semântica, cache offline,
  preservação após resposta inválida e revalidação condicional.
- [x] A atualização não bloqueia o startup, usa origem HTTPS fixa sem redirects,
  limita o tamanho do documento e grava cache por substituição atômica. Evidência:
  `internal/llmcatalog/service.go` e `GetLLMCatalog` em
  `internal/wailsapi/llm_catalog.go`.
- [ ] Provedores conhecidos aplicam as políticas de edição declaradas; URLs de
  cadastro e obtenção de chave são acessíveis e localizadas.
- [x] A conexão persiste o ID do perfil de servidor escolhido explicitamente,
  preserva a associação em exportação/importação e deixa conexões antigas sem
  perfil; agentes ACP e a conexão OAuth do ChatGPT não recebem perfil HTTP.
  Evidência: `llm.ProviderConfig.ServerProfileID`, coluna aditiva
  `llm_providers.server_profile_id`, conversão no `DBStore` e cobertura em
  `db_store_stream_timeout_test.go` e `portability/providers_catalog_test.go`.
- [ ] Atualizar o catálogo não sobrescreve configurações existentes nem envia
  dados locais ao site.
- [ ] Modelos e aliases só recebem capacidades por associação exata; IDs sem
  associação continuam selecionáveis com recursos desconhecidos.
- [ ] Nenhuma conexão é identificada por inferência de hostname ou prefixo de
  modelo; configurações antigas ambíguas permanecem personalizadas.
- [ ] A fase de vozes do AEP-0113 consome o catálogo e distingue dados completos,
  parciais e desconhecidos, sem sondagem paga automática durante o uso normal.
- [ ] O AEP e o índice são atualizados em cada PR que entregar uma fase, com
  evidências verificáveis e status sincronizado.
