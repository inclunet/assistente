# AEP-0115 — Catálogo público de provedores e modelos

Status: In Progress — contrato aprovado; fases 1 (schema e asset inicial) e 2 (consumo, validação e cache backend) implementadas; fase 3 parcialmente entregue com persistência explícita, formulário por perfis, aceite por conexão para HTTP remoto e fábrica backend allowlisted; provedores legados sem aceite permanecem editáveis e bloqueados até reautorização explícita; onboarding desktop reutiliza o formulário; CLI consolidada com autenticação e cadastro compartilhados; fase 4 com resolução exata e contratos de parâmetros compartilhados entre editor e envio; aliases privados e vozes pendentes; Cohere e Ollama aguardam suporte às rotas/runtime catalogados

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
URLs `http` no catálogo só podem apontar para loopback (`localhost`, `127.0.0.0/8`
ou `::1`), inclusive em perfis locais e de proxy. Isso impede que uma alteração
indevida no asset direcione credenciais para um host remoto sem TLS. URLs remotas
`http` informadas manualmente pelo usuário exigem aviso e autorização explícita
antes de qualquer envio de credencial. A fase 3 integra esse percurso ao
DecisionDialog e persiste o aceite vinculado à URL exata da conexão; alterar a
URL invalida o aceite. O backend exige essa correspondência também para sondagem,
criação, edição e uso runtime de credenciais.

Conexões persistidas antes dessa exigência e sem aceite explícito permanecem
visíveis para edição, mas não podem ser usadas para chat, sumarização, health
check, voz ou sondagem autenticada. A migração não cria aceite retroativo. O
usuário precisa testar a conexão e autorizar a URL exata uma vez por conexão;
renovar a credencial na mesma URL não pede novo aceite. Mudar a URL invalida o
aceite anterior. Conexões legadas com URL inválida também permanecem visíveis
para correção e bloqueadas para uso até que a URL seja corrigida.

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
`unsupported` ou `unknown`; limites só são preenchidos quando conhecidos.
Esses dados têm uso funcional: delimitam parâmetros e opções no editor e são
validados pelo backend antes do envio. A validação do servidor continua sendo
necessária, pois o catálogo pode estar desatualizado.

### Contrato efetivo de parâmetros

O backend resolve um único contrato para o modelo e sua exposição exata. O
editor consome esse resultado; não mantém uma segunda tabela de faixas,
defaults ou opções por modelo. O envio usa a mesma resolução no pipeline
existente, incluindo chamadas originadas fora da interface gráfica.

- O catálogo pode declarar suporte, mínimo, máximo, opções e default de cada
  parâmetro. Restrições da exposição prevalecem sobre as do modelo canônico.
- Restrições aprendidas localmente pela AEP-0113 continuam prevalecendo sobre
  suporte declarado no catálogo, no escopo da conexão, modelo e operação.
- Ausência de informação significa desconhecido e mantém configuração manual;
  não cria um preset local alternativo nem infere associação por nome.
- Ausência de valor configurado e zero explícito são estados distintos. Um
  default catalogado só se aplica ao primeiro. Valores já configurados são
  preservados; atualizar o catálogo ou mudar o modelo não os reescreve.
- Valores explícitos fora do contrato são sinalizados para correção, sem
  truncamento, clamp ou troca silenciosa. Campos comprovadamente não suportados
  seguem a supressão existente da AEP-0113, sem apagar a configuração salva.
- O contrato resolve configuração, não duplica transporte, serialização de
  requisições, seleção de modelo ou fluxo de envio.

Esta extensão foi aprovada pelo mantenedor após a entrega da associação exata
no PR #952. Modelos declaram `parameters` e exposições declaram
`parameter_overrides`, com `support`, `minimum`, `maximum`, `choices` e `default`.
Um override substitui o contrato inteiro daquele campo, sem misturar restrições
de rotas diferentes. Os quatro campos são `temperature`, `top_p`,
`max_output_tokens` e `reasoning_effort`. O limite canônico de saída também
restringe o contrato canônico, mesmo quando este declara um default.

`Service.ChatParameterContract` resolve o contrato usado pelo editor e por
`ApplyChatParameterContract` no envio. Valores opcionais usam ponteiros até o
transporte, distinguindo ausência e zero sem flags paralelas. O limite conhecido
de contexto limita a configuração da janela; zero continua desativando o
gerenciamento automático. O orçamento efetivo de saída acompanha a resposta
até a sumarização, sem recarregar um default divergente do perfil salvo.
Chamadas internas `SimpleChat` conservam seu orçamento explícito de tarefa;
não são configuração de parâmetros do perfil. Adaptadores Anthropic e Google
não oferecem `reasoning_effort` enquanto não traduzirem esse campo canônico.
Regras condicionais entre parâmetros não são representadas nesta entrega.
Perfis legados com `reasoning_effort` omitido mantêm a escolha desativada;
novos perfis serializam `null` para o default do modelo, preservando a distinção
também ao exportar. O evento de invalidação `llm_catalog:changed`, sem payload,
é emitido após publicar um documento validado e faz o editor reconsultar seu
contrato sem alterar os valores do formulário nem desmontar controles durante
a revalidação. Se uma nova restrição remover o campo focado, o foco retorna ao
controle restante do grupo ou ao conteúdo da guia. Raciocínio manual também
distingue explicitamente desativado, default do modelo e valor personalizado.
Erros de valor carregam campo,
código e limite no marcador `parameter_error:v1`, traduzido pelo apresentador
de erros existente. A escolha do modelo reutiliza `llm.ResolveModel` em
catálogo, compatibilidade e transporte.
Domínios numéricos intrínsecos são aplicados por `Parameter.WithDomain` e
entregues no mesmo contrato, mesmo para modelos desconhecidos; o frontend não
repete uma tabela de limites. Sem orçamento de saída conhecido, a sumarização
mantém sua margem proporcional de segurança existente, sem inventar 4096 tokens
nem recuperar do perfil um campo que o transporte suprimiu.

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

1. **Contrato e asset JSON — concluída neste PR.** Schema Draft 2020-12 validado contra o asset inicial publicado em `docs/static/json/`, cobrindo provedores hospedados, Ollama/LocalAI/llama.cpp, OpenRouter, LiteLLM, perfil personalizado, modelos, exposição por proxy e completude de vozes.
2. **Consumo e cache no backend — concluída no PR filho desta fase.** Buscar a URL fixa sem seguir redirects, validar estrutura e integridade semântica, persistir atomicamente a última versão válida com ETag/Last-Modified e expor snapshots tipados por API Wails. O cache carrega localmente e atualiza em segundo plano; falhas mantêm o snapshot válido anterior. Timestamps futuros no cache mantêm o documento disponível como stale e forçam revalidação; o shutdown aguarda refreshes explícitos e de fundo.
3. **Configuração de provedores pelo catálogo — em andamento.** A persistência explícita de `server_profile_id` e o consumo pelo formulário estão implementados: lista perfis ativos, usa allowlists locais para família/formato/autenticação, aplica políticas de campo, oferece links acessíveis e localizados, mantém configuração customizada quando o catálogo falha e preserva valores salvos e entradas manuais durante o carregamento, sem associar perfis a conexões legadas. O `default_model_id` do perfil preenche uma conexão vinculada apenas quando o campo está vazio; valores salvos têm precedência, inclusive para políticas editáveis. A fábrica backend resolve perfis do snapshot por IDs explícitos e allowlists locais da tupla perfil/família/formato e autenticação. Aplica a mesma precedência da UI: políticas `fixed`, `hidden` e `choice` usam seu default; `editable` preserva o valor top-level e usa o default somente quando ausente ou vazio. O modo efetivo vem de `auth.mode`, sem depender de `auth.required` potencialmente divergente. Sem snapshot ou perfil compatível, a fábrica não cria presets locais: o fluxo manual continua disponível na configuração do provedor; a entrada legada de CLI retorna erro claro e não inventa URL ou modelo. O caminho Ollama mantém o adaptador runtime atual sem mapear genericamente o formato `ollama`; o perfil nativo permanece fora da fábrica até o suporte do protocolo ser definido e implementado. HTTP remoto manual exige aceite acessível antes da sondagem; o aceite fica salvo por conexão, vinculado à URL exata, é conferido no backend para o uso futuro e não é exportado na portabilidade. Conexões legadas sem aceite permanecem visíveis para edição, sem receber aceite retroativo, e são bloqueadas em chat, sumarização, health check, voz e sondagem até autorização explícita; renovar a credencial na mesma URL preserva o aceite. Conexões legadas com URL inválida também continuam editáveis e bloqueadas até a correção. O Cohere fica fora da UI e da fábrica enquanto o runtime não representar a rota catalogada `/compatibility/v1/chat/completions`; a configuração manual segue disponível. O onboarding desktop reutiliza o formulário de provedores; a CLI usa um cadastro compartilhado; modelos e vozes permanecem pendentes.
Na fábrica backend, a allowlist local inclui a origem completa (esquema, host e porta) junto à tupla perfil/família/formato e autenticação. Provedores hospedados e OpenRouter usam origens HTTPS fixas no runtime; LiteLLM fica limitado a `http://localhost:4000`, e LocalAI e llama.cpp a `http://localhost:8080`. Assim, o catálogo remoto não escolhe o destino de credenciais nem pode redirecionar uma chave para outro serviço local. Endpoints customizados permanecem no fluxo de configuração manual sob controle local do usuário.

A criação catalogada reutiliza a transação existente do cofre/consumidor
(`captureCredentialSave` / `saveWithCredential`) quando recebe chave; não usa
rollback manual nem exclui credenciais para tentar restaurá-las depois.
IDs existentes são recusados; edição continua pelo fluxo próprio. Cada
conexão nova usa referência explícita `provider:<id>`, isolada por usuário,
sem reaproveitar chaves pelo hostname, inclusive entre portas de localhost.
A tabela `BuiltinTemplate` foi removida. O onboarding desktop prepara o cofre
com os questionários existentes e abre o mesmo `ProviderForm` das configurações.
Não há formulário, sondagem ou persistência alternativa de provedor. O retorno
true de `Welcome.RunWelcomeWizard` significa cofre preparado; sucesso do onboarding
só é anunciado após a persistência autenticada do formulário compartilhado.
Cancelar preserva o cofre. Troca de sessão desmonta o draft e descarta resultados
atrasados; replay de efeitos não repete a preparação. O primeiro provedor criado
pelo controller compartilhado atualiza o cliente pelo hook existente.
A implementação está coberta por `WelcomeSetup.test.tsx`, pelos testes de
catálogo/credenciais de `ProviderForm`, por `controllers/welcome_catalog_test.go`
e pela integração em `internal/app/app_wizard_test.go`.
Os helpers e testes exclusivos do antigo percurso desktop foram removidos; as
validações de URL/conexão continuam cobertas diretamente no serviço compartilhado.
A CLI usa `configureProvider` para `setup` e `providers add`, com snapshots da
mesma API autenticada do catálogo, filtrados por `TemplateFromCatalogServer`.
As listas legadas de provedores/modelos do wizard foram removidas. Políticas
fixas/ocultas não são editáveis; escolhas e campos editáveis seguem o catálogo.
Configuração manual permanece disponível sem snapshot. Uma única listagem
sonda a conexão e oferece IDs sem reescrever aliases; o modelo confirmado é
persistido na conexão. `setup` define o ID retornado como padrão; `providers add`
preserva o padrão existente e os perfis de conversa. `setup` ajusta apenas o
perfil ativo para os sentinelas de provedor/modelo padrão, evitando que um
modelo explícito antigo oculte a nova escolha.
Credenciais usam o mesmo resolvedor e a transação do cadastro compartilhado;
HTTP remoto exige aceite explícito para a URL exata antes da sondagem.
No serviço de criação, toda credencial nova fornecida em draft ou API key
legada recebe `provider:<id>` após validação e é salva atomicamente com a
conexão. O draft original da sondagem e entradas anteriores por hostname ou
wildcard permanecem intactos. Criar uma conexão sem fornecer credencial nova
mantém o contrato de reutilização de referência existente; duplicação explícita
por ID continua compartilhando a referência autorizada, sem copiar segredos.
A criação de credencial recusa qualquer referência já existente no cofre,
inclusive sem consumidor, na mesma transação da conexão. Não enumera consumidores
nem interpreta seus metadados: cópias, agentes e consumidores futuros ficam
protegidos pela proibição de sobrescrita na origem. Recriar um ID excluído exige
um novo ID quando sua credencial permanece; atualização é uma operação explícita.
Edição preserva referências existentes, sem migração implícita. Regressões em
`internal/providers/credential_save_test.go` cobrem sobrescrita, portas locais,
entrada legada, rollback, duplicação explícita e preservação de fontes externas.
A autenticação de ambos reutiliza SetupVault/UnlockVault/CreateAdminUser/Login/
RefreshAuth, sem inferir usuário nem contornar o isolamento por sessão.
Evidências: `cmd/asst/provider_setup_test.go` e `provider_auth_test.go`, além dos
testes dos serviços de credenciais e provedores. A fase 3 permanece In Progress
quanto às rotas catalogadas ainda sem suporte (Cohere e Ollama nativo); modelos,
vozes e manutenção do catálogo continuam pendentes nas fases seguintes.

4. **Modelos, capacidades e proxies — parcialmente entregue.** A API existente
   `GetModelCatalogByProvider` e sua atualização resolvem a tupla exata do perfil
   persistido, formato usado pelo adaptador e ID listado pelo servidor. O
   resolvedor único `llmcatalog.Document.ResolveModels` aplica overrides da
   exposição sobre o modelo canônico e devolve cópias independentes. O seletor
   existente recebe nome legível acompanhado do ID remoto, sem mudar o valor
   selecionado. `asst providers models <id> --json` consome a mesma API e expõe
   capacidades, limites e contratos de parâmetros. Sem correspondência, `catalog`
   é ausente; capacidades ausentes significam desconhecidas. O catálogo nunca
   acrescenta modelos à lista da conta nem reescreve configuração ou aprendizado.
   Parâmetros ausentes recebem defaults conhecidos na cópia da requisição;
   valores explícitos são validados e campos não suportados são suprimidos.
   Evidências: `llmcatalog/resolve_test.go`,
   `providers/model_catalog_metadata_test.go` e `cmd/asst/providers_test.go`.
   O contrato efetivo está integrado ao editor e ao envio, com evidências em
   `llmcatalog/parameters_test.go`, `providers/chat_parameters_test.go`,
   `llm/parameter_presence_test.go`, `profiles/parameter_presence_test.go` e
   `ProfileChatSection.test.tsx`. Ainda falta oferecer o vínculo explícito de
   aliases privados; não se infere associação por nome.
5. **Vozes TTS na API do catálogo.** Integrar a coleção de vozes do asset ao consumo tipado, associada à exposição de perfil, rota/API e modelo, incluindo estado de completude. Esta fase fornece os dados consumidos pela fase 4 do AEP-0113, que implementa a seleção e o comportamento de compatibilidade na interface. Assim, a dependência é unidirecional: AEP-0113 fase 4 depende das fases 1–5 desta AEP.
6. **Manutenção do catálogo.** Definir processo e cadência de atualização, validação e publicação. Sondagens de síntese potencialmente pagas podem ser usadas no processo editorial, com custo e escopo controlados; abrir a UI ou selecionar modelo não dispara sondagem paga automaticamente.

## Riscos

- Catálogo desatualizado pode sugerir capacidade ou limite incorreto; manter `unknown`, cache do último documento válido e validação na API real. Um catálogo remoto adulterado não pode redirecionar credenciais de perfis conhecidos: o runtime fixa origens permitidas e endpoints customizados permanecem sob controle local do usuário.
- Mudança de schema pode quebrar versões antigas; versionar schema e ignorar campos opcionais desconhecidos sem aceitar formatos incompatíveis.
- Um proxy pode usar alias sem revelar seu modelo; preservar o alias utilizável com capacidades desconhecidas até existir associação explícita.
- Políticas de campos muito rígidas podem impedir configuração necessária; cada campo e perfil deve declarar a política e permitir opções suportadas suficientes para o uso pretendido.
- HTTP remoto pode expor credenciais. O asset limita esse esquema a loopback; configuração manual fora de loopback exige aceite explícito, persistido por URL exata e revalidado pelo backend. Esse aceite não torna o canal criptografado e deve ser invalidado quando o destino muda.
- Uma atualização pode encontrar conexões legadas HTTP sem registro de aceite. Elas devem permanecer gerenciáveis, sem aceite retroativo, e todo caminho que envie credenciais deve falhar fechado até consentimento explícito para a URL exata.
- Confiar em JSON remoto para selecionar comportamento executável ampliaria a superfície de ataque; tratar o catálogo como dados validados e usar adaptadores locais allowlisted.

## Critérios de aceitação

- [x] Editor e envio consomem uma resolução backend de parâmetros aceitos,
  faixas, opções e defaults por modelo e exposição. Valores explícitos,
  inclusive zero, são preservados; incompatibilidades são sinalizadas sem
  ajuste silencioso. Ausência de catálogo permite configuração manual, e
  restrições aprendidas mantêm precedência.

- [x] `asst setup` e `asst providers add` compartilham autenticação, formulário de catálogo/manual, sondagem única e cadastro pelo controller existente. O CLI remove listas legadas, preserva aliases e pede consentimento antes de HTTP remoto. Evidências: `cmd/asst/provider_setup_test.go`, `provider_auth_test.go` e `provider_setup_integration_test.go` (headers HTTP e persistência pelo controller/cofre reais).
- [x] O asset público tem schema versionado, exemplos de provedor, modelo, exposição por proxy e estado de completude de vozes. Evidência: `docs/static/json/assistente-catalog.schema.json` e `docs/static/json/assistente-catalog.json`, servidos pelo Hugo sob `/json/`; validação Draft 2020-12 e conferência semântica das referências e unicidade deste asset registradas no PR. A validação semântica de catálogos baixados é coberta também pelo consumidor da fase 2.
- [x] A aplicação valida estrutura e integridade semântica do documento (IDs únicos, referências existentes, tuplas de exposição únicas e defaults de `choice`), mantém cache válido e continua a funcionar offline. Evidência: `internal/llmcatalog/{parser.go,validate.go,service.go,service_test.go}`; testes cobrem asset publicado, rejeição semântica, cache offline, preservação após resposta inválida e revalidação condicional.
- [x] A atualização não bloqueia o startup, usa origem HTTPS fixa sem redirects, limita o tamanho do documento e grava cache por substituição atômica. Evidência: `internal/llmcatalog/service.go` e `GetLLMCatalog` em `internal/wailsapi/llm_catalog.go`; testes de serviço cobrem atualização em segundo plano, timestamps futuros no cache e espera de refreshes durante o shutdown.
- [x] A conexão persiste o ID do perfil de servidor escolhido explicitamente, preserva a associação em exportação/importação e deixa conexões antigas sem perfil; agentes ACP e a conexão OAuth do ChatGPT não recebem perfil HTTP. Evidência: `llm.ProviderConfig.ServerProfileID`, coluna aditiva `llm_providers.server_profile_id`, conversão no `DBStore` e cobertura em `db_store_stream_timeout_test.go`, `portability/providers_catalog_test.go` e rejeição em `providers/chatgpt_test.go` para updates de OAuth.
- [x] O formulário aplica perfis e políticas de campo, mostra links de cadastro/chave, preserva configurações salvas e mantém o modo customizado disponível offline. O validador rejeita defaults e opções fixas de URL, formato, autenticação e modelo que contrariem as allowlists e limites do contrato. Para HTTP remoto manual, o DecisionDialog exige aceite antes da sondagem, grava o aceite por URL exata e o backend impede o envio de credenciais se essa URL mudar ou não tiver sido autorizada. Conexões antigas sem aceite continuam visíveis, não recebem consentimento retroativo e ficam bloqueadas em chat, sumarização, health check, voz e sondagem até reautorização explícita; o aceite permanece válido ao renovar a credencial na mesma URL. Conexões legadas com URL inválida continuam editáveis e bloqueadas até a correção. Evidência: `frontend/src/components/settings/ProviderForm.tsx`, `internal/llmcatalog/validate.go`, `internal/llm/provider.go`, `internal/llm/registry.go`, `internal/providers/{service.go,health.go}` e `internal/speech/` com testes focados.
- [x] A fábrica backend consome perfis do catálogo e não mantém presets duplicados nos caminhos de criação cobertos. Evidência: `internal/providers/defaults.go`, `ServiceConfig.CatalogSnapshot` e `internal/providers/catalog_defaults_test.go`; perfis e combinações sem suporte são recusados, e a ausência de snapshot retorna erro sem criar configuração. A interface de configuração manual permanece disponível.
- [x] A fábrica backend resolve perfis do snapshot por IDs explícitos, valida a tupla perfil/família/formato, autenticação e origem completa (esquema/host/porta), e segue a precedência da UI: políticas `fixed`/`hidden`/`choice` usam seu default; `editable` mantém o top-level quando presente e usa o default quando ausente/vazio. Deriva autenticação do `auth.mode` efetivo (`bearer_api_key`/`api_key_header` obrigatório, `optional_api_key` opcional, `none` sem credencial), ignorando `auth.required` divergente. Snapshot vazio/offline não fabrica preset nem endpoint; o bootstrap legado da CLI retorna erro e a configuração manual continua disponível. Provedores hospedados e OpenRouter usam origens HTTPS fixas; LiteLLM só aceita `http://localhost:4000`, LocalAI e llama.cpp só aceitam `http://localhost:8080`, impedindo que o catálogo remoto escolha o destino da credencial. Evidência: `internal/providers/defaults.go`, `ServiceConfig.CatalogSnapshot` e `internal/providers/catalog_defaults_test.go`. Os perfis nativos Ollama e Cohere são recusados enquanto o runtime não representar seus formatos/rotas.
- [ ] Atualizar o catálogo não sobrescreve configurações existentes nem envia dados locais ao site.
- [x] Modelos e aliases só recebem capacidades por associação exata; IDs sem associação continuam selecionáveis com recursos desconhecidos. Evidência: `internal/llmcatalog/resolve_test.go` e `internal/providers/model_catalog_metadata_test.go` cobrem formato/perfil/ID exatos, overrides, ausência de inferência, atualização de limites e lista restrita aos modelos retornados pelo servidor.
- [x] Nenhuma conexão é identificada por inferência de hostname ou prefixo de modelo; configurações antigas ambíguas permanecem personalizadas. A resolução usa o `ServerProfileID` explícito e o snapshot do adaptador que fez a listagem; testes do catálogo e de persistência cobrem conexões sem vínculo.
- [ ] A fase de vozes do AEP-0113 consome o catálogo e distingue dados completos, parciais e desconhecidos, sem sondagem paga automática durante o uso normal.
- [ ] O AEP e o índice são atualizados em cada PR que entregar uma fase, com evidências verificáveis e status sincronizado.
