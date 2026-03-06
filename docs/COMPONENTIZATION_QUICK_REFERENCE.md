# Guia Rápido de Oportunidades de Componentisação

**Última atualização**: 5 de março de 2026

---

## 🎯 Oportunidades Críticas (Implementar Primeiro)

### 1. Padrão CRUD (5 páginas afetadas)
**Status**: 🔴 Crítica | **Impacto**: -40% linhas | **Complexidade**: 🟢 Média

**Páginas**:
- ProfilesPage (~1268 linhas) → ~350 linhas
- SkillsPage (~490 linhas) → ~150 linhas
- McpPage (~529 linhas) → ~180 linhas
- AllowlistPage (~284 linhas) → ~100 linhas
- ChannelsPage (similar)

**Solução**: `useCRUDPage` hook + `CRUDPageLayout` componente

**Código Duplicado**:
```typescript
// Presente em todas as 5 páginas:
const [rows, setRows] = useState([]);
const [loading, setLoading] = useState(true);
const [searchTerm, setSearchTerm] = useState('');
const [selectedIds, setSelectedIds] = useState(new Set());
const [editing, setEditing] = useState(null);
const [isNew, setIsNew] = useState(false);
const [saving, setSaving] = useState(false);

const loadItems = useCallback(async () => { /* 20-40 linhas */ }, [...]);
const handleEdit = useCallback(async (item) => { /* 15-25 linhas */ }, [...]);
const handleNew = useCallback(() => { /* 10-15 linhas */ }, [...]);
const handleSave = useCallback(async () => { /* 30-50 linhas */ }, [...]);
const handleDelete = useCallback(async (item) => { /* 20-30 linhas */ }, [...]);
```

**Ganho**: ~2000 linhas removidas

---

### 2. Validação & Formatação (8+ páginas/componentes)
**Status**: 🔴 Crítica | **Impacto**: -35% validação | **Complexidade**: 🟢 Baixa

**Localidades**:
- ProfilesPage: validação de slug, name, description
- SkillsPage: validação de name, tools
- McpPage: validação de command, args
- AllowlistPage: validação de rules
- Múltiplos formulários

**Solução**: `src/lib/formValidation.ts`

**Código Duplicado**:
```typescript
// Em TODA página com formulário:
if (!name.trim()) { /* erro */ }
if (name.length < 2 || name.length > 100) { /* erro */ }
if (slug && !/^[a-z0-9_-]+$/.test(slug)) { /* erro */ }
```

**Ganho**: ~150 linhas removidas de validação

---

### 3. Pickers & Seletores (4 componentes)
**Status**: 🔴 Crítica | **Impacto**: -50% linhas picker | **Complexidade**: 🟡 Média

**Componentes**:
- ProfilePicker (~80 linhas)
- ModelPicker (~80 linhas)
- STTProviderPicker (~80 linhas)
- VoicePicker (~90 linhas)

**Solução**: `BasePicker` componente genérico

**Padrão**:
```typescript
// Todos os 4 fazem isso:
export const XXXPicker = forwardRef<Ref, Props>(({ value, onChange, ...props }) => {
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const [filter, setFilter] = useState('');

  useEffect(() => {
    // Carregar items via API
  }, []);

  return (
    <Combobox
      items={items}
      selectedValue={value}
      onSelect={onChange}
      // ...
    />
  );
});
```

**Ganho**: ~200 linhas removidas, cada picker reduzido para ~30 linhas

---

## 🟠 Oportunidades Altas (Implementar Após Críticas)

### 4. Chat Message Components (~400 linhas duplicado)
**Status**: 🟠 Alta | **Impacto**: -35% chat code | **Complexidade**: 🟠 Alta

**Componentes**:
- ChatMessage.tsx (~538 linhas)
- MessageNode.tsx (~508 linhas)
- MessageList.tsx (~650 linhas)

**Lógica Duplicada**:
- Renderização de markdown
- Threading/tree rendering
- Reasoning section display
- Tool calls display
- Audio playback

**Solução**:
1. Extrair `MessagePresenter` para renderização pura
2. Criar `MessageActions` para toolbar
3. Criar `MessageTree` para hierarquia

**Ganho**: ~800 linhas removidas, componentes 50% menores

---

### 5. Modal & Dialog System (6+ componentes)
**Status**: 🟠 Alta | **Impacto**: -40% modal code | **Complexidade**: 🟡 Média

**Modais Existentes**:
- QuestionnaireDialog
- TokenStatsModal
- MermaidEditorModal
- CreateChannelModal
- E mais...

**Padrão Duplicado**:
```typescript
// Todos fazem:
const [isOpen, setIsOpen] = useState(false);
const onClose = () => setIsOpen(false);
const onSubmit = async () => { /* ... */ };

return (
  <Modal isOpen={isOpen} onClose={onClose}>
    <div className="modal-header">
      <h2>{title}</h2>
      <button onClick={onClose}>✕</button>
    </div>
    <div className="modal-content">
      {/* conteúdo */}
    </div>
    <div className="modal-footer">
      <button onClick={onClose}>Cancelar</button>
      <button onClick={onSubmit}>OK</button>
    </div>
  </Modal>
);
```

**Solução**: `GenericModal` template componente

**Ganho**: ~300 linhas removidas

---

### 6. Terminal Components (~250 linhas duplicado)
**Status**: 🟠 Alta | **Impacto**: -30% terminal | **Complexidade**: 🟡 Média

**Componentes**:
- TerminalEntry.tsx (TerminalCommandNode + TerminalOutputNode)
- TerminalHistory.tsx
- TerminalTabs.tsx

**Padrão**:
- Ambos precisam renderizar nós em árvore
- Ambos precisam de virtual scrolling
- Ambos lidam com refs e focus

**Solução**: `TerminalNodeBase` + `useTerminalHistory` hook

**Ganho**: ~180 linhas removidas

---

## 🟡 Oportunidades Médias (Nice-to-Have)

### 7. Keyboard Navigation Patterns
**Componentes Afetados**: 8+
- Grid navigation (DataGrid, tabs)
- Combobox navigation
- Context menu navigation

**Solução**: Centralizar em `useKeyboardNav` hook

---

### 8. Form Field Components
**Componentes**: Input + Label + Error em múltiplos places
**Solução**: `FormField` componente reutilizável

---

### 9. Toolbar Patterns
**Localidades**: Chat, Editor, Pages
**Solução**: `GenericToolbar` + template

---

## 📈 Impacto por Prioridade

```
Críticas:
├─ CRUD Pattern        → -2000 linhas  (Fase 2)
├─ Validation          → -150 linhas   (Fase 1)
├─ Pickers             → -200 linhas   (Fase 1)
└─ Subtotal            → -2350 linhas ✓✓✓

Altas:
├─ Chat Components     → -800 linhas   (Fase 3)
├─ Modal System        → -300 linhas   (Fase 5)
├─ Terminal            → -180 linhas   (Fase 4)
└─ Subtotal            → -1280 linhas ✓✓

Médias:
├─ Keyboard Nav        → -150 linhas   (Fase 7)
├─ Form Fields         → -120 linhas   (Fase 1)
├─ Toolbars            → -100 linhas   (Fase 7)
└─ Subtotal            → -370 linhas  ✓

TOTAL ESPERADO         → -4000 linhas ✓✓✓✓
```

---

## ⚡ Quick Start para Cada Oportunidade

### CRUD Pattern Refactoring

**Passo 1**: Criar hook `useCRUDPage`
```bash
# Novo arquivo: src/hooks/useCRUDPage.ts
# Contém: state, load, create, update, delete, search
# Testes: 20+ cenários
```

**Passo 2**: Criar layout `CRUDPageLayout`
```bash
# Novo arquivo: src/components/layout/CRUDPageLayout.tsx
# Contém: toolbar, grid, modal para editor
```

**Passo 3**: Refatorar página por página
```bash
# ProfilesPage.tsx: 1268 → 350 linhas
# SkillsPage.tsx: 490 → 150 linhas
# etc...
```

---

### Validation Refactoring

**Passo 1**: Criar validators
```bash
# Novo arquivo: src/lib/formValidation.ts
# Exports: validators.slug, validators.name, validators.description, etc.
```

**Passo 2**: Usar em FormField
```bash
# Novo arquivo: src/components/ui/FormField.tsx
# Reutilizar em TODA página com input
```

---

### Pickers Refactoring

**Passo 1**: Criar base
```bash
# Novo arquivo: src/components/pickers/BasePicker.tsx
# Genérico com items, selection, filtering
```

**Passo 2**: Refatorar cada picker
```bash
# ProfilePicker: 80 → 30 linhas (reutiliza BasePicker)
# ModelPicker: 80 → 30 linhas
# etc...
```

---

## 🧪 Testing Strategy

Para cada refactoring:

1. **Unit Tests** (60% do tempo)
   - Lógica pura
   - Edge cases
   - Error handling

2. **Component Tests** (30% do tempo)
   - Snapshots
   - User interactions
   - Props validation

3. **Integration Tests** (10% do tempo)
   - Fluxos end-to-end
   - Context integration

**Mínimo por componente**: 8+ testes

---

## 🚨 Red Flags para Evitar

1. ❌ **Refatorar tudo de uma vez**
   - Fazer por fase, validar, progredir

2. ❌ **Remover testes antigos antes de novos existir**
   - Sempre ter cobertura >90%

3. ❌ **Não documentar padrão novo**
   - Cada nova abstração precisa de doc + exemplos

4. ❌ **Ignorar feedback de usuários**
   - Pausar refactoring se bugs aparecerem

5. ❌ **Criar abstrações muito genéricas**
   - Melhor ter 2-3 componentes simples que 1 genérico complexo

---

## ✅ Checklist por Refactoring

```markdown
### [Nome do Refactoring]

- [ ] Criar novo componente/hook
- [ ] Testes unitários (8+)
- [ ] Refatorar 1 página/componente
- [ ] Testes de integração
- [ ] Validar em staging (48h)
- [ ] Zero regressões
- [ ] Documentação escrita
- [ ] Code review aprovado
- [ ] Mergear para main
- [ ] Monitorar metrics
```

---

## 📞 Referências Rápidas

- **Plano Completo**: [COMPONENTIZATION_PLAN.md](./COMPONENTIZATION_PLAN.md)
- **Padrões Estabelecidos**: [COMPONENT_PATTERNS.md](./COMPONENT_PATTERNS.md) *(será criado)*
- **CRUD Pattern**: [CRUD_PAGE_PATTERN.md](./CRUD_PAGE_PATTERN.md) *(será criado)*
- **Chat Architecture**: [CHAT_COMPONENTS.md](./CHAT_COMPONENTS.md) *(será criado)*

---

**Última Atualização**: 5 de março de 2026  
**Próximo Review**: Após conclusão da Fase 1
