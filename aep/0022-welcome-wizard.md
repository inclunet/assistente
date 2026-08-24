# AEP-0022 — Welcome Wizard

**Status:** Done

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

### 4. Proteção do Auto-Update

Auto-update agora só funciona com LLM configurado:
- Evita verificar atualizações em sistema não configurado
- Previne uso desnecessário de recursos de rede
- Melhor experiência para usuários novos

### 5. Verificação de Updates Após Wizard

Após completar o wizard, o sistema automaticamente:
- Aguarda 2 segundos para finalizar configuração
- Verifica se há atualizações disponíveis
- Oferece ao usuário a oportunidade de atualizar se houver nova versão
- Usa o mesmo fluxo de questionário para confirmação

## Arquivos Modificados

### Backend (Go)

- **`internal/wailsapi/welcome.go`** — binding Wails `Welcome`, avaliação
  pré/pós-login e delegação de `NeedsWelcomeWizard`/`RunWelcomeWizard`.
- **`controllers/welcome_controller.go`** — fluxo do wizard, validação de
  conexão/URL, criação do provider e verificação de update.
- **`internal/app/app_welcome.go`** — wiring de runtime, compatibilidade da CLI
  e thin wrappers usados pelos testes. O arquivo citado em revisões antigas
  como `internal/app/app_wizard.go` não existe no HEAD; o nome vigente é
  `app_welcome.go`.
- **`internal/app/app_wire.go`** — construção e conexão do controller/binding.

### Frontend (TypeScript/React)

- **`frontend/src/App.tsx`** — consumidor enxuto do binding
  `@wailsjs/go/wailsapi/Welcome`; inicia o wizard quando solicitado pelo
  backend. A lógica de domínio não reside no componente.
- **Questionnaire global** — renderiza os passos tipados emitidos pelo
  controller, com strings traduzíveis.

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

## Cobertura verificada

- [x] Providers, formatos de API, URL customizada, persistência SQLite e
  provider default: `internal/app/app_wizard_test.go`.
- [x] URLs inválidas, autenticação, indisponibilidade, erros HTTP e listagem de
  modelos: `internal/app/app_wizard_test.go`.
- [x] Binding não conectado, avaliação pré/pós-login e delegação ao runtime:
  `internal/wailsapi/welcome_test.go`.
- [x] Payloads e mensagens traduzíveis do questionário:
  `controllers/welcome_dialogs_i18n_test.go`.
- [x] Chaves de questionário disponíveis nos locales:
  `frontend/src/locales/agentQuestionnaireKeys.test.ts`.

## Notas de Implementação

- Todos os questionários permitem cancelamento
- Configuração é salva atomicamente apenas no final
- Em caso de erro, não deixa sistema em estado inconsistente
- Logs detalhados para debug
- Mensagens de erro amigáveis para o usuário
