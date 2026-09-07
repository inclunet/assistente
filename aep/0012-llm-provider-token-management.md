# Gerenciamento de Tokens e Contexto

Este documento descreve o sistema de gerenciamento e monitoramento de tokens implementado no assistente.

## Visão Geral

O sistema permite:
- ✅ Rastrear consumo de tokens por mensagem, turno e conversa
- ✅ Visualizar estatísticas detalhadas de uso
- ✅ Receber alertas quando próximo do limite de contexto
- ✅ Gerenciar proativamente o contexto antes de atingir limites

## Dois conceitos distintos: ocupação da janela vs. billing acumulado

O sistema trabalha com **duas grandezas diferentes** que não devem ser confundidas:

- **`contextTokens` — ocupação ATUAL da janela de contexto.** É o `usage` (prompt + completion) reportado pelo provedor no **turno mais recente**, ou seja, quantos tokens a próxima requisição já carrega de contexto. É o único valor que deve ser comparado ao `contextLimit` e que alimenta `contextUsage` (%), `isNearLimit` e `isCritical`.
- **`totalTokens` — base de CUSTO/BILLING acumulado.** É a soma de todos os tokens (prompt + completion) ao longo de toda a conversa. Serve para estimar custo, **nunca** para medir a ocupação da janela.

> **Por que somar `total_tokens` por mensagem estava errado (bug da issue #197):** a contagem antiga somava o `total_tokens` de cada mensagem do assistente. Como o provedor recebe **todo o histórico reenviado a cada turno**, o `prompt_tokens` cresce a cada interação e o mesmo histórico era contado repetidamente. O resultado inflava o percentual de ocupação (podia passar de 100% facilmente) e disparava alertas falsos de `isNearLimit`/`isCritical`. A correção passa a usar o `usage` do **último turno** como ocupação real da janela (`contextTokens`), mantendo o acumulado apenas como base de billing (`totalTokens`).

## Armazenamento de Tokens

Cada mensagem (`ChatMessage`) armazena automaticamente:

```go
PromptTokens     int    // Tokens de entrada (prompt)
CompletionTokens int    // Tokens de saída (resposta)
TotalTokens      int    // Total de tokens
Model            string // Modelo usado
```

Esses valores são capturados automaticamente a partir da resposta da API do LLM.

## API Backend

### 1. Estatísticas por Conversa

```go
func (a *App) GetConversationTokenStats(conversationID uint) (*TokenStatsResult, error)
```

**Retorna:**
```json
{
  "promptTokens": 9500,       // Entrada ACUMULADA (inflada pelo histórico reenviado a cada turno)
  "completionTokens": 2950,   // Saída ACUMULADA
  "totalTokens": 12450,       // CUSTO/BILLING acumulado (= prompt+completion somados na conversa). NÃO usar para ocupação da janela
  "messageCount": 10,
  "model": "gpt-4",
  "contextTokens": 2300,      // OCUPAÇÃO ATUAL da janela: usage (prompt+completion) reportado pelo provedor no ÚLTIMO turno
  "contextUsage": 28.1,       // Porcentagem da janela (0-100) = contextTokens / contextLimit. Base de isNearLimit/isCritical
  "contextLimit": 8192,       // Limite do modelo
  "isNearLimit": false,       // true se contextUsage >= 80%
  "isCritical": false         // true se contextUsage >= 95%
}
```

> **Atenção:** o percentual de ocupação (`contextUsage`) deriva de `contextTokens`, **não** de `totalTokens`. Note que `totalTokens` (12450) pode ultrapassar o `contextLimit` (8192) sem que a janela esteja cheia — porque é o acumulado de billing, e não o que está carregado no contexto agora.

**Uso no Frontend:**
```typescript
const stats = await GetConversationTokenStats(conversationId);
console.log(`Ocupação atual: ${stats.contextUsage}% (${stats.contextTokens}/${stats.contextLimit})`);
console.log(`Custo acumulado (billing): ${stats.totalTokens} tokens`);
```

### 2. Estatísticas por Turno

```go
func (a *App) GetTurnTokenStats(conversationID uint, turnID uint) (*TokenStatsResult, error)
```

Útil para entender o custo de cada interação específica (pergunta + resposta + tool calls).

### 3. Tokens de Mensagens Recentes

```go
func (a *App) GetRecentMessagesTokenCount(conversationID uint, messageLimit int) (int, error)
```

Retorna o total de tokens das N mensagens mais recentes. Útil para estimar quanto contexto será enviado na próxima requisição.

**Exemplo:**
```typescript
// Estima tokens das últimas 20 mensagens
const recentTokens = await GetRecentMessagesTokenCount(conversationId, 20);
```

### 4. Verificação de Limite

```go
func (a *App) CheckContextWindowThreshold(conversationID uint, threshold float64) (bool, float64, error)
```

Verifica se a conversa ultrapassou um threshold específico (padrão: 80%).

**Exemplo:**
```typescript
// Verifica se está acima de 75%
const [isAbove, percentage] = await CheckContextWindowThreshold(conversationId, 75.0);
if (isAbove) {
  console.warn(`Contexto em ${percentage}%`);
}
```

## Funções de Database

### Agregação de Tokens

```go
// Total por conversa (simples)
database.GetConversationTokenStats(conversationID uint) (map[string]int, error)

// Total por conversa (detalhado com modelo e contagem)
database.GetConversationDetailedTokenStats(conversationID uint) (*TokenStats, error)

// Total por turno específico
database.GetTurnTokenStats(conversationID, turnID uint) (*TokenStats, error)

// Uso da janela de contexto
database.GetContextWindowUsage(conversationID uint, contextLimit int) (float64, int, error)
```

### Estrutura TokenStats

```go
type TokenStats struct {
    PromptTokens     int    `json:"prompt_tokens"`
    CompletionTokens int    `json:"completion_tokens"`
    TotalTokens      int    `json:"total_tokens"`
    MessageCount     int    `json:"message_count"`
    Model            string `json:"model,omitempty"`
}
```

## Sistema de Alertas

### Eventos Emitidos

O sistema emite eventos automaticamente após cada resposta do assistente:

#### 1. `chat:token_stats`

Enviado sempre após uma resposta, com estatísticas atualizadas:

O payload (`TokenStatsEvent`) carrega tanto a ocupação atual (`contextTokens`) quanto o acumulado de billing (`totalTokens`):

```typescript
EventsOn("chat:token_stats", (data) => {
  console.log(`Conversa ${data.conversationId}:`);
  console.log(`- Ocupação da janela: ${data.contextTokens}/${data.contextLimit} (${data.contextUsage}%)`);
  console.log(`- Custo acumulado (billing): ${data.totalTokens}`);
  console.log(`- Mensagens: ${data.messageCount}`);
});
```

#### 2. `chat:context_warning`

Enviado quando o contexto atinge níveis críticos:

O payload (`ContextWarningEvent`) usa `contextTokens` (ocupação atual da janela) no numerador — o mesmo valor que define `percentage`. Não há campo de billing acumulado neste evento.

**Warning Level (>= 80%):**
```json
{
  "conversationId": 123,
  "level": "warning",
  "message": "Contexto em 82.5% (6758/8192 tokens). Considere limpar a conversa em breve.",
  "percentage": 82.5,
  "contextTokens": 6758,
  "contextLimit": 8192
}
```

**Critical Level (>= 95%):**
```json
{
  "conversationId": 123,
  "level": "critical",
  "message": "Atenção: Contexto em 96.3% (7888/8192 tokens). Considere limpar a conversa ou resumir o histórico.",
  "percentage": 96.3,
  "contextTokens": 7888,
  "contextLimit": 8192
}
```

### Implementação no Frontend

```typescript
// Exibir alerta visual
EventsOn("chat:context_warning", (data) => {
  if (data.level === "critical") {
    showNotification({
      type: "error",
      title: "Contexto Crítico",
      message: data.message,
      actions: [
        { label: "Limpar Conversa", onClick: () => clearConversation(data.conversationId) },
        { label: "Resumir Histórico", onClick: () => summarizeHistory(data.conversationId) }
      ]
    });
  } else if (data.level === "warning") {
    showNotification({
      type: "warning",
      title: "Contexto Alto",
      message: data.message
    });
  }
});

// Mostrar indicador visual contínuo (ocupação da janela)
EventsOn("chat:token_stats", (data) => {
  updateContextIndicator({
    percentage: data.contextUsage,
    tokens: data.contextTokens, // ocupação atual da janela, não o billing acumulado
    limit: data.contextLimit,
    isNearLimit: data.isNearLimit,
    isCritical: data.isCritical
  });
});
```

## Estratégias de Gerenciamento

### 1. Interface do Usuário

**Botão de Tokens na Toolbar:**
- Exibe resumo compacto da ocupação atual: `contextTokens/limite` (ex: 4.2K/8K)
- Cores indicam status automático:
  - 🟢 Verde (< 80%): contexto normal
  - 🟡 Amarelo (≥ 80%): contexto alto
  - 🔴 Vermelho (≥ 95%): contexto crítico
- Ao clicar, abre modal com estatísticas detalhadas
- Não intrusivo - usuário consulta quando desejar

**Modal de Estatísticas (TokenStatsModal):**
- **Uso do Contexto:** barra de progresso visual + percentual
- **Detalhamento:** tokens entrada/saída, mensagens, modelo principal
- **Estimativa de Custo:** cálculo aproximado baseado em preços
- **Dicas de Gerenciamento:** sugestões contextuais
- **Atualização em Tempo Real:** via evento `chat:token_stats`

**Implementação:**
```tsx
// Na ChatToolbar
<TokenStatsButton
  conversationId={activeTab?.conversationId}
  onOpenModal={() => setIsTokenModalOpen(true)}
/>

<TokenStatsModal
  conversationId={activeTab.conversationId}
  isOpen={isTokenModalOpen}
  onClose={() => setIsTokenModalOpen(false)}
/>
```

### 2. Compactação Automática de Contexto

Implementar estratégias quando contexto ≥ 80%:

**A. Sumarização Incremental:**
```typescript
async function compactOldMessages(conversationId: number) {
  // 1. Buscar mensagens antigas (exceto as últimas 10-15)
  const oldMessages = await getOldMessages(conversationId, 15);
  
  if (oldMessages.length < 5) return; // Não vale a pena
  
  // 2. Criar resumo via LLM
  const summary = await createSummary(oldMessages);
  
  // 3. Substituir mensagens antigas por mensagem resumida
  await replaceWithSummary(conversationId, oldMessages, summary);
  
  // 4. Notificar usuário
  showNotification('Contexto compactado automaticamente');
}

async function createSummary(messages: Message[]): Promise<string> {
  const prompt = `Resuma a seguinte conversa preservando:
  - Tópicos principais discutidos
  - Decisões tomadas
  - Informações importantes
  - Tom e estilo da conversa
  
  Mensagens:
  ${messages.map(m => `${m.role}: ${m.content}`).join('\n\n')}
  
  Resumo conciso:`;
  
  return await callLLM(prompt);
}
```

**B. Remoção Seletiva:**
```typescript
// Remover mensagens intermediárias menos importantes
function selectMessagesToRemove(messages: Message[]): Message[] {
  const keep = new Set<number>();
  
  // Sempre manter primeira e última
  keep.add(0);
  keep.add(messages.length - 1);
  
  // Manter últimas N mensagens
  const recentCount = 10;
  for (let i = messages.length - recentCount; i < messages.length; i++) {
    keep.add(i);
  }
  
  // Remover tool calls repetitivos
  // Manter mensagens marcadas como importantes
  // Priorizar mensagens do usuário sobre do assistente
  
  return messages.filter((_, idx) => !keep.has(idx));
}
```

**C. Truncamento Inteligente:**
```typescript
// Truncar mensagens muito longas preservando início/fim
function truncateMessage(content: string, maxTokens: number): string {
  const tokens = estimateTokens(content);
  if (tokens <= maxTokens) return content;
  
  const keepRatio = maxTokens / tokens;
  const keepChars = Math.floor(content.length * keepRatio);
  const halfKeep = Math.floor(keepChars / 2);
  
  return (
    content.slice(0, halfKeep) + 
    "\n\n[... conteúdo truncado ...]\n\n" + 
    content.slice(-halfKeep)
  );
}
```

### 3. Políticas de Limpeza

**A. Manual (via Modal):**
- Botão "Compactar Contexto" exibido quando ≥ 80%
- Opções: Resumir / Remover antigas / Truncar longas
- Preview das mudanças antes de aplicar
- Confirmar antes de executar

**B. Automática (Configurável por Perfil):**
```typescript
interface ContextPolicy {
  enabled: boolean;
  triggerPercentage: number; // Ex: 85%
  keepRecentMessages: number; // Ex: 15
  strategy: 'summarize' | 'truncate' | 'remove';
  autoBackup: boolean;
}

// Exemplo de uso
const policy: ContextPolicy = {
  enabled: true,
  triggerPercentage: 85,
  keepRecentMessages: 15,
  strategy: 'summarize',
  autoBackup: true
};

// Hook que monitora e executa
EventsOn('chat:token_stats', async (data) => {
  if (data.contextUsage >= policy.triggerPercentage) {
    if (policy.autoBackup) {
      await backupConversation(data.conversationId);
    }
    await executeCompaction(data.conversationId, policy);
  }
});
```

**C. Por Perfil:**
- Perfis podem definir `contextManagement` customizado
- Alguns podem desabilitar compactação (ex: perfil "Memória Total")
- Outros podem forçar agressiva (ex: perfil "Modo Econômico")

### 4. Estimativa Preventiva

Antes de enviar nova mensagem:

```typescript
async function validateBeforeSend(
  conversationId: number, 
  newMessage: string
): Promise<ValidationResult> {
  // Estimar tokens da nova mensagem
  const estimatedTokens = estimateTokens(newMessage);
  
  // Pegar ocupação atual da janela (não o billing acumulado)
  const stats = await GetConversationTokenStats(conversationId);
  const projectedTotal = stats.contextTokens + estimatedTokens;
  
  // Verificar contra limite
  if (projectedTotal > stats.contextLimit) {
    return {
      canSend: false,
      reason: 'Contexto excederia o limite',
      suggestion: 'Compacte o histórico antes de enviar',
      actions: ['compact', 'clear', 'cancel']
    };
  }
  
  if (projectedTotal > stats.contextLimit * 0.9) {
    return {
      canSend: true,
      warning: 'Contexto ficará próximo do limite após envio',
      suggestion: 'Considere compactar após esta mensagem'
    };
  }
  
  return { canSend: true };
}
```

### 5. Backup Antes de Compactação

```typescript
async function compactWithBackup(conversationId: number) {
  try {
    // 1. Criar backup exportável
    const backup = await ExportConversation(conversationId);
    
    // 2. Salvar localmente com timestamp
    const filename = `backup_${conversationId}_${Date.now()}.json`;
    await saveBackup(filename, backup);
    
    // 3. Executar compactação
    const result = await compactOldMessages(conversationId);
    
    // 4. Notificar usuário com estatísticas
    showNotification({
      title: 'Contexto Compactado',
      message: `Reduzido de ${result.before} para ${result.after} tokens`,
      action: { label: 'Ver Backup', onClick: () => openBackup(filename) }
    });
  } catch (error) {
    console.error('Erro ao compactar:', error);
    showNotification({
      type: 'error',
      title: 'Falha na Compactação',
      message: 'Nenhuma alteração foi feita'
    });
  }
}
```

### 6. Visualização de Histórico Compactado

```tsx
// Indicar mensagens resumidas
<ChatMessage 
  message={msg}
  isCompacted={msg.metadata?.compacted}
  originalCount={msg.metadata?.originalMessageCount}
  onExpandOriginal={() => viewBackup(msg.metadata?.backupId)}
/>

// Badge especial para mensagens compactadas
{isCompacted && (
  <span className="message-compacted-badge" title="Mensagem resumida">
    🗜️ Resumo de {originalCount} mensagens
  </span>
)}
```

### 1. Limpeza Manual

```typescript
// Limpar todas as mensagens (mantém conversa)
await ClearConversation(conversationId);
```

### 2. Resumo de Histórico

Implementar função que:
1. Pega mensagens antigas
2. Envia ao LLM pedindo resumo
3. Substitui mensagens antigas por um resumo
4. Mantém mensagens recentes intactas

```typescript
async function summarizeHistory(conversationId: number) {
  const stats = await GetConversationTokenStats(conversationId);
  
  if (stats.messageCount > 20) {
    // Pegar mensagens antigas (exceto últimas 10)
    const oldMessages = await GetOldMessages(conversationId, stats.messageCount - 10);
    
    // Criar resumo via LLM
    const summary = await CreateSummary(oldMessages);
    
    // Arquivar mensagens antigas e inserir resumo
    await ArchiveAndSummarize(conversationId, oldMessages, summary);
  }
}
```

### 3. Paginação Inteligente

Implementar carregamento de mensagens por demanda:
- Carregar apenas últimas N mensagens
- Carregar mais sob demanda (scroll infinito)
- Enviar ao LLM apenas contexto relevante

### 4. Configuração de Limites

Configurar `contextWindow` no perfil:

```json
{
  "chat": {
    "model": "gpt-4-turbo",
    "contextWindow": 128000,
    "temperature": 0.7
  }
}
```

## Limites Comuns de Modelos

| Modelo | Limite de Tokens |
|--------|-----------------|
| gpt-3.5-turbo | 16,385 |
| gpt-4 | 8,192 |
| gpt-4-32k | 32,768 |
| gpt-4-turbo | 128,000 |
| claude-3-opus | 200,000 |
| claude-3-sonnet | 200,000 |
| claude-3-haiku | 200,000 |
| gemini-1.5-pro | 2,000,000 |
| llama-3-70b | 8,192 |
| mixtral-8x7b | 32,768 |

## Cálculo de Custos

Combine estatísticas de tokens com preços da API para calcular custos:

```typescript
const PRICING = {
  "gpt-4": {
    prompt: 0.03 / 1000,      // $0.03 per 1K tokens
    completion: 0.06 / 1000    // $0.06 per 1K tokens
  },
  "gpt-3.5-turbo": {
    prompt: 0.0015 / 1000,
    completion: 0.002 / 1000
  }
};

function calculateCost(stats: TokenStatsResult): number {
  const pricing = PRICING[stats.model];
  if (!pricing) return 0;
  
  const promptCost = stats.promptTokens * pricing.prompt;
  const completionCost = stats.completionTokens * pricing.completion;
  
  return promptCost + completionCost;
}
```

## Monitoramento e Logs

Os logs do console mostram informações de contexto:

```
⚠️  [CONTEXT] Conversa 123 próxima do limite: 82.3% (6738/8192 tokens)
⚠️  [CONTEXT] Conversa 456 em nível CRÍTICO: 96.5% (7900/8192 tokens)
```

## Boas Práticas

1. **Configure limites corretos**: Sempre configure `contextWindow` no perfil com o limite real do modelo
2. **Monitore proativamente**: Use os eventos para mostrar indicadores visuais constantes
3. **Aja nos alertas**: Implemente ações automáticas ou manuais quando alertas forem emitidos
4. **Otimize contexto**: Envie apenas mensagens relevantes ao LLM (não precisa enviar todo histórico)
5. **Arquive conversas longas**: Para conversas muito longas, considere arquivar e começar nova
6. **Use resumos**: Para contexto histórico importante, use resumos em vez de mensagens completas

## Exemplo Completo

```typescript
// Inicialização
EventsOn("chat:token_stats", updateTokenDisplay);
EventsOn("chat:context_warning", handleContextWarning);

// Antes de enviar mensagem
async function beforeSendMessage(conversationId: number) {
  const stats = await GetConversationTokenStats(conversationId);
  
  if (stats.isCritical) {
    const confirm = await showConfirmDialog({
      title: "Contexto Crítico",
      message: `Você está usando ${stats.contextUsage}% do contexto. Deseja continuar ou limpar a conversa?`,
      options: ["Continuar", "Limpar", "Resumir"]
    });
    
    if (confirm === "Limpar") {
      await ClearConversation(conversationId);
    } else if (confirm === "Resumir") {
      await summarizeHistory(conversationId);
    }
  }
}

// Exibição de estatísticas
function updateTokenDisplay(data: any) {
  const percentage = data.contextUsage;
  const color = data.isCritical ? "red" : data.isNearLimit ? "orange" : "green";
  
  document.getElementById("token-bar").style.width = `${percentage}%`;
  document.getElementById("token-bar").style.backgroundColor = color;
  document.getElementById("token-text").textContent = 
    `${data.contextTokens}/${data.contextLimit} tokens (${percentage.toFixed(1)}%)`;
}

// Gerenciamento de alertas
function handleContextWarning(data: any) {
  if (data.level === "critical") {
    showBanner({
      type: "error",
      message: data.message,
      persistent: true
    });
  } else {
    showToast({
      type: "warning",
      message: data.message
    });
  }
}
```

## Implementação Frontend

### Componentes Criados

#### 1. TokenStatsButton

Botão compacto na toolbar que exibe resumo de tokens:

**Localização:** `frontend/src/components/chat/TokenStatsButton.tsx`

**Características:**
- Exibe a ocupação atual `contextTokens/limite` formatado (ex: 4.2K/8K)
- Ícone indica status: 📊 (normal), 🟡 (warning), 🔴 (critical)
- Atualização automática via evento `chat:token_stats`
- Aparece apenas quando há conversationId ativo
- Ao clicar, abre modal de estatísticas

**Props:**
```typescript
interface TokenStatsButtonProps {
  conversationId?: number;
  onOpenModal: () => void;
}
```

**Uso:**
```tsx
<TokenStatsButton
  conversationId={activeTab?.conversationId}
  onOpenModal={() => setIsTokenModalOpen(true)}
/>
```

#### 2. TokenStatsModal

Modal completo com estatísticas detalhadas:

**Localização:** `frontend/src/components/chat/TokenStatsModal.tsx`

**Seções:**
1. **Uso do Contexto:**
   - Números grandes: usado/limite
   - Barra de progresso colorida
   - Percentual de uso
   - Alertas contextuais

2. **Detalhamento:**
   - Cards com tokens de entrada/saída
   - Contagem de mensagens
   - Modelo principal usado
   - Médias (tokens/mensagem)

3. **Estimativa de Custo:**
   - Custo de entrada/saída
   - Total estimado
   - Nota sobre preços aproximados

4. **Dicas de Gerenciamento:**
   - Lista de sugestões contextuais
   - Ações recomendadas

**Props:**
```typescript
interface TokenStatsModalProps {
  conversationId: number;
  isOpen: boolean;
  onClose: () => void;
}
```

**Uso:**
```tsx
<TokenStatsModal
  conversationId={conversationId}
  isOpen={isTokenModalOpen}
  onClose={() => setIsTokenModalOpen(false)}
/>
```

### Integração na ChatToolbar

**Arquivo:** `frontend/src/components/chat/ChatToolbar.tsx`

```tsx
import { TokenStatsButton } from './TokenStatsButton';
import { TokenStatsModal } from './TokenStatsModal';

export const ChatToolbar: React.FC<ChatToolbarProps> = ({...}) => {
  const [isTokenModalOpen, setIsTokenModalOpen] = useState(false);
  const activeTab = getActiveTab();
  
  return (
    <>
      <Toolbar
        right={
          <>
            {/* ... outros botões ... */}
            
            <TokenStatsButton
              conversationId={activeTab?.conversationId}
              onOpenModal={() => setIsTokenModalOpen(true)}
            />
            
            {/* ... */}
          </>
        }
      />
      
      {activeTab?.conversationId && (
        <TokenStatsModal
          conversationId={activeTab.conversationId}
          isOpen={isTokenModalOpen}
          onClose={() => setIsTokenModalOpen(false)}
        />
      )}
    </>
  );
};
```

### Fluxo de Dados

```
Backend (llm.go)
    ↓ OnDone após resposta
checkAndEmitContextWarning()
    ↓ GetConversationTokenStats()
    ↓ runtime.EventsEmit("chat:token_stats", stats)
    ↓
Frontend (TokenStatsButton)
    ↓ EventsOn("chat:token_stats")
    ↓ Atualiza estado local
    ↓ Re-renderiza botão com novos dados
    ↓
Usuário clica no botão
    ↓ onOpenModal()
    ↓
TokenStatsModal abre
    ↓ GetConversationTokenStats() (carga inicial)
    ↓ EventsOn("chat:token_stats") (atualizações)
    ↓ Exibe dados completos
```

### Customização Visual

**Variáveis CSS (defina em seu tema):**
```css
:root {
  /* Cores de status */
  --warning-border: #f59e0b;
  --warning-background: rgba(245, 158, 11, 0.1);
  --warning-background-hover: rgba(245, 158, 11, 0.2);
  
  --error-border: #ef4444;
  --error-background: rgba(239, 68, 68, 0.1);
  --error-background-hover: rgba(239, 68, 68, 0.2);
  
  /* Background e borders padrão */
  --background-primary: #ffffff;
  --background-secondary: #f3f4f6;
  --background-tertiary: #e5e7eb;
  --border-color: #d1d5db;
  --border-hover: #9ca3af;
  
  /* Texto */
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-tertiary: #9ca3af;
  
  /* Focus ring */
  --focus-ring: #3b82f6;
}
```

### Acessibilidade

**Implementado:**
- ✅ ARIA labels descritivos
- ✅ Role="dialog" e aria-modal no modal
- ✅ Escape fecha o modal
- ✅ Role="progressbar" na barra de progresso
- ✅ Focus trap no modal
- ✅ Navegação via teclado

**Exemplos:**
```tsx
// Botão com label descritivo (ocupação atual da janela)
<button
  aria-label={`Estatísticas de tokens: ${formatNumber(stats.contextTokens)} de ${formatNumber(stats.contextLimit)} ocupados`}
>

// Modal com roles corretos
<div
  role="dialog"
  aria-modal="true"
  aria-labelledby="token-stats-title"
>

// Barra de progresso acessível
<div
  role="progressbar"
  aria-valuenow={stats.contextUsage}
  aria-valuemin={0}
  aria-valuemax={100}
/>
```

## Próximos Passos

### Melhorias Prioritárias

- [ ] **Compactação Automática:**
  - Implementar função de resumo via LLM
  - Adicionar backup automático antes de compactar
  - Criar UI para gerenciar backups
  
- [ ] **Políticas por Perfil:**
  - Adicionar `contextManagement` nos perfis
  - Permitir configuração de threshold e estratégia
  - Implementar modo "Memória Total" (sem compactação)
  
- [ ] **Estimativa Preventiva:**
  - Validar contexto antes de enviar mensagem
  - Sugerir compactação se próximo do limite
  - Bloquear envio se exceder limite
  
- [ ] **Dashboard de Custos:**
  - Visualizar custos por conversa
  - Gráfico de uso ao longo do tempo
  - Comparação entre modelos
  
- [ ] **Visualização Avançada:**
  - Indicar mensagens compactadas
  - Permitir expandir mensagens resumidas
  - Mostrar estatísticas antes/depois
  
- [ ] **Melhorias no Modal:**
  - Adicionar botão "Compactar Agora"
  - Preview das mudanças antes de aplicar
  - Histórico de compactações realizadas

### Recursos Futuros

- [ ] Análise de padrões de uso
- [ ] Recomendações de modelo baseadas em uso
- [ ] Exportação de relatórios de tokens
- [ ] Integração com billing APIs dos provedores
- [ ] Alertas configuráveis por email/notificação

## Histórico de Revisões

| Versão | Mudança |
|--------|---------|
| 1.0 | Versão inicial: rastreio de tokens, estatísticas, alertas de contexto e estratégias de gerenciamento. |
| 1.1 | **Distinção explícita entre ocupação da janela e billing acumulado (issue #197).** `contextTokens`/`contextUsage` passam a refletir a OCUPAÇÃO ATUAL da janela (usage do último turno reportado pelo provedor) e são a base de `isNearLimit`/`isCritical`; `totalTokens` permanece como CUSTO/BILLING acumulado e deixa de ser usado para o percentual de ocupação. O evento `chat:context_warning` passou a carregar `contextTokens` (antes `totalTokens`). Exemplos numéricos corrigidos para que a ocupação fique consistente com `contextUsage` e abaixo do `contextLimit`. |
