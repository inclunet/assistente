# Sistema de Anúncios para Leitores de Tela (Screen Reader Announcer)

## Visão Geral

O sistema de announcer fornece uma maneira centralizada e controlada de anunciar informações para usuários de leitores de tela, sem poluir a interface com múltiplas regiões `aria-live`.

## Arquitetura

### Componentes

1. **ScreenReaderAnnouncer** (`components/ui/ScreenReaderAnnouncer.tsx`)
   - Componente único montado no nível do App
   - Contém duas regiões aria-live ocultas visualmente:
     - `polite`: Para anúncios que não devem interromper (padrão)
     - `assertive`: Para anúncios urgentes que devem interromper imediatamente

2. **useAnnouncer** (`hooks/useAnnouncer.ts`)
   - Hook React para enviar anúncios de dentro de componentes
   - Função global `announce()` para uso fora de componentes

### Vantagens

✅ **Centralização**: Uma única região aria-live em vez de múltiplas espalhadas
✅ **Controle**: Decide-se explicitamente o que deve ser anunciado
✅ **Performance**: Reduz re-renderizações e anúncios duplicados
✅ **Manutenibilidade**: Fácil de gerenciar e debugar
✅ **Acessibilidade**: Evita sobrecarga de informações para leitores de tela

## Como Usar

### 1. Dentro de Componentes React

```tsx
import { useAnnouncer } from '../hooks/useAnnouncer';

function MyComponent() {
  const { announce } = useAnnouncer();

  const handleAction = () => {
    // Faz algo...
    
    // Anuncia para leitores de tela
    announce('Ação concluída com sucesso');
    
    // Ou com prioridade assertive (interrompe imediatamente)
    announce('Erro crítico!', 'assertive');
  };

  return <button onClick={handleAction}>Executar</button>;
}
```

### 2. Fora de Componentes React

```typescript
import { announce } from '../hooks/useAnnouncer';

// Em qualquer arquivo TypeScript/JavaScript
function someFunction() {
  // Faz algo...
  
  // Anuncia para leitores de tela
  announce('Operação concluída');
}
```

## Exemplos de Uso

### Navegação entre Guias

```tsx
// Em useTabsKeyboardShortcuts.ts
const { announce } = useAnnouncer();

setActiveTab(nextTab.id);
announce(`Guia ${tabNumber}: ${tabTitle}`);
```

### Ações do Usuário

```tsx
// Ao criar nova guia
createTab();
announce('Nova guia criada');

// Ao fechar guia
closeTab(activeTabId);
announce('Guia fechada');
```

### Status de Operações

```tsx
// Ao salvar
saveData();
announce('Dados salvos com sucesso');

// Ao carregar
loadData();
announce('Carregando dados...', 'polite');
```

### Erros e Avisos

```tsx
// Erro crítico
announce('Erro ao salvar. Tente novamente.', 'assertive');

// Aviso
announce('Alguns campos não foram preenchidos', 'polite');
```

## Quando Usar vs. aria-live Direto

### Use o Announcer ✅

- Anúncios de mudanças de navegação
- Feedback de ações do usuário
- Status de operações assíncronas
- Mensagens de sucesso/erro
- Mudanças de contexto

### Use aria-live Direto ❌ (evite)

- Raramente necessário com o announcer
- Apenas para regiões específicas que DEVEM ser monitoradas continuamente
- Exemplo: Relógio/timer atualizado em tempo real

## Prioridades

### `polite` (padrão)

- Espera o leitor terminar o que está lendo
- Para anúncios informativos
- Não urgente
- **Exemplos**: "Guia 2: Nova conversa", "Dados salvos"

### `assertive`

- Interrompe o leitor imediatamente
- Para alertas urgentes
- Deve ser usado com moderação
- **Exemplos**: "Erro crítico", "Conexão perdida"

## Integração no App

O componente deve estar montado no nível do App:

```tsx
// App.tsx
import { ScreenReaderAnnouncer } from './components/ui/ScreenReaderAnnouncer';

function App() {
  return (
    <>
      <ScreenReaderAnnouncer />
      <Outlet />
      <ToastContainer />
    </>
  );
}
```

## Debugging

Para testar anúncios, use um leitor de tela como:
- **Windows**: NVDA (grátis) ou JAWS
- **macOS**: VoiceOver (built-in, Cmd+F5)
- **Linux**: Orca

Ou habilite o console do browser e adicione logs temporários:

```tsx
const { announce } = useAnnouncer();

const handleAction = () => {
  const message = 'Ação concluída';
  announce(message);
};
```

## Boas Práticas

### ✅ Faça

- Seja conciso e claro
- Use linguagem natural
- Anuncie resultados de ações
- Use 'polite' como padrão
- Teste com leitores de tela reais

### ❌ Evite

- Anúncios muito frequentes (< 1 segundo entre eles)
- Mensagens muito longas
- Anúncios redundantes (informação já visível)
- Usar 'assertive' para informações não-críticas
- Anunciar cada mudança visual mínima

## Implementação Técnica

### Como Funciona

1. **ScreenReaderAnnouncer** registra uma função global ao montar
2. Hook/função `announce()` chama essa função registrada
3. A função atualiza o state (`politeMessage` ou `assertiveMessage`)
4. React re-renderiza a região aria-live com o novo conteúdo
5. Leitor de tela detecta a mudança e anuncia
6. Após 1 segundo, a mensagem é limpa automaticamente

### Limitações

- Mensagens muito rápidas (< 1s) podem sobrescrever anteriores
- Apenas uma mensagem `polite` e uma `assertive` por vez
- Solução: adicionar fila se necessário no futuro

## Migração de aria-live Existentes

Para migrar código que usa `aria-live` direto:

### Antes ❌

```tsx
<div aria-live="polite">
  {status}
</div>
```

### Depois ✅

```tsx
const { announce } = useAnnouncer();

useEffect(() => {
  if (status) {
    announce(status);
  }
}, [status, announce]);
```

## Referências

- [WAI-ARIA Live Regions](https://www.w3.org/TR/wai-aria-1.2/#live_region_roles)
- [WebAIM: Invisible Content](https://webaim.org/techniques/css/invisiblecontent/)
- [MDN: ARIA live regions](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/ARIA_Live_Regions)
