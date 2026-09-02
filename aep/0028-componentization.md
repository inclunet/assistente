# Plano de Excelência em Componentisação Frontend

**Status:** In Progress — componentização avançou, mas o plano e seus critérios amplos não estão encerrados

**Data**: 5 de março de 2026  
**Objetivo**: Eliminar duplicação de código, aumentar reusabilidade, melhorar testabilidade e reduzir risk de erros

---

## 📊 Análise Executiva

### Situação Atual

O codebase frontend possui:
- **13+ páginas** (ProfilesPage, SkillsPage, McpPage, ChannelsPage, AllowlistPage, etc.)
- **8 categorias de componentes** (chat, editor, terminal, pickers, modals, ui, tabs, layout)
- **Padrão CRUD** repetido em ~5 páginas com código praticamente idêntico
- **~650 linhas no DataGrid** + estado de grid manual em cada página
- **Componentes hermétticos** com lógica acoplada ao contexto específico
- **Lógica de editor** duplicada em múltiplas páginas

### Oportunidades Identificadas

#### 🔴 **Críticas (Alto Impacto, Baixo Risco)**

1. **Padrão CRUD Genérico** (~800 linhas potencial duplicado)
   - ProfilesPage, SkillsPage, McpPage, AllowlistPage, ChannelsPage
   - Compartilham: load, search, grid, edit modal, save, delete
   - Impacto: -40% linhas de código em 5 páginas
   - Complexidade: 🟢 Média (estado bem definido)

2. **Hooks de Interação** (~200 linhas duplicado)
   - useGridFocus, useCheckboxListNav, useAnnouncer
   - Padrão de keyboard nav repetido
   - Impacto: -30% linhas em 8+ componentes
   - Complexidade: 🟢 Baixa (abstrações limpas)

3. **Componentes Picker** (~400 linhas duplicado)
   - ProfilePicker, ModelPicker, STTProviderPicker, VoicePicker
   - Todos seguem padrão Combobox similar
   - Impacto: -50% linhas em 4 componentes
   - Complexidade: 🟡 Média (lógica de seleção variada)

#### 🟠 **Altas (Impacto Médio, Risco Médio)**

4. **Chat Message Components** (~400 linhas duplicado potencial)
   - ChatMessage, MessageNode compartilham renderização
   - ReasoningSection, ToolCallsSection repetidos em múltiplos contextos
   - Impacto: -35% linhas em chat
   - Complexidade: 🟠 Alta (threading, streaming, audio)

5. **Diálogos e Modais** (~300 linhas duplicado)
   - QuestionnaireDialog, TokenStatsModal, MermaidEditorModal
   - Padrão modal genérico não centralizado
   - Impacto: -40% linhas em 6+ modais
   - Complexidade: 🟡 Média (UI patterns consistentes)

6. **Editor Panels** (~200 linhas duplicado)
   - EditorPanel, EditorPanelFields, EditorPanelFooter
   - Repetição em ProfilesPage, SkillsPage, McpPage, AllowlistPage
   - Impacto: -25% linhas de layout em telas de edição
   - Complexidade: 🟢 Baixa (apenas layout + slots)

#### 🟡 **Médias (Impacto Baixo, Risco Baixo)**

7. **Toolbar Patterns** (~150 linhas duplicado)
   - ChatToolbar, EditorTabs toolbar, page toolbars
   - Compartilham padrões de ação
   - Impacto: -20% linhas em componentes de toolbar
   - Complexidade: 🟢 Baixa

8. **Terminal Components** (~250 linhas duplicado)
   - TerminalHistory, TerminalEntry compartilham renderização
   - Impacto: -30% linhas em terminal
   - Complexidade: 🟡 Média (virtual scrolling)

9. **Input Validators & Formatters** (~150 linhas duplicado)
   - Validação de nome, description, slug em múltiplas páginas
   - Formatação de arrays, strings
   - Impacto: -35% linhas em formulários
   - Complexidade: 🟢 Baixa (utilitários simples)

---

## 🎯 Estratégia em Fases

### Filosofia de Fases

Cada fase:
1. **Reduz risco** através de escopo focado
2. **Maximiza impacto** priorizando padrões mais repetidos
3. **Mantém estabilidade** com testes em cada etapa
4. **Prova ROI** com métricas tangíveis
5. **Documenta aprendizados** para fases subsequentes

---

## 📋 FASE 1: Fundação de Componentes de Input & Utilitários (1-2 semanas)

### Objetivo
Criar abstrações reutilizáveis para inputs, validação e formatação. Estabelecer padrões de testes.

### Tarefas

#### 1.1 - Criar biblioteca de Hooks de Formulário
**Arquivo novo**: `src/hooks/useFormField.ts`
**O que faz**: Unifica validação, estado e handleChange

```typescript
// useFormField.ts
export interface FormFieldOptions<T> {
  validate?: (value: T) => string | null;
  format?: (value: T) => string;
  parse?: (value: string) => T;
}

export function useFormField<T>(
  initialValue: T,
  options?: FormFieldOptions<T>
) {
  const [value, setValue] = useState(initialValue);
  const [error, setError] = useState<string | null>(null);

  const handleChange = useCallback((newValue: string | T) => {
    const parsed = typeof newValue === 'string' ? options?.parse?.(newValue) ?? newValue : newValue;
    setValue(parsed);
    if (options?.validate) {
      setError(options.validate(parsed as T));
    }
  }, [options]);

  return { value, setValue, error, handleChange, isValid: !error };
}
```

**Impacto**: Elimina ~50 linhas por página de formulário  
**Testes**: `useFormField.test.ts` com 8+ cenários de validação

---

#### 1.2 - Criar utilitários de Formatação & Validação
**Arquivo novo**: `src/lib/formValidation.ts`
**O que faz**: Consolidar validadores comuns (slug, name, description, etc.)

```typescript
// formValidation.ts
export const validators = {
  slug: (val: string): string | null => {
    if (!val.trim()) return 'Slug é obrigatório';
    if (!/^[a-z0-9_-]+$/.test(val)) return 'Slug só pode ter letras, números, - e _';
    return null;
  },
  name: (val: string): string | null => {
    if (!val.trim()) return 'Nome é obrigatório';
    if (val.length < 2) return 'Nome deve ter mínimo 2 caracteres';
    if (val.length > 100) return 'Nome deve ter máximo 100 caracteres';
    return null;
  },
  description: (val: string): string | null => {
    if (val.length > 500) return 'Descrição deve ter máximo 500 caracteres';
    return null;
  },
  // ... mais validadores
};
```

**Impacto**: Elimina ~40 linhas por página de validação  
**Testes**: `formValidation.test.ts` com 30+ casos

---

#### 1.3 - Criar compostos de Input com Suporte a Erro
**Arquivo novo**: `src/components/ui/FormField.tsx`
**O que faz**: Input + Label + Error + assistência em um componente

```typescript
// FormField.tsx
export interface FormFieldProps extends InputProps {
  label?: string;
  error?: string | null;
  helpText?: string;
  required?: boolean;
}

export const FormField = forwardRef<HTMLInputElement, FormFieldProps>(
  ({ label, error, helpText, required, ...props }, ref) => {
    return (
      <div className="form-field">
        {label && (
          <label>
            {label}
            {required && <span className="required">*</span>}
          </label>
        )}
        <Input ref={ref} {...props} aria-invalid={!!error} />
        {error && <span className="error">{error}</span>}
        {helpText && <span className="help">{helpText}</span>}
      </div>
    );
  }
);
```

**Impacto**: Elimina ~80 linhas por página de markup de input  
**Testes**: `FormField.test.ts` com 12+ variações

---

#### 1.4 - Refatorar Componentes Picker para Base Genérica
**Arquivo novo**: `src/components/pickers/BasePicker.tsx`
**Modificar**: ProfilePicker, ModelPicker, STTProviderPicker, VoicePicker

```typescript
// BasePicker.tsx
export interface BasePickerProps<T> {
  items: T[];
  selectedValue?: string;
  getLabel: (item: T) => string;
  getValue: (item: T) => string;
  getSubtitle?: (item: T) => string;
  onSelect?: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  loading?: boolean;
  error?: string;
}

export const BasePicker = forwardRef<HTMLDivElement, BasePickerProps<any>>(
  // Implementação genérica com Combobox internamente
);
```

**Impacto**: Reduz cada Picker de ~80 linhas para ~30 linhas  
**Testes**: `BasePicker.test.ts` com 15+ casos

---

#### 1.5 - Criar Utilitários de Confirmação Genéricos
**Arquivo novo**: `src/lib/confirmPatterns.ts`
**O que faz**: Templates de confirmação reutilizáveis

```typescript
// confirmPatterns.ts
export function getDeleteConfirmMessage(itemType: string, itemName: string): string {
  return `Tem certeza que deseja excluir o ${itemType} "${itemName}"?`;
}

export function getUnsavedChangesMessage(): string {
  return 'Você tem alterações não salvas. Deseja sair sem salvar?';
}
```

**Impacto**: Elimina ~30 linhas por página de tradução/strings  
**Testes**: `confirmPatterns.test.ts` com 8+ mensagens

---

### Métrica de Sucesso - Fase 1
- ✅ Novo projeto que usa 2+ páginas existentes com novos componentes
- ✅ 40+ testes criados (jest + vitest)
- ✅ 0 regressões em funcionalidade existente
- ✅ ~500 linhas de código removidas de duplicação
- ✅ Documentação de padrões em `docs/COMPONENT_PATTERNS.md`

---

## 🎯 FASE 2: Abstração de Padrão CRUD Genérico (2-3 semanas)

### Objetivo
Criar `UseCRUDPage` hook + `CRUDPageTemplate` componente que encapsule 80% da lógica de ProfilesPage, SkillsPage, etc.

### Tarefas

#### 2.1 - Criar Hook useCRUDPage
**Arquivo novo**: `src/hooks/useCRUDPage.ts`

```typescript
export interface CRUDPageConfig<T, K extends keyof T = 'id'> {
  // API
  onLoad: () => Promise<T[]>;
  onFetch: (id: string) => Promise<T>;
  onCreate: (data: Partial<T>) => Promise<void>;
  onUpdate: (id: string, data: Partial<T>) => Promise<void>;
  onDelete: (id: string) => Promise<void>;

  // UI
  itemIdKey?: K;
  searchFields?: (keyof T)[];
  columns: DataGridColumn<T>[];

  // Messages
  i18nPrefix?: string;
  onLoadError?: (error: any) => void;
  onSaveSuccess?: () => void;
}

export function useCRUDPage<T, K extends keyof T = 'id'>(config: CRUDPageConfig<T, K>) {
  const [items, setItems] = useState<T[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [editing, setEditing] = useState<T | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);

  // Funções genéricas
  const loadItems = useCallback(async () => { /* ... */ }, [config]);
  const handleEdit = useCallback(async (item: T) => { /* ... */ }, [config]);
  const handleNew = useCallback(() => { /* ... */ }, [config]);
  const handleSave = useCallback(async () => { /* ... */ }, [config, editing, isNew]);
  const handleDelete = useCallback(async (id: string) => { /* ... */ }, [config]);
  const filteredItems = useMemo(() => { /* ... */ }, [items, searchTerm, config]);

  return {
    items: filteredItems,
    loading,
    searchTerm,
    setSearchTerm,
    selectedIds,
    setSelectedIds,
    editing,
    isNew,
    saving,
    loadItems,
    handleEdit,
    handleNew,
    handleSave,
    handleDelete,
  };
}
```

**Impacto**: Reduz ~300 linhas de estado por página  
**Testes**: `useCRUDPage.test.ts` com 20+ cenários

---

#### 2.2 - Criar CRUDPageLayout Componente
**Arquivo novo**: `src/components/layout/CRUDPageLayout.tsx`

```typescript
export interface CRUDPageLayoutProps<T> {
  title: string;
  items: T[];
  columns: DataGridColumn<T>[];
  loading: boolean;
  searchTerm: string;
  onSearchChange: (term: string) => void;
  selectedIds: Set<string | number>;
  onSelectionChange: (ids: Set<string | number>) => void;
  
  // Ações
  onNew: () => void;
  onEdit: (item: T) => void;
  onDelete: (item: T) => void;
  onCellAction?: (item: T, col: DataGridColumn<T>) => void;

  // Editor
  isEditOpen: boolean;
  onEditClose: () => void;
  children?: React.ReactNode; // Editor content renderizado inside modal
}

export function CRUDPageLayout<T>({
  title,
  items,
  columns,
  loading,
  searchTerm,
  onSearchChange,
  selectedIds,
  onSelectionChange,
  onNew,
  onEdit,
  onDelete,
  onCellAction,
  isEditOpen,
  onEditClose,
  children,
}: CRUDPageLayoutProps<T>) {
  return (
    <div className="crud-page">
      <PageToolbar>
        <h1>{title}</h1>
        <Input
          type="text"
          placeholder="Buscar..."
          value={searchTerm}
          onChange={(e) => onSearchChange(e.target.value)}
        />
        <Button onClick={onNew} variant="primary">
          + Novo
        </Button>
      </PageToolbar>

      <DataGrid
        items={items}
        columns={columns}
        selectedIds={selectedIds}
        onSelectionChange={onSelectionChange}
        onActivate={onEdit}
        onDelete={onDelete}
        onCellAction={onCellAction}
        loading={loading}
      />

      <Modal isOpen={isEditOpen} onClose={onEditClose}>
        {children}
      </Modal>
    </div>
  );
}
```

**Impacto**: Elimina ~200 linhas de layout por página  
**Testes**: `CRUDPageLayout.test.ts` com 10+ snapshots

---

#### 2.3 - Refatorar ProfilesPage para Usar Nova Abstração
**Modificar**: `src/pages/ProfilesPage.tsx`

```typescript
export default function ProfilesPage() {
  const { t } = useTranslation();
  const { addToast } = useUIStore();
  const { focusFirstCell, handleGridReady } = useGridFocus();

  const {
    items: profiles,
    loading,
    searchTerm,
    setSearchTerm,
    selectedIds,
    setSelectedIds,
    editing,
    isNew,
    saving,
    loadItems,
    handleEdit,
    handleNew,
    handleSave,
    handleDelete,
  } = useCRUDPage({
    onLoad: GetProfiles,
    onFetch: GetProfile,
    onCreate: CreateProfile,
    onUpdate: UpdateProfile,
    onDelete: DeleteProfile,
    columns: [
      { key: 'name', label: 'Nome' },
      { key: 'description', label: 'Descrição' },
      // ...
    ],
  });

  return (
    <CRUDPageLayout
      title={t('profiles.title')}
      items={profiles}
      columns={/* ... */}
      loading={loading}
      searchTerm={searchTerm}
      onSearchChange={setSearchTerm}
      selectedIds={selectedIds}
      onSelectionChange={setSelectedIds}
      onNew={handleNew}
      onEdit={handleEdit}
      onDelete={handleDelete}
      isEditOpen={!!editing}
      onEditClose={() => { /* ... */ }}
    >
      {/* Editor form específica de Profile */}
    </CRUDPageLayout>
  );
}
```

**Redução**: De ~1268 linhas para ~350 linhas  
**Impacto**: -73% linhas

---

#### 2.4 - Refatorar SkillsPage, McpPage, AllowlistPage, ChannelsPage
**Modificar**: 4 arquivos similares

**Redução esperada**: ~1000 linhas totais de duplicação eliminada

---

#### 2.5 - Criar Documentação de Padrão CRUD
**Arquivo novo**: `docs/CRUD_PAGE_PATTERN.md`

Guia step-by-step:
- Como criar nova página CRUD
- Como customizar editor form
- Como adicionar ações customizadas
- Exemplos completos

---

### Métrica de Sucesso - Fase 2
- ✅ 5 páginas refatoradas (ProfilesPage, SkillsPage, McpPage, AllowlistPage, ChannelsPage)
- ✅ ~2000 linhas de código removidas
- ✅ 30+ testes do hook useCRUDPage
- ✅ 0 bugs reportados em páginas refatoradas
- ✅ Novo desenvolvedor consegue criar página CRUD em <30 min
- ✅ ~50% redução de tempo para manutenção de páginas CRUD

---

## 🎯 FASE 3: Componentização de Chat & Message Display (3-4 semanas)

### Objetivo
Extrair lógica de exibição de mensagens em componentes especializados e reutilizáveis. Melhorar testabilidade da lógica de chat.

### Tarefas

#### 3.1 - Criar MessagePresenter Componente Genérico
**Arquivo novo**: `src/components/chat/MessagePresenter.tsx`

```typescript
export interface MessagePresenterProps {
  role: 'user' | 'assistant' | 'system';
  content: string;
  timestamp?: number;
  
  // Opcional: Features
  hasReasoning?: boolean;
  hasToolCalls?: boolean;
  isEditable?: boolean;
  isStreaming?: boolean;

  // Callbacks
  onEdit?: (content: string) => void;
  onDelete?: () => void;
  onSpeak?: () => void;
  onSendToEditor?: (payload: any) => void;
}

export const MessagePresenter: React.FC<MessagePresenterProps> = React.memo(({
  role,
  content,
  timestamp,
  hasReasoning,
  hasToolCalls,
  isEditable,
  isStreaming,
  onEdit,
  onDelete,
  onSpeak,
  onSendToEditor,
}) => {
  // Renderização genérica, sem TreeView ou threading
  // Essa é apenas a apresentação da mensagem
  return (
    <div className={`message message--${role}`}>
      <div className="message-header">
        <span className="role">{role}</span>
        {timestamp && <span className="time">{formatTime(timestamp)}</span>}
      </div>
      <div className="message-content">
        <MarkdownRenderer content={content} />
      </div>
      {/* Reasoning section*/}
      {hasReasoning && <ReasoningSection /* ... */ />}
      {/* Tool calls */}
      {hasToolCalls && <ToolCallsSection /* ... */ />}
      {/* Actions */}
      <MessageActions
        isEditable={isEditable}
        onEdit={onEdit}
        onDelete={onDelete}
        onSpeak={onSpeak}
        onSendToEditor={onSendToEditor}
      />
    </div>
  );
});
```

**Impacto**: Reduz ChatMessage de ~538 linhas para 200 linhas focadas em threading/state  
**Testes**: `MessagePresenter.test.ts` com 25+ snapshots

---

#### 3.2 - Extrair MessageActions em Componente
**Arquivo novo**: `src/components/chat/MessageActions.tsx`

```typescript
export interface MessageActionsProps {
  isEditable?: boolean;
  onEdit?: (content: string) => void;
  onDelete?: () => void;
  onSpeak?: () => void;
  onSendToEditor?: (payload: any) => void;
}

export const MessageActions: React.FC<MessageActionsProps> = ({
  isEditable,
  onEdit,
  onDelete,
  onSpeak,
  onSendToEditor,
}) => {
  return (
    <div className="message-actions" role="toolbar">
      {isEditable && onEdit && (
        <button onClick={() => { /* trigger edit mode */ }}>Editar</button>
      )}
      {onSpeak && <button onClick={onSpeak}>Ler em voz alta</button>}
      {onSendToEditor && (
        <button onClick={() => onSendToEditor({ /* ... */ })}>
          Enviar para editor
        </button>
      )}
      {onDelete && <button onClick={onDelete}>Deletar</button>}
    </div>
  );
};
```

**Impacto**: Elimina ~80 linhas de código duplicado de actions  
**Testes**: `MessageActions.test.ts` com 10+ casos

---

#### 3.3 - Criar Hook useMessageNode para Lógica de Interação
**Arquivo novo**: `src/hooks/useMessageNode.ts`

```typescript
export interface UseMessageNodeOptions {
  messageId: string;
  onDelete?: (id: string) => Promise<void>;
  onEdit?: (id: string, content: string) => Promise<void>;
  onSpeak?: (id: string) => void;
}

export function useMessageNode(options: UseMessageNodeOptions) {
  const [isEditing, setIsEditing] = useState(false);
  const [editContent, setEditContent] = useState('');
  const [isPlayingAudio, setIsPlayingAudio] = useState(false);

  const handleEdit = useCallback(async () => {
    if (options.onEdit) {
      await options.onEdit(options.messageId, editContent);
      setIsEditing(false);
    }
  }, [options, editContent]);

  const handleDelete = useCallback(async () => {
    if (options.onDelete) {
      await options.onDelete(options.messageId);
    }
  }, [options]);

  const handleSpeak = useCallback(() => {
    if (options.onSpeak) {
      options.onSpeak(options.messageId);
      setIsPlayingAudio(true);
    }
  }, [options]);

  return {
    isEditing,
    setIsEditing,
    editContent,
    setEditContent,
    isPlayingAudio,
    setIsPlayingAudio,
    handleEdit,
    handleDelete,
    handleSpeak,
  };
}
```

**Impacto**: Reduz duplicação de lógica em MessageNode e ChatMessage  
**Testes**: `useMessageNode.test.ts` com 15+ cenários

---

#### 3.4 - Refatorar ReasoningSection & ToolCallsSection
**Modificar**: `src/components/chat/ReasoningSection.tsx`, `ToolCallsSection.tsx`

- Eliminar lógica de exibição genérica
- Melhorar props interface
- Adicionar testes mais robustos

---

#### 3.5 - Criar MessageTree Componente para Hierarquia
**Arquivo novo**: `src/components/chat/MessageTree.tsx`

```typescript
export interface MessageTreeProps {
  nodes: MessageNodeType[];
  onLoadMore?: (nodeId: string) => Promise<MessageNodeType[]>;
  onEdit?: (messageId: string, content: string) => Promise<void>;
  onDelete?: (messageId: string) => Promise<void>;
  onSpeak?: (messageId: string) => void;
}

export const MessageTree: React.FC<MessageTreeProps> = ({
  nodes,
  onLoadMore,
  onEdit,
  onDelete,
  onSpeak,
}) => {
  return (
    <div className="message-tree">
      {nodes.map((node) => (
        <MessageTreeNode
          key={node.message.id}
          node={node}
          onLoadMore={onLoadMore}
          onEdit={onEdit}
          onDelete={onDelete}
          onSpeak={onSpeak}
        />
      ))}
    </div>
  );
};
```

**Impacto**: Separa concern de tree rendering de message rendering  
**Testes**: `MessageTree.test.ts` com 12+ snapshots

---

### Métrica de Sucesso - Fase 3
- ✅ ChatMessage reduzida de 538 linhas para ~200 linhas
- ✅ 4 novos componentes especializados criados
- ✅ 35+ novos testes de chat
- ✅ Tempo de render de lista de mensagens reduzido >20%
- ✅ 0 regressões em threading e streaming
- ✅ Documentação de chat components em `docs/CHAT_COMPONENTS.md`

---

## 🎯 FASE 4: Editor & Terminal Componentização (2-3 semanas)

### Objetivo
Extrair lógica de editor e terminal em componentes menores e testáveis.

### Tarefas

#### 4.1 - Criar EditorCodeBlock Componente
**Arquivo novo**: `src/components/editor/EditorCodeBlock.tsx`

Consolida MermaidCodeBlockNodeView + renderização genérica de code blocks.

---

#### 4.2 - Criar TerminalEntry Base Components
**Arquivo novo**: `src/components/terminal/TerminalEntryBase.tsx`

Separa TerminalCommandNode e TerminalOutputNode em componentes reusáveis.

---

#### 4.3 - Extrair Hook useTerminalHistory
**Arquivo novo**: `src/hooks/useTerminalHistory.ts`

Lógica de histórico de terminal desacoplada de TerminalTabs.

---

### Métrica de Sucesso - Fase 4
- ✅ Editor reduzida de ~500 linhas para ~300 linhas
- ✅ Terminal reduzida de ~400 linhas para ~250 linhas
- ✅ 20+ novos testes de editor/terminal
- ✅ 0 regressões em funcionalidade de editor/terminal

---

## 🎯 FASE 5: Modal & Dialog Consolidação (1-2 semanas)

### Objetivo
Criar sistema centralizado de modais com template reutilizável.

### Tarefas

#### 5.1 - Criar GenericModal Template
**Arquivo novo**: `src/components/ui/GenericModal.tsx`

```typescript
export interface GenericModalProps {
  isOpen: boolean;
  title: string;
  onClose: () => void;
  onSubmit?: () => void;
  submitLabel?: string;
  submitVariant?: 'primary' | 'danger';
  isLoading?: boolean;
  children: React.ReactNode;
}
```

---

#### 5.2 - Refatorar Todos os Modais Existentes
**Modificar**: QuestionnaireDialog, TokenStatsModal, MermaidEditorModal, etc.

---

### Métrica de Sucesso - Fase 5
- ✅ 6+ modais refatoradas
- ✅ ~300 linhas de duplicação removida
- ✅ 15+ novos testes de modais

---

## 🎯 FASE 6: Store & State Management Refactoring (2-3 semanas)

### Objetivo
Centralizar padrões de store e criar abstrações reutilizáveis para async state.

### Tarefas

#### 6.1 - Criar useAsyncStore Hook
**Arquivo novo**: `src/hooks/useAsyncStore.ts`

```typescript
export interface UseAsyncStoreOptions<T> {
  initialState: T;
  loadFn: () => Promise<T>;
  errorHandler?: (error: any) => void;
  onSuccess?: (data: T) => void;
}

export function useAsyncStore<T>(options: UseAsyncStoreOptions<T>) {
  const [data, setData] = useState(options.initialState);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await options.loadFn();
      setData(result);
      options.onSuccess?.(result);
    } catch (e) {
      setError(e);
      options.errorHandler?.(e);
    } finally {
      setLoading(false);
    }
  }, [options]);

  return { data, loading, error, load, setData };
}
```

---

#### 6.2 - Consolidar Padrão em Múltiplos Stores
**Modificar**: chatStore, settingsStore, mcpStore, etc.

---

### Métrica de Sucesso - Fase 6
- ✅ 5+ stores refatoradas
- ✅ ~400 linhas de duplicação em async state removida
- ✅ 25+ testes de async store patterns

---

## 🎯 FASE 7: Validação, Performance & Otimização (2 semanas)

### Objetivo
Garantir estabilidade e performance da refatoração.

### Tarefas

#### 7.1 - Análise de Bundle Size
- Medir impacto de novo bundle size
- Validar code splitting
- Identificar oportunidades de otimização

#### 7.2 - Testes de Integração End-to-End
- Cenários críticos de cada página
- Fluxo de chat completo
- Fluxo de editor completo

#### 7.3 - Performance Profiling
- Render time de componentes críticos
- Memory leaks
- Unnecessary re-renders

#### 7.4 - Documentação Final
- Guia de manutenção
- Padrões estabelecidos
- Best practices

---

## 📊 Métricas de Sucesso Gerais

### Por Fase

| Fase | Linhas Reduzidas | Testes Novos | Tempo Estimado | ROI |
|------|------------------|--------------|---------------|-----|
| 1    | ~500             | 40+          | 1-2 sem       | 🟢 Alto |
| 2    | ~2000            | 30+          | 2-3 sem       | 🟢 Alto |
| 3    | ~600             | 35+          | 3-4 sem       | 🟢 Alto |
| 4    | ~400             | 20+          | 2-3 sem       | 🟡 Médio |
| 5    | ~300             | 15+          | 1-2 sem       | 🟡 Médio |
| 6    | ~400             | 25+          | 2-3 sem       | 🟡 Médio |
| 7    | -                | 20+          | 2 sem         | 🟢 Alto |
| **Total** | **~4200** | **~185+** | **~15-20 semanas** | **🟢 Alto** |

### Impacto Total Esperado

**Antes**:
- ~8000 linhas de código duplicado
- Componentes com 500+ linhas
- Páginas com 1200+ linhas
- Testes frágeis/incompletos
- Manutenção lenta

**Depois**:
- ✅ ~3800 linhas removidas por reutilização
- ✅ Componentes 40-60% menores
- ✅ Páginas 60-75% menores
- ✅ 185+ novos testes
- ✅ Manutenção 50% mais rápida
- ✅ Confiabilidade aumentada
- ✅ Onboarding de devs reduzido

---

## 🚀 Estratégia de Rollout & Risco

### Risk Mitigation

1. **Feature Flags**
   - Cada refatoração atrás de flag
   - Rollback rápido se problema detectado

2. **Testes Abrangentes**
   - Unit tests para cada componente novo
   - Integration tests para fluxos críticos
   - E2E tests para jornadas de usuário

3. **Validação Incremental**
   - Refatorar 1 página por vez
   - Validar 48h antes de próxima
   - Coletar feedback de usuários

4. **Documentação**
   - Cada fase produz guia de uso
   - Exemplos de código completos
   - Video walkthroughs para devs

### Phase Gates

**Após cada fase deve-se validar:**

- [ ] Todos os testes passando
- [ ] 0 regressões reportadas
- [ ] Performance não regrediu
- [ ] Documentação atualizada
- [ ] Code review aprovado por +2 devs
- [ ] Rodado em staging por 48h sem problemas

---

## 📚 Documentação de Suporte

### Documentos Necessários

1. **COMPONENT_PATTERNS.md** - Padrões estabelecidos
2. **CRUD_PAGE_PATTERN.md** - Como criar página CRUD
3. **CHAT_COMPONENTS.md** - Arquitetura de chat
4. **FORM_VALIDATION.md** - Validação e formatting
5. **TESTING_GUIDELINES.md** - Como testar novos componentes
6. **MIGRATION_GUIDE.md** - Para cada page/component refatorada

---

## 💡 Próximos Passos Imediatos

1. **Semana 1**: Iniciar Fase 1
   - [ ] Criar useFormField hook
   - [ ] Criar validators utilitários
   - [ ] Criar FormField componente
   - [ ] Refatorar BasePicker
   - [ ] 40+ testes

2. **Semana 2-3**: Continuar Fase 1 + Iniciar Fase 2
   - [ ] Criar useCRUDPage hook
   - [ ] Criar CRUDPageLayout componente
   - [ ] Refatorar ProfilesPage
   - [ ] 30+ testes

3. **Semana 4+**: Fases subsequentes conforme progresso

---

## 📝 Notas Finais

Este plano é **vivo e iterativo**. Ajustamos conforme aprendemos:

- Se Fase 1 levar mais tempo, atraso fases posteriores
- Se descobrirmos padrão novo, adicionamos à fase apropriada
- Se algo não funcionar, pivotamos rapidamente
- Coletamos retrospectiva após cada fase para melhorar próximas

**O objetivo final é excelência**: código limpo, testável, manutenível e confiável que permita o time crescer e iterar rapidamente sem medo.

---

**Autor**: GitHub Copilot  
**Data de Criação**: 5 de março de 2026  
**Estado histórico**: plano pronto para execução; implementação parcial em andamento
**Próxima Revisão**: Após conclusão da Fase 1
