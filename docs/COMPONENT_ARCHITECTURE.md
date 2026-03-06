# Arquitetura de Componentes Reutilizáveis - Referência

**Data**: 5 de março de 2026  
**Objetivo**: Definir padrões arquiteturais para o novo design system

---

## 🏗️ Camadas de Abstração

```
┌─────────────────────────────────────────────────────┐
│ 🎨 Apresentação (Pages)                             │
│ ├─ ProfilesPage, SkillsPage, etc                    │
│ └─ Usam: Layout, Hooks, Componentes                 │
├─────────────────────────────────────────────────────┤
│ 📦 Layouts & Containers                             │
│ ├─ CRUDPageLayout, ModalLayout, etc                 │
│ └─ Orquestram componentes UI + lógica               │
├─────────────────────────────────────────────────────┤
│ 🧩 Componentes Especializados                       │
│ ├─ MessagePresenter, FormField, BasePicker          │
│ └─ Compostos de UI + lógica específica              │
├─────────────────────────────────────────────────────┤
│ 🎯 Componentes Base (UI)                            │
│ ├─ Button, Input, Modal, DataGrid, Tabs             │
│ └─ Primitivos sem opinião de negócio                │
├─────────────────────────────────────────────────────┤
│ 🪝 Hooks & Lógica                                   │
│ ├─ useCRUDPage, useFormField, useMessageNode        │
│ └─ State, async, effects, computations              │
├─────────────────────────────────────────────────────┤
│ 🔧 Utilitários & Lib                                │
│ ├─ formValidation.ts, dateUtils.ts, etc             │
│ └─ Funções puras, sem side effects                  │
└─────────────────────────────────────────────────────┘
```

---

## 📐 Padrão CRUD Genérico

### Fluxo de Dados

```
┌────────────────────────────────────────────┐
│ CRUDPageLayout (Apresentação)              │
│ ├─ Toolbar (search, new button)            │
│ ├─ DataGrid (lista, actions)               │
│ └─ Modal (editor form)                     │
└────────────────┬─────────────────────────────┘
                 │
                 └─ useCRUDPage Hook
                    ├─ items (filtered)
                    ├─ loading
                    ├─ searchTerm
                    ├─ editing (current item)
                    ├─ isNew (create mode)
                    ├─ saving (submit loading)
                    │
                    ├─ loadItems()      ← GetXXX()
                    ├─ handleEdit()     ← GetXXX(id)
                    ├─ handleNew()      ← UI
                    ├─ handleSave()     ← CreateXXX() ou UpdateXXX()
                    └─ handleDelete()   ← DeleteXXX()
                        │
                        └─ Store/Toast updates
```

### Exemplo: ProfilesPage com Nova Abstração

**Antes (1268 linhas)**:
```typescript
// State
const [profileRows, setProfileRows] = useState<ProfileRow[]>([]);
const [activeSlug, setActiveSlug] = useState('padrao');
const [loading, setLoading] = useState(true);
const [searchTerm, setSearchTerm] = useState('');
const [selectedIds, setSelectedIds] = useState(new Set());
const [editingProfile, setEditingProfile] = useState(null);
const [editingSlug, setEditingSlug] = useState(null);
const [isNew, setIsNew] = useState(false);
const [saving, setSaving] = useState(false);

// Callbacks - ~500 linhas
const loadProfiles = useCallback(async () => { /* ... */ }, [...]);
const handleEditProfile = useCallback(async (row) => { /* ... */ }, [...]);
const handleNew = useCallback(() => { /* ... */ }, [...]);
const handleSave = useCallback(async () => { /* ... */ }, [...]);
const handleDelete = useCallback(async (row) => { /* ... */ }, [...]);

// UI - ~400 linhas
return (
  <div>
    <Toolbar>
      <Input value={searchTerm} onChange={setSearchTerm} />
      <Button onClick={handleNew}>Novo</Button>
    </Toolbar>
    <DataGrid
      items={profileRows}
      onEdit={handleEditProfile}
      onDelete={handleDelete}
      // ... 20+ props
    />
    <Modal isOpen={!!editingProfile} onClose={() => setEditingProfile(null)}>
      {/* Form com 100+ linhas */}
    </Modal>
  </div>
);
```

**Depois (350 linhas)**:
```typescript
export default function ProfilesPage() {
  const { t } = useTranslation();
  const { addToast } = useUIStore();

  // Toda lógica CRUD centralizada no hook
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
  } = useCRUDPage<Profile>({
    onLoad: GetProfiles,
    onFetch: GetProfile,
    onCreate: CreateProfile,
    onUpdate: UpdateProfile,
    onDelete: DeleteProfile,
    columns: [
      { key: 'name', label: t('fields.name') },
      { key: 'description', label: t('fields.description') },
      { key: 'source', label: t('fields.source') },
    ],
  });

  // Só a UI específica de Profile
  return (
    <CRUDPageLayout
      title={t('profiles.title')}
      items={profiles}
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
      {/* Form específica de Profile (~150 linhas) */}
      <ProfileEditor
        profile={editing}
        isNew={isNew}
        isSaving={saving}
        onSave={handleSave}
      />
    </CRUDPageLayout>
  );
}
```

---

## 🧩 Padrão de Picker Genérico

### Arquitetura

```
┌─────────────────────────────────────┐
│ ProfilePicker / ModelPicker / etc   │
│ (thin wrapper)                      │
├─────────────────────────────────────┤
│ └─ BasePicker (genérico)            │
│    ├─ Items list management         │
│    ├─ Filtering logic               │
│    ├─ Selection handling            │
│    └─ Combobox UI                   │
└─────────────────────────────────────┘
```

### Implementação

**BasePicker.tsx**:
```typescript
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
  renderItem?: (item: T) => React.ReactNode; // Customização
}

export const BasePicker = forwardRef<HTMLDivElement, BasePickerProps<any>>(
  ({
    items,
    selectedValue,
    getLabel,
    getValue,
    getSubtitle,
    onSelect,
    placeholder,
    disabled,
    loading,
    error,
    renderItem,
  }, ref) => {
    const [open, setOpen] = useState(false);
    const [filter, setFilter] = useState('');

    const filtered = useMemo(() => {
      if (!filter) return items;
      return items.filter(item =>
        getLabel(item).toLowerCase().includes(filter.toLowerCase())
      );
    }, [items, filter, getLabel]);

    const selected = items.find(item => getValue(item) === selectedValue);

    return (
      <div ref={ref} className="base-picker">
        <Combobox
          isOpen={open}
          onOpenChange={setOpen}
          value={selectedValue}
          onValueChange={onSelect}
          disabled={disabled}
        >
          <ComboboxInput
            placeholder={placeholder}
            value={filter}
            onChange={e => setFilter(e.target.value)}
          />
          <ComboboxContent>
            {loading ? (
              <div className="loading">Carregando...</div>
            ) : filtered.length ? (
              filtered.map(item => (
                <ComboboxItem key={getValue(item)} value={getValue(item)}>
                  {renderItem ? renderItem(item) : (
                    <div>
                      <div className="label">{getLabel(item)}</div>
                      {getSubtitle && (
                        <div className="subtitle">{getSubtitle(item)}</div>
                      )}
                    </div>
                  )}
                </ComboboxItem>
              ))
            ) : (
              <div className="empty">Nenhum item encontrado</div>
            )}
          </ComboboxContent>
        </Combobox>
        {error && <div className="error">{error}</div>}
      </div>
    );
  }
);
```

**ProfilePicker.tsx** (novo, thin wrapper):
```typescript
export const ProfilePicker = forwardRef<ProfilePickerRef, ProfilePickerProps>(
  ({ value, onChange, ...props }, ref) => {
    const [profiles, setProfiles] = useState<ProfileInfo[]>([]);
    const [loading, setLoading] = useState(false);

    useEffect(() => {
      setLoading(true);
      GetProfiles().then(setProfiles).finally(() => setLoading(false));
    }, []);

    return (
      <BasePicker
        ref={ref}
        items={profiles}
        selectedValue={value}
        getLabel={p => p.name}
        getValue={p => p.slug}
        getSubtitle={p => p.description}
        onSelect={onChange}
        loading={loading}
        placeholder="Selecione um perfil..."
        {...props}
      />
    );
  }
);
```

---

## 💬 Padrão de Message Display

### Hierarquia

```
┌─────────────────────────────────────────┐
│ MessageList (container)                 │
│ ├─ Virtual scrolling                    │
│ ├─ Thread expansion management          │
│ └─ List of MessageTreeNode              │
├─────────────────────────────────────────┤
│ └─ MessageTreeNode (tree node)          │
│    ├─ Message content (MessagePresenter)│
│    ├─ Children (recursivo)              │
│    └─ Thread indicator                  │
├─────────────────────────────────────────┤
│ └─ MessagePresenter (renderização)      │
│    ├─ Content (markdown)                │
│    ├─ Reasoning section                 │
│    ├─ Tool calls section                │
│    └─ Actions toolbar                   │
├─────────────────────────────────────────┤
│ └─ MessageActions (toolbar)             │
│    ├─ Edit button                       │
│    ├─ Speak button                      │
│    ├─ Send to editor button             │
│    └─ Delete button                     │
└─────────────────────────────────────────┘
```

### Fluxo de State

```
ChatStore (global)
├─ messages: Message[] (flat list, com IDs)
├─ expandedThreads: Set<messageId>
├─ expandedReasonings: Set<messageId>
├─ editingMessageId: string | null
├─ readingMessageId: string | null
├─ streamingMessageId: string | null
├─ activeToolCalls: ToolCallStatus[]
└─ completedSegments: TurnSegment[]

MessageTreeNode (local state)
├─ isEditing: boolean
├─ editContent: string
├─ isPlayingAudio: boolean
└─ isLoading: boolean (para carregamento de children)
```

---

## 🎛️ Padrão de Modal/Dialog

### Base Template

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
  size?: 'small' | 'medium' | 'large';
  closeOnBackdropClick?: boolean;
}

export const GenericModal: React.FC<GenericModalProps> = ({
  isOpen,
  title,
  onClose,
  onSubmit,
  submitLabel = 'OK',
  submitVariant = 'primary',
  isLoading = false,
  children,
  size = 'medium',
  closeOnBackdropClick = true,
}) => {
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size={size}
      closeOnBackdropClick={closeOnBackdropClick}
    >
      <div className={`modal modal--${size}`}>
        <div className="modal-header">
          <h2>{title}</h2>
          <button onClick={onClose} aria-label="Fechar">✕</button>
        </div>

        <div className="modal-content">
          {children}
        </div>

        {onSubmit && (
          <div className="modal-footer">
            <button onClick={onClose}>Cancelar</button>
            <button
              onClick={onSubmit}
              variant={submitVariant}
              disabled={isLoading}
            >
              {isLoading ? 'Salvando...' : submitLabel}
            </button>
          </div>
        )}
      </div>
    </Modal>
  );
};
```

### Uso

```typescript
// TokenStatsModal refatorada
export const TokenStatsModal: React.FC<TokenStatsModalProps> = ({
  isOpen,
  stats,
  onClose,
}) => {
  return (
    <GenericModal
      isOpen={isOpen}
      title="Estatísticas de Tokens"
      onClose={onClose}
      size="small"
      closeOnBackdropClick
    >
      <div className="token-stats">
        <div className="stat">
          <span>Tokens de Entrada:</span>
          <strong>{stats?.inputTokens}</strong>
        </div>
        <div className="stat">
          <span>Tokens de Saída:</span>
          <strong>{stats?.outputTokens}</strong>
        </div>
        {/* ... */}
      </div>
    </GenericModal>
  );
};
```

---

## 🔄 Padrão de Form Field

### Structure

```
┌──────────────────────────────────┐
│ FormField (wrapper)              │
├──────────────────────────────────┤
│ ├─ label + required indicator    │
│ ├─ Input/Textarea/Select         │
│ ├─ Error message (if present)    │
│ └─ Help text (if present)        │
└──────────────────────────────────┘
```

### Implementation

```typescript
export interface FormFieldProps extends InputProps {
  label?: string;
  error?: string | null;
  helpText?: string;
  required?: boolean;
  fullWidth?: boolean;
}

export const FormField = forwardRef<HTMLInputElement, FormFieldProps>(
  (
    {
      label,
      error,
      helpText,
      required = false,
      fullWidth = true,
      ...props
    },
    ref
  ) => {
    const htmlFor = props.id || props.name;

    return (
      <div className={`form-field ${fullWidth ? 'form-field--full' : ''}`}>
        {label && (
          <label htmlFor={htmlFor}>
            {label}
            {required && <span className="required">*</span>}
          </label>
        )}
        <Input
          ref={ref}
          {...props}
          aria-invalid={!!error}
          aria-describedby={error ? `${htmlFor}-error` : undefined}
        />
        {error && (
          <span id={`${htmlFor}-error`} className="error" role="alert">
            {error}
          </span>
        )}
        {helpText && !error && (
          <span className="help-text">{helpText}</span>
        )}
      </div>
    );
  }
);
```

---

## 🧩 Padrão de Hook Reutilizável

### Estrutura Padrão

```typescript
// src/hooks/useXXX.ts

export interface UseXXXOptions {
  // Config
}

export interface UseXXXReturn {
  // State
  data: T;
  loading: boolean;
  error: any;
  
  // Methods
  method1: () => void;
  method2: (arg: T) => Promise<void>;
}

export function useXXX(options: UseXXXOptions): UseXXXReturn {
  // Implementação
  const [data, setData] = useState<T>(/* ... */);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const method1 = useCallback(() => {
    // ...
  }, [/* deps */]);

  const method2 = useCallback(async (arg: T) => {
    // ...
  }, [/* deps */]);

  return {
    data,
    loading,
    error,
    method1,
    method2,
  };
}
```

### Exemplo: useFormField

```typescript
export interface FormFieldOptions<T> {
  validate?: (value: T) => string | null;
  format?: (value: T) => string;
  parse?: (value: string) => T;
  onChange?: (value: T) => void;
}

export function useFormField<T>(
  initialValue: T,
  options?: FormFieldOptions<T>
) {
  const [value, setValue] = useState(initialValue);
  const [error, setError] = useState<string | null>(null);
  const [touched, setTouched] = useState(false);

  const handleChange = useCallback(
    (newValue: string | T) => {
      const parsed = typeof newValue === 'string'
        ? options?.parse?.(newValue) ?? newValue
        : newValue;

      setValue(parsed);

      if (touched && options?.validate) {
        const err = options.validate(parsed as T);
        setError(err);
      }

      options?.onChange?.(parsed as T);
    },
    [options, touched]
  );

  const handleBlur = useCallback(() => {
    setTouched(true);
    if (options?.validate) {
      const err = options.validate(value);
      setError(err);
    }
  }, [options, value]);

  return {
    value,
    setValue,
    error,
    touched,
    setTouched,
    handleChange,
    handleBlur,
    isValid: !error,
  };
}
```

---

## 📋 Checklist de Implementação

Para criar novo componente reutilizável:

- [ ] **Interface clara**
  - Props bem documentadas
  - Return types especificados
  - Defaults apropriados

- [ ] **Testes abrangentes**
  - Casos normais (8+)
  - Edge cases
  - Error handling
  - Cobertura >90%

- [ ] **Documentação**
  - Exemplos de uso
  - Props table
  - Padrões
  - Troubleshooting

- [ ] **Acessibilidade**
  - ARIA labels
  - Keyboard support
  - Screen reader friendly
  - Focus management

- [ ] **Performance**
  - Sem rerenders desnecessários
  - Memoization onde apropriado
  - Callbacks em useCallback

- [ ] **Composabilidade**
  - Aceitar children
  - Slots quando apropriado
  - Render props opcionais

---

## 🎯 Próximas Camadas de Abstração

### Após Fase 7, considerar:

1. **Custom Hooks Library**
   - `useAsync`, `useFetch`, `useDebounce`, `useLocalStorage`
   - Reutilizável em qualquer aplicação React

2. **Design System Completo**
   - Componentes de UI base documentados
   - Tema centralizado
   - Padrões de design

3. **State Management Layer**
   - Abstrair Zustand patterns
   - Centralizar async logic
   - Shared state patterns

4. **API Client Abstraction**
   - Centralizar chamadas Wails
   - Cache patterns
   - Error handling global

---

**Última Atualização**: 5 de março de 2026  
**Próxima Revisão**: Após conclusão da Fase 1
