# Welcome Wizard - Assistente de Configuração Inicial

## Visão Geral

Sistema de wizard de boas-vindas que guia usuários novos através da configuração inicial do assistente, melhorando significativamente a experiência de primeiro uso.

## Funcionalidades Implementadas

### 1. Detecção Automática de Configuração

O sistema verifica automaticamente se o assistente está configurado ao iniciar:
- Verifica presença de API key e URL do servidor
- Se não configurado, inicia o wizard automaticamente
- Não redireciona mais para página de configurações (melhor UX)

### 2. Wizard de Configuração em 4 Etapas

#### Etapa 1: Seleção de Provedor
Oferece opções dos provedores mais comuns:
- OpenAI
- Anthropic (Claude)
- Google (Gemini)
- Azure OpenAI
- Ollama (Local)
- LiteLLM
- Outro (URL personalizada)

Cada opção vem com URL pré-configurada quando aplicável.

#### Etapa 2: URL Personalizada (Condicional)
Solicita URL apenas quando necessário:
- Quando usuário seleciona "Outro (URL personalizada)"
- Quando seleciona Azure OpenAI (requer URL específica da instância)

#### Etapa 3: Chave de API
Solicita chave de autenticação:
- Campo opcional (permite servidores sem autenticação)
- Mensagem contextual baseada no provedor escolhido
- Para Ollama, indica claramente que pode deixar em branco

#### Etapa 4: Seleção de Modelo
Lista modelos disponíveis e permite escolha:
- Consulta API do servidor para listar modelos reais
- Se falhar, permite entrada manual com sugestões
- Atualiza automaticamente TODOS os perfis com o modelo escolhido
- Define como modelo padrão na configuração global

### 3. Integração com Sistema de Questionários

Utiliza o sistema de questionários existente (`questionnaire.Manager`):
- Interface consistente com resto da aplicação
- Suporte a cancelamento em todas as etapas
- Validação de campos obrigatórios
- Feedback visual de progresso

### 4. Auto-Update Independente de Provedor

A verificação de atualização é uma responsabilidade da instância e não depende
de configuração de LLM:
- funciona mesmo com zero providers cadastrados;
- usa o scheduler único do updater, cancelável no shutdown;
- mantém o guard de desenvolvimento (`AppVersion == "dev"`).

### 5. Verificação de Updates Após Wizard

Após completar o wizard, o sistema sinaliza o scheduler único para antecipar a
primeira verificação. O mesmo fluxo atende startup, pós-wizard e periodicidade,
evitando fetches e prompts concorrentes. Se houver nova versão, a decisão usa
`Questionnaire KindDecision` conforme o AEP-0091.

## Arquivos Modificados

### Backend (Go)

1. **app.go**
   - `NeedsWelcomeWizard()` - Verifica se precisa do wizard
   - `RunWelcomeWizard()` - Executa o fluxo completo do wizard
   - `getWizardProviderInfo()` - Mapeia escolha do wizard para tipo/ID/nome do provedor
   - `createWizardProvider()` - Cria provedor no registry + credential manager + SQLite
   - `saveWelcomeConfig()` - Salva configuração legada (config.json)
   - `updateAllProfilesProviderAndModel()` - Atualiza provedor e modelo em todos os perfis
   - `checkForUpdatesOnStartup()` - Executa o scheduler cancelável do updater
   - `RequestUpdateCheck()` - Antecipa o check ao concluir o wizard

### Frontend (TypeScript/React)

1. **App.tsx**
   - Importa novas funções: `NeedsWelcomeWizard`, `RunWelcomeWizard`
   - Adiciona verificação no `useEffect` de carregamento
   - Executa wizard antes de carregar configuração se necessário
   - Feedback visual via toasts

## Fluxo de Execução

```
1. App.tsx inicia
   ↓
2. Verifica se Wails está pronto
   ↓
3. Chama NeedsWelcomeWizard()
   ↓
4. Se precisa do wizard:
   ├── Executa RunWelcomeWizard()
   ├── Usuário completa 4 etapas
   ├── Salva configuração
   ├── Atualiza perfis
   ├── Reinicializa LLM client
   └── Verifica atualizações disponíveis
   ↓
5. Carrega configuração normalmente
   ↓
6. Continua inicialização do app
```

## Benefícios

### Para Usuários Novos
- Processo guiado e intuitivo
- Não precisa navegar em configurações complexas
- Feedback claro em cada etapa
- Configuração validada em tempo real
- Oportunidade de atualizar logo após configuração

### Para Usuários Existentes
- Não afeta configurações existentes
- Zero impacto no fluxo normal
- Mantém compatibilidade com configurações antigas

### Para Manutenção
- Código bem documentado
- Usa sistemas existentes (questionnaire)
- Fácil adicionar novos provedores
- Tratamento robusto de erros

## Próximas Melhorias Possíveis

1. **Validação Avançada**
   - Testar conexão antes de salvar
   - Validar formato da API key por provedor

2. **Configurações Avançadas Opcionais**
   - Temperature, max tokens, etc.
   - Opção de "Configuração Avançada"

3. **Templates de Configuração**
   - Configurações pré-definidas populares
   - Import/export de configurações

4. **Tutorial Interativo**
   - Breve tour após configuração
   - Demonstração de funcionalidades principais

## Testes Recomendados

- [ ] Primeiro uso com OpenAI
- [ ] Primeiro uso com Ollama
- [ ] Primeiro uso com URL personalizada
- [ ] Cancelamento em diferentes etapas
- [ ] Erro na listagem de modelos
- [ ] Entrada manual de modelo
- [ ] Verificar se perfis foram atualizados
- [ ] Verificar se auto-update roda mesmo sem provider configurado

## Notas de Implementação

- Todos os questionários permitem cancelamento
- Configuração é salva atomicamente apenas no final
- Em caso de erro, não deixa sistema em estado inconsistente
- Logs detalhados para debug
- Mensagens de erro amigáveis para o usuário
