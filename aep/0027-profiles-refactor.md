# Plano de Refatoração: ProfilesPage

**Objetivo**: Migrar ProfilesPage para usar `useEditableList` + extrair componentes reutilizáveis testados

**Princípios**:
- ✅ Reaproveitar código existente antes de criar novo
- ✅ Cada extração deve ter testes
- ✅ Mudanças incrementais e verificáveis
- ✅ Seguir padrão já estabelecido (AllowlistPage, SkillsPage)

---

## Análise do Estado Atual

### Código Existente Reutilizável

#### 1. **useEditableList** (✅ Já existe)
- Hook genérico para CRUD completo
- Usado em: AllowlistPage, SkillsPage
- Features:
  - loadItems, loadItem, createItem, updateItem, deleteItem
  - Estado: items, loading, editingItem, isNew, saving
  - Validação, confirmação de exclusão, mensagens customizadas
  - Callbacks: onSuccess, onDeleteSuccess

#### 2. **useProfileDependencies** (✅ Criado agora)
- Hook para carregar Tools/Skills/Allowlists
- Escuta eventos MCP
- Reduz complexidade do ProfilesPage

#### 3. **Componentes UI Reutilizáveis** (✅ Já existem)
- DataGrid + Toolbar + Modal (padrão em todas as páginas)
- EditorPanel, EditorPanelFields, EditorPanelFooter
- ModelPicker, VoicePicker, STTProviderPicker

### ProfilesPage - Complexidades Únicas

**Lógica de Negócio Específica**:
1. **Perfil Ativo**: Não pode deletar perfil ativo, badge de status
2. **Ação de Ativar**: Botão específico para ativar perfil
3. **Search Paths**: Exibir caminhos de busca de perfis no footer
4. **Editor Complexo**: ~800 linhas de JSX com:
   - Seções colapsáveis (Chat, Voice, STT)
   - Skills com autoload/on-demand + grid navegável
   - Tools com grid + allowlist de comandos
   - Muitos campos numéricos, ranges, toggles

---

## Estratégia de Refatoração Progressiva

### Fase 1: Preparação e Validação ✅ CONCLUÍDA

**Status**: ✅ Feito
- [x] Remover console.log do ProfilesPage
- [x] Criar `useProfileDependencies` hook
- [x] Validar build/lint/test

### Fase 2: Extrair Seções do Editor (Componentes Testáveis)

**Objetivo**: Quebrar o editor gigante em componentes menores e testáveis

#### 2.1 Criar `ProfileGeneralSection.tsx` ✅ Prioritário
**Localização**: `frontend/src/components/profiles/ProfileGeneralSection.tsx`

**Responsabilidade**: Campos básicos (nome, descrição, ícone)

**Props**:
```typescript
interface ProfileGeneralSectionProps {
  profile: Profile;
  onChange: (field: keyof Profile, value: any) => void;
  disabled?: boolean;
}
```

**Testes** (`ProfileGeneralSection.test.tsx`):
- Renderiza campos corretamente
- Chama onChange ao editar
- Respeita disabled

**Reuso**: Campos simples existem em AllowlistPage/SkillsPage, podemos extrair `TextField` component genérico se fizer sentido.

---

#### 2.2 Criar `ProfileChatSection.tsx` ⭐ Alto Impacto
**Localização**: `frontend/src/components/profiles/ProfileChatSection.tsx`

**Responsabilidade**: Configurações de LLM (modelo, temperatura, tokens, reasoning)

**Props**:
```typescript
interface ProfileChatSectionProps {
  chatConfig: ChatConfig;
  onChange: (field: string, value: any) => void;
  disabled?: boolean;
}
```

**Sub-componentes a extrair**:
- `TemperatureSlider` (reutilizável para Volume/Rate/Pitch)
- `ReasoningSelect` (dropdown específico)

**Testes**:
- Renderiza todos os campos de Chat
- ModelPicker integração
- Sliders atualizam valores
- Reasoning dropdown funciona

---

#### 2.3 Criar `ProfileSkillsSection.tsx` 🎯 Mais Complexo
**Localização**: `frontend/src/components/profiles/ProfileSkillsSection.tsx`

**Responsabilidade**: Seção colapsável de Skills (autoload/on-demand)

**Props**:
```typescript
interface ProfileSkillsSectionProps {
  chatConfig: ChatConfig;
  availableSkills: skills.SkillInfo[];
  onChange: (field: string, value: any) => void;
  disabled?: boolean;
}
```

**Sub-componentes a extrair**:
- `CollapsibleSection` (⭐ REUTILIZÁVEL - usar em Voice/Tools/STT)
- `SkillsGrid` (grid navegável com checkboxes)
- `SkillsToolbar` (botões Selecionar todas/Desmarcar/On-demand)

**Testes**:
- Colapsar/expandir funcionam
- Checkbox grid navegação por teclado
- Autoload vs on-demand
- Botões de ação

---

#### 2.4 Criar `ProfileToolsSection.tsx` 🔧 Médio
**Localização**: `frontend/src/components/profiles/ProfileToolsSection.tsx`

**Responsabilidade**: Seção de ferramentas + allowlist de comandos

**Props**:
```typescript
interface ProfileToolsSectionProps {
  chatConfig: ChatConfig;
  availableTools: main.ToolInfo[];
  availableAllowlists: allowlist.AllowlistInfo[];
  onChange: (field: string, value: any) => void;
  disabled?: boolean;
}
```

**Reuso**: 
- `CollapsibleSection` (já criado em 2.3)
- Grid de ferramentas similar a Skills

**Testes**:
- Grid de tools com seleção
- Allowlist dropdown
- Enable/disable all tools

---

#### 2.5 Criar `ProfileVoiceSection.tsx` 🎤 Simples
**Localização**: `frontend/src/components/profiles/ProfileVoiceSection.tsx`

**Responsabilidade**: Configurações TTS (provedor, voz, velocidade, volume)

**Props**:
```typescript
interface ProfileVoiceSectionProps {
  voiceConfig: VoiceConfig;
  onChange: (field: string, value: any) => void;
  disabled?: boolean;
}
```

**Reuso**:
- `CollapsibleSection` (já criado)
- `TemperatureSlider` → renomear para `RangeSlider` (genérico)
- `VoicePicker` (já existe)

**Testes**:
- VoicePicker integração
- Sliders de rate/volume
- Checkboxes TTS agent/user

---

#### 2.6 Criar `ProfileInteractionSection.tsx` 🎙️ Simples
**Localização**: `frontend/src/components/profiles/ProfileInteractionSection.tsx`

**Responsabilidade**: Configurações STT (provedor, idioma, sons de feedback)

**Props**:
```typescript
interface ProfileInteractionSectionProps {
  interactionConfig: InteractionConfig;
  onChange: (field: string, value: any) => void;
  disabled?: boolean;
}
```

**Reuso**:
- `CollapsibleSection` (já criado)
- `STTProviderPicker` (já existe)

**Testes**:
- STTProviderPicker integração
- Campo de idioma
- Checkbox feedback sounds

---

### Fase 3: Criar Componentes Genéricos Reutilizáveis

**Extrair durante a Fase 2, conforme necessário**:

#### 3.1 `CollapsibleSection.tsx` ⭐ ALTO REUSO
**Responsabilidade**: Seção colapsável com header + badge de status

**Props**:
```typescript
interface CollapsibleSectionProps {
  title: string;
  isOpen: boolean;
  onToggle: () => void;
  badge?: 'on' | 'off';
  children: React.ReactNode;
  'aria-expanded'?: boolean;
}
```

**Usado em**: Skills, Tools, Voice, Interaction (4 lugares)

**Testes**:
- Renderiza aberto/fechado
- onToggle funciona
- Badge aparece corretamente
- Acessibilidade (aria-expanded)

---

#### 3.2 `RangeSlider.tsx` ⭐ MÉDIO REUSO
**Responsabilidade**: Input range com valor visual

**Props**:
```typescript
interface RangeSliderProps {
  id: string;
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  onChange: (value: number) => void;
  formatValue?: (value: number) => string;
  disabled?: boolean;
}
```

**Usado em**: Temperature, Top P, Voice Rate, Voice Volume

**Testes**:
- Renderiza com valor correto
- onChange dispara ao mover slider
- formatValue customiza display
- Disabled funciona

---

#### 3.3 `CheckboxGrid.tsx` ⭐ MÉDIO REUSO
**Responsabilidade**: Grid navegável de checkboxes (Skills/Tools)

**Props**:
```typescript
interface CheckboxGridProps<T> {
  items: T[];
  selectedIds: string[];
  onToggle: (id: string) => void;
  getItemId: (item: T) => string;
  getItemLabel: (item: T) => string;
  getItemDescription?: (item: T) => string;
  getBadge?: (item: T) => string;
  'aria-label': string;
}
```

**Usado em**: Skills, Tools

**Testes**:
- Renderiza lista de checkboxes
- onToggle funciona
- Navegação por teclado (arrows, home, end)
- Badge opcional aparece

---

### Fase 4: Migrar ProfilesPage para useEditableList

**Objetivo**: Refatorar ProfilesPage para usar `useEditableList` + componentes extraídos

#### 4.1 Adaptar ProfilesPage para useEditableList

**Mudanças**:
1. Substituir estado manual por `useEditableList`
2. Usar `useProfileDependencies` para tools/skills/allowlists
3. Manter lógica específica de perfil ativo
4. Usar componentes extraídos no editor

**Estrutura final** (~300-400 linhas):
```typescript
export default function ProfilesPage() {
  const { tools, skills, allowlists } = useProfileDependencies();
  const [activeSlug, setActiveSlug] = useState('padrao');
  const [searchPaths, setSearchPaths] = useState<string[]>([]);

  const crud = useEditableList<ProfileRow, Profile, Profile>(
    {
      loadItems: async () => {
        const [profiles, currentSlug, paths] = await Promise.all([
          GetProfiles(),
          GetActiveProfileSlug(),
          GetProfileSearchPaths(),
        ]);
        setActiveSlug(currentSlug);
        setSearchPaths(paths);
        return profiles.map(p => ({ ...p, isActive: p.slug === currentSlug }));
      },
      loadItem: async (id) => await GetProfile(id as string),
      createItem: async (data) => await CreateProfile(data),
      updateItem: async (id, data) => await UpdateProfile(id as string, data),
      deleteItem: async (id) => await DeleteProfile(id as string),
    },
    {
      entityName: 'Perfil',
      canDelete: (item) => {
        if (item.isActive) {
          return 'Não é possível excluir o perfil ativo';
        }
        return true;
      },
      createDefault: () => ({
        name: 'Novo Perfil',
        description: '',
        icon: 'chatbox',
        chat: { /* defaults */ },
        voice: { provider: 'disabled' },
        interaction: { stt_provider: 'webspeech' },
      }),
      // ... validate, messages
    }
  );

  const handleActivateProfile = async (row: ProfileRow) => {
    await SetActiveProfile(row.slug);
    await crud.loadItems(); // Recarrega para atualizar badges
  };

  const updateField = (path: string, value: any) => {
    // Lógica de nested update (já existe no ProfilesPage atual)
    crud.setEditingItem(updatedProfile);
  };

  return (
    <div className="profiles-page">
      <Toolbar /* ... */ />
      <DataGrid /* ... */ />
      <Modal isOpen={!!crud.editingItem} /* ... */>
        <ProfileGeneralSection profile={crud.editingItem} onChange={updateField} />
        <ProfileChatSection chatConfig={crud.editingItem.chat} onChange={updateField} />
        <ProfileSkillsSection chatConfig={crud.editingItem.chat} availableSkills={skills} onChange={updateField} />
        <ProfileToolsSection chatConfig={crud.editingItem.chat} availableTools={tools} allowlists={allowlists} onChange={updateField} />
        <ProfileVoiceSection voiceConfig={crud.editingItem.voice} onChange={updateField} />
        <ProfileInteractionSection interactionConfig={crud.editingItem.interaction} onChange={updateField} />
      </Modal>
      {searchPaths.length > 0 && <SearchPathsFooter paths={searchPaths} />}
    </div>
  );
}
```

#### 4.2 Adicionar Lógica Específica ao useEditableList

**Opção 1**: Usar callbacks existentes (`onSuccess`)
**Opção 2**: Estender useEditableList com hook customizado

```typescript
// Se precisar de lógica muito específica
function useProfilesEditableList() {
  const baseHook = useEditableList(/* ... */);
  
  // Adiciona lógica específica de perfis
  const activateProfile = async (slug: string) => {
    await SetActiveProfile(slug);
    await baseHook.loadItems();
  };

  return {
    ...baseHook,
    activateProfile,
  };
}
```

---

### Fase 5: Testes de Integração

**Objetivo**: Garantir que refatoração não quebrou funcionalidades

#### 5.1 Testes de Componentes (Unitários)
- ✅ Cada componente extraído tem seu arquivo .test.tsx
- ✅ Cobertura mínima: renderização, eventos, props

#### 5.2 Testes de Hooks (Unitários)
- ✅ `useProfileDependencies.test.ts`: carrega dados, escuta MCP
- ⚠️ `useEditableList` já existe mas não tem testes → **criar agora**

#### 5.3 Testes de Integração (ProfilesPage)
**Criar** `ProfilesPage.test.tsx`:
- Renderiza grid de perfis
- Abre editor ao clicar em editar
- Cria novo perfil
- Atualiza perfil existente
- Deleta perfil (exceto ativo)
- Ativa perfil
- Carrega dependências (tools/skills/allowlists)

---

## Cronograma de Execução

### Sprint 1: Componentes Genéricos (2-3 dias)
1. **Dia 1**:
   - [ ] Criar `CollapsibleSection.tsx` + testes
   - [ ] Criar `RangeSlider.tsx` + testes
   - [ ] Validar build/lint/test

2. **Dia 2**:
   - [ ] Criar `CheckboxGrid.tsx` + testes
   - [ ] Criar testes para `useEditableList` (cobrir gaps)
   - [ ] Validar build/lint/test

3. **Dia 3**:
   - [ ] Criar `ProfileGeneralSection.tsx` + testes
   - [ ] Validar build/lint/test

### Sprint 2: Seções Simples (2-3 dias)
4. **Dia 4**:
   - [ ] Criar `ProfileVoiceSection.tsx` + testes
   - [ ] Criar `ProfileInteractionSection.tsx` + testes
   - [ ] Validar build/lint/test

5. **Dia 5**:
   - [ ] Criar `ProfileChatSection.tsx` (sem Skills/Tools) + testes
   - [ ] Validar build/lint/test

### Sprint 3: Seções Complexas (3-4 dias)
6. **Dia 6**:
   - [ ] Criar `ProfileSkillsSection.tsx` + testes
   - [ ] Validar build/lint/test

7. **Dia 7**:
   - [ ] Criar `ProfileToolsSection.tsx` + testes
   - [ ] Validar build/lint/test

### Sprint 4: Migração ProfilesPage (2-3 dias)
8. **Dia 8-9**:
   - [ ] Refatorar ProfilesPage para usar useEditableList
   - [ ] Integrar todos os componentes extraídos
   - [ ] Manter lógica de perfil ativo
   - [ ] Validar build/lint/test

9. **Dia 10**:
   - [ ] Criar testes de integração ProfilesPage.test.tsx
   - [ ] Teste manual completo (criar/editar/deletar/ativar)
   - [ ] Validar build/lint/test
   - [ ] Review final e merge

---

## Checklist de Validação (A Cada Fase)

Antes de avançar para próxima fase:

- [ ] **Lint**: `npm --prefix frontend run lint` passa sem novos erros
- [ ] **Testes**: `npm --prefix frontend run test` passa (100% dos novos componentes testados)
- [ ] **Build**: `npm --prefix frontend run build` compila sem erros
- [ ] **Teste Manual**: Funcionalidade testada no app rodando
- [ ] **Code Review**: Outro dev valida mudanças (ou self-review rigoroso)
- [ ] **Commit**: Mensagem descritiva seguindo padrão do projeto

---

## Métricas de Sucesso

**Antes da Refatoração**:
- ProfilesPage: 1254 linhas
- Nenhum teste
- Lógica duplicada com AllowlistPage/SkillsPage
- Editor monolítico (difícil de testar)

**Depois da Refatoração**:
- ProfilesPage: ~300-400 linhas (usa useEditableList + componentes)
- 10+ componentes testados (unitários)
- 1 teste de integração ProfilesPage.test.tsx
- Componentes reutilizáveis:
  - `CollapsibleSection` (usado em 4 lugares)
  - `RangeSlider` (usado em 4+ lugares)
  - `CheckboxGrid` (usado em 2 lugares)
- Cobertura de testes: ~80% do código refatorado
- Redução de duplicação: useEditableList compartilhado com Allowlist/Skills

---

## Riscos e Mitigações

### Risco 1: Quebrar funcionalidade existente
**Mitigação**: 
- Testes unitários + integração antes de refatorar
- Validar cada fase antes de avançar
- Manter branch separada até conclusão

### Risco 2: Over-engineering (criar componentes desnecessários)
**Mitigação**:
- Só extrair componentes com reuso real (2+ lugares)
- Começar simples, refinar se necessário
- Sempre perguntar: "isso pode usar algo que já existe?"

### Risco 3: Testes inadequados (falsa sensação de segurança)
**Mitigação**:
- Focar em testes de comportamento (não implementação)
- Testar casos de erro (não só happy path)
- Teste manual obrigatório em cada fase

### Risco 4: Prazo longo demais
**Mitigação**:
- Dividir em sprints de 2-3 dias
- Se um sprint atrasar, reavaliar escopo
- Priorizar: genéricos → simples → complexos

---

## Próximos Passos

1. **Review deste plano** com equipe/stakeholder
2. **Decidir ordem de execução**: seguir cronograma ou adaptar?
3. **Começar Sprint 1**: CollapsibleSection + RangeSlider + testes
4. **Check-in diário**: validar progresso e ajustar plano

---

## Notas de Implementação

### Padrão de Nomenclatura
- Componentes: `Profile<Seção>Section.tsx` (ex: ProfileChatSection)
- Testes: `<Componente>.test.tsx` (ex: ProfileChatSection.test.tsx)
- Hooks: `use<Nome>.ts` (ex: useProfileDependencies.ts)
- Testes de hooks: `<Hook>.test.ts` (ex: useProfileDependencies.test.ts)

### Estrutura de Pastas
```
frontend/src/
├── components/
│   ├── profiles/           # Novo: componentes específicos de profiles
│   │   ├── ProfileGeneralSection.tsx
│   │   ├── ProfileGeneralSection.test.tsx
│   │   ├── ProfileChatSection.tsx
│   │   ├── ProfileChatSection.test.tsx
│   │   ├── ProfileSkillsSection.tsx
│   │   ├── ProfileSkillsSection.test.tsx
│   │   ├── ProfileToolsSection.tsx
│   │   ├── ProfileToolsSection.test.tsx
│   │   ├── ProfileVoiceSection.tsx
│   │   ├── ProfileVoiceSection.test.tsx
│   │   ├── ProfileInteractionSection.tsx
│   │   └── ProfileInteractionSection.test.tsx
│   └── ui/                 # Componentes genéricos UI
│       ├── CollapsibleSection.tsx
│       ├── CollapsibleSection.test.tsx
│       ├── RangeSlider.tsx
│       ├── RangeSlider.test.tsx
│       ├── CheckboxGrid.tsx
│       └── CheckboxGrid.test.tsx
├── hooks/
│   ├── useEditableList.ts
│   ├── useEditableList.test.ts  # Novo: adicionar testes
│   ├── useProfileDependencies.ts ✅ Criado
│   └── useProfileDependencies.test.ts # A criar
└── pages/
    ├── ProfilesPage.tsx     # Refatorar: usar useEditableList + componentes
    └── ProfilesPage.test.tsx # Novo: testes de integração
```

### Convenções de Código
- **TypeScript strict mode**: sem `any` sempre que possível
- **Props interfaces**: sempre exportadas (reutilização)
- **Accessibility**: aria-labels, roles, keyboard navigation
- **i18n**: usar `useTranslation` para todos os textos
- **Estilo**: seguir padrão existente (CSS modules ou classes)

---

## Conclusão

Este plano prioriza:
✅ **Reuso** de código existente (useEditableList, components UI)
✅ **Testes** em cada etapa (unitários + integração)
✅ **Incrementalidade** (sprints curtos, validação frequente)
✅ **Simplicidade** (só extrai o que tem reuso real)

**Resultado esperado**: ProfilesPage mais limpo, testado e alinhado com padrão do projeto, sem duplicação de código.
