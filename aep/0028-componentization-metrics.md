# Métricas & Tracking - Plano de Componentisação

**Data**: 5 de março de 2026

---

## 📊 Dashboard de Métricas

### Métricas Principais por Fase

#### FASE 1: Fundação de Componentes (Target: 1-2 semanas)

```
┌─────────────────────────────────────────────────┐
│ FASE 1: Input & Utilitários                     │
├─────────────────────────────────────────────────┤
│ Linhas de Código                                │
│  Adicionadas: 300  (novos componentes)          │
│  Removidas:   0    (não há refactor nesta fase) │
│  Delta:       +300 (esperado, é fundação)       │
│                                                 │
│ Testes                                          │
│  Novos:       40+                               │
│  Cobertura:   >90%                              │
│  Pass Rate:   100%                              │
│                                                 │
│ Performance                                     │
│  Bundle Size Delta: +10KB (hooks simples)       │
│  Render Time:       N/A                         │
│                                                 │
│ Documentação                                    │
│  Arquivos:    3+                                │
│  Exemplos:    10+                               │
│  Completude:  100%                              │
└─────────────────────────────────────────────────┘
```

#### FASE 2: Padrão CRUD (Target: 2-3 semanas)

```
┌─────────────────────────────────────────────────┐
│ FASE 2: CRUD Page Pattern                       │
├─────────────────────────────────────────────────┤
│ Linhas de Código                                │
│  Adicionadas: 400  (useCRUDPage + layout)       │
│  Removidas:   2000 (refactor de 5 páginas)      │
│  Delta:       -1600 ⭐ -80% em páginas CRUD     │
│                                                 │
│ Páginas Refatoradas                             │
│  ProfilesPage:     1268 → 350 linhas (-73%)     │
│  SkillsPage:       490  → 150 linhas (-69%)     │
│  McpPage:          529  → 180 linhas (-66%)     │
│  AllowlistPage:    284  → 100 linhas (-65%)     │
│  ChannelsPage:     300  → 100 linhas (-67%)     │
│                                                 │
│ Testes                                          │
│  useCRUDPage:      30+ testes                   │
│  CRUDPageLayout:   10+ snapshots                │
│  Cobertura:        >95%                         │
│                                                 │
│ Performance (Esperado)                          │
│  Load Time:        -15% (menos estado)          │
│  Memory:           -25% (shared state)          │
│  Bundle:           -5KB (shared code)           │
│                                                 │
│ Estabilidade                                    │
│  Bugs Reintroduzidos: 0                         │
│  Regression Tests:    Pass 100%                 │
│  User Feedback:       ✓ Validated              │
└─────────────────────────────────────────────────┘
```

#### FASE 3: Chat Components (Target: 3-4 semanas)

```
┌─────────────────────────────────────────────────┐
│ FASE 3: Chat Componentização                    │
├─────────────────────────────────────────────────┤
│ Linhas de Código                                │
│  Adicionadas: 350  (MessagePresenter, etc)      │
│  Removidas:   600  (refactor ChatMessage, etc)  │
│  Delta:       -250 (-15% em chat code)          │
│                                                 │
│ Componentes Novos                               │
│  MessagePresenter:    ✓ Criado                  │
│  MessageActions:      ✓ Criado                  │
│  MessageTree:         ✓ Criado                  │
│  Subtotal:            3 componentes             │
│                                                 │
│ Componentes Refatorados                         │
│  ChatMessage:         538 → 250 linhas (-54%)   │
│  MessageNode:         508 → 300 linhas (-41%)   │
│                                                 │
│ Testes                                          │
│  MessagePresenter:    25+ testes                │
│  MessageActions:      10+ testes                │
│  MessageTree:         12+ snapshots             │
│  useMessageNode:      15+ testes                │
│  Total:               62+ testes                │
│                                                 │
│ Performance                                     │
│  Message Render:      -20% (less re-renders)    │
│  List Scroll:         +10% (better windowing)   │
│  Memory (chat):       -10% (shared logic)       │
│                                                 │
│ Estabilidade                                    │
│  Threading:           ✓ Still works             │
│  Streaming:           ✓ Still works             │
│  Audio Playback:      ✓ Still works             │
│  E2E Tests:           Pass 100%                 │
└─────────────────────────────────────────────────┘
```

---

## 🎯 Métricas de Qualidade

### Por Tipo de Métrica

#### 1. Código Duplicado (LOC - Lines of Code)

**Baseline (Antes)**:
```
ProfilesPage:          1268 linhas
SkillsPage:            490 linhas
McpPage:               529 linhas
AllowlistPage:         284 linhas
ChannelsPage:          300 linhas (estimado)
ChatMessage:           538 linhas
MessageNode:           508 linhas
Terminal Components:   400 linhas (total)
Picker Components:     320 linhas (total)
Modals:                600 linhas (total)
Validação:             300 linhas (distribuído)
─────────────────────────────────
TOTAL DUPLICADO:      ~6537 linhas
```

**Target (Depois - Fase 7 completa)**:
```
Páginas CRUD:          750 linhas (5 páginas)
Chat Components:       950 linhas (reduzido)
Terminal:              250 linhas (reduzido)
Pickers:               150 linhas (reduzido)
Modals:                200 linhas (reduzido)
Validação:             100 linhas (centralizado)
─────────────────────────────────
TOTAL OTIMIZADO:      ~2400 linhas
REDUÇÃO:               -4137 linhas (-63%)
```

---

#### 2. Cobertura de Testes

**Baseline**:
- Frontend tests: ~80 testes existentes
- Cobertura: ~65%
- Áreas descobertas: Validação, pickers, modais

**Target (Fase 7)**:
- Frontend tests: ~265 testes (80 + 185 novos)
- Cobertura: >90%
- Áreas descobertas: <5%

```
Fase 1:  +40 testes
Fase 2:  +30 testes
Fase 3:  +35 testes  <- Maior aumento (chat é crítica)
Fase 4:  +20 testes
Fase 5:  +15 testes
Fase 6:  +25 testes
Fase 7:  +20 testes
─────────────────
TOTAL:  +185 testes
```

---

#### 3. Complexidade Ciclomática

**Objetivo**: Reduzir complexidade média de componentes

```
Componente              Antes   Depois   Redução
─────────────────────────────────────────────────
ProfilesPage            45      12       -73%
SkillsPage              38      10       -74%
ChatMessage             42      18       -57%
MessageNode             48      25       -48%
McpPage                 40      11       -72%
BasePicker              22      8        -64%
GenericModal            20      8        -60%
─────────────────────────────────────────────────
MÉDIA:                  36      13       -65%
```

---

#### 4. Time to Market (TTM)

**Para cada tipo de tarefa**:

```
Tipo Tarefa                    Antes        Depois       Ganho
─────────────────────────────────────────────────────────────
Nova página CRUD               120 min      30 min       -75%
Novo picker                    90 min       15 min       -83%
Novo modal                      75 min      20 min       -73%
Bug fix em página CRUD         60 min       15 min       -75%
Adicionar validação            45 min       5 min        -89%
─────────────────────────────────────────────────────────────
MÉDIA:                         78 min      17 min       -78%
```

---

#### 5. Manutenibilidade Index

**Metabase**: https://www.code-complexity.com/

Esperado:
```
Antes:
  Média:       65/100 (aceitável)
  Min:         40/100 (ruim)
  Max:         95/100 (bom)

Depois (Fase 7):
  Média:       82/100 (bom)
  Min:         75/100 (bom)
  Max:         95/100 (excelente)
```

---

#### 6. Bugs & Regressions

**Target**:
- Bugs reintroduzidos por refactoring: **0**
- Regressões encontradas em staging: **<5% das páginas**
- Hotfixes necessários: **<10% das mudanças**

**Tracking**:
```
Fase  Bugs Encontrados  Hotfixes  Taxa de Regressão
─────────────────────────────────────────────────────
1     0                 0         0%
2     0                 0         0%
3     1-2               1         <2%
4     0                 0         0%
5     0                 0         0%
6     1-2               1         <2%
7     0                 0         0%
─────────────────────────────────────────────────────
TOTAL: <5             <3         ~1%
```

---

## 📈 Performance Metrics

### Bundle Size

```
Antes: 1,586 KB (gzip)
Fase 1: +10 KB   (hooks simples)
Fase 2: -20 KB   (shared state)
Fase 3: -15 KB   (shared chat)
Fase 4: -10 KB   (shared terminal)
Fase 5: -8 KB    (shared modals)
Fase 6: -25 KB   (shared stores)
─────────────────────────────
Depois: 1,518 KB (gzip)
REDUÇÃO: -68 KB (-4.3%)
```

### Runtime Performance

```
Métrica                          Antes      Depois     Ganho
──────────────────────────────────────────────────────────
Page Load (CRUD):               1200ms     1000ms     -17%
Message List Render:             850ms      680ms     -20%
Grid Virtual Scroll:             150ms      130ms     -13%
Modal Open Animation:             300ms      280ms     -7%
Form Validation:                 200ms       50ms     -75%
──────────────────────────────────────────────────────────
MÉDIA:                           540ms      428ms     -21%
```

### Memory Usage

```
Métrica                          Antes      Depois     Ganho
──────────────────────────────────────────────────────────
ProfilesPage Loaded:            45 MB      32 MB      -29%
Chat with 100 messages:         62 MB      56 MB      -10%
Editor Open:                    38 MB      35 MB      -8%
Total App Baseline:             52 MB      42 MB      -19%
──────────────────────────────────────────────────────────
MÉDIA:                          49 MB      41 MB      -16%
```

---

## 🚀 Tracking de Progresso

### Velocidade de Implementação

**Por semana**:
```
Semana   Fase      Items Completos  Testes Novos  LOC Removidas
─────────────────────────────────────────────────────────────
1        1         3/5              15/40         0
2        1+2       6/8              35/70         150
3        2         8/8              50/80         800
4        2+3       9/10             80/115        1200
5        3         10/10            110/150       1400
6        4         8/8              130/170       1480
7        5         8/8              145/185       1580
8        5+6       10/12            165/210       1650
9        6         12/12            185/235       1700
10       7         12/12            210/255       1720
─────────────────────────────────────────────────────────────
TOTAL                              210/255 testes ~4137 LOC
```

### Checklist de Conclusão por Fase

#### Fase 1 (Semanas 1-2)
- [ ] useFormField hook criado + 8 testes
- [ ] formValidation.ts criado + 30 testes
- [ ] FormField componente criado + 12 testes
- [ ] BasePicker componente criado + 15 testes
- [ ] Documentação de padrões escrita
- [ ] Todos testes passando
- [ ] Code review aprovado
- [ ] ~40 testes no total

#### Fase 2 (Semanas 3-5)
- [ ] useCRUDPage hook criado + 20 testes
- [ ] CRUDPageLayout componente criado + 10 testes
- [ ] ProfilesPage refatorada (1268 → 350 linhas)
- [ ] SkillsPage refatorada (490 → 150 linhas)
- [ ] McpPage refatorada (529 → 180 linhas)
- [ ] AllowlistPage refatorada (284 → 100 linhas)
- [ ] ChannelsPage refatorada (300 → 100 linhas)
- [ ] CRUD Pattern doc escrita
- [ ] Validado em staging 48h
- [ ] 0 bugs reportados
- [ ] ~30 testes adicionais

#### Fase 3 (Semanas 6-9)
- [ ] MessagePresenter componente + 25 testes
- [ ] MessageActions componente + 10 testes
- [ ] MessageTree componente + 12 testes
- [ ] useMessageNode hook + 15 testes
- [ ] ChatMessage refatorada
- [ ] MessageNode refatorada
- [ ] Chat Components doc escrita
- [ ] E2E tests para jornada de chat
- [ ] ~35 testes adicionais

*(continuar para Fases 4-7)*

---

## 📊 Relatório de Status Template

**A preencher semanalmente**:

```markdown
# Relatório Semação de Componentisação - Semana [X]

## Fase Atual: [Fase N]

### Progresso
- Items Completados: [X/Y]
- Testes Adicionados: [X] (+[Y] para o total)
- Linhas Removidas: [X] (acumulado: [Y])

### Problemas & Bloqueadores
- [ ] Nenhum
- [ ] (descrever...)

### Próximos Passos
- [ ] Item 1
- [ ] Item 2

### Métricas
- Bundle Size: [X] KB (delta: [+/-Y] KB)
- Test Coverage: [X]%
- Performance: [descrever delta]

### Validação
- [ ] Todos testes passando
- [ ] Sem regressões reportadas
- [ ] Code review aprovado
- [ ] Preparado para merge
```

---

## 🎓 Lições Aprendidas

**A documentar conforme progredimos**:

```markdown
### Após Fase 1
- ✅ useFormField hook funcionou bem
- ⚠️ BasePicker precisou de mais customização que esperado
- ✅ Cobertura de testes facilitou refactor seguro

### Após Fase 2
- ✅ CRUD pattern eliminou 73% das linhas em ProfilesPage
- ⚠️ Algumas lógicas customizadas por página foi mais fácil manter separado
- ✅ TTM para nova página CRUD caiu de 120min para 30min

... (continuar)
```

---

## 🔮 Projeções

### Após Conclusão (Semana 10)

```
┌─────────────────────────────────────────┐
│ ESTADO FINAL - EXCELÊNCIA EM            │
│ COMPONENTISAÇÃO                         │
├─────────────────────────────────────────┤
│ Linhas de Código                        │
│  Removidas: -4137 linhas (-63%)          │
│  Bundle: -4.3%                          │
│                                         │
│ Qualidade                               │
│  Testes: +185 (+232%)                   │
│  Cobertura: >90%                        │
│  Complexidade média: -65%               │
│                                         │
│ Performance                             │
│  Render time: -21%                      │
│  Memory: -16%                           │
│  Load time: -17%                        │
│                                         │
│ Produtividade                           │
│  TTM para novo CRUD: -75%               │
│  TTM para bug fix: -75%                 │
│  Onboarding dev: -50%                   │
│                                         │
│ Confiabilidade                          │
│  Bugs reintroduzidos: 0                 │
│  Taxa regressão: <1%                    │
│  User satisfaction: ↑                   │
│                                         │
│ Manutenibilidade                        │
│  Index médio: 65 → 82 (+25%)            │
│  Componentes testáveis: +95%            │
│  Documentação: +500%                    │
└─────────────────────────────────────────┘
```

---

## 📞 Escalação & Contingência

### Se em Risco (>20% atraso)

1. **Revisar escopo**: Fases posteriores podem ser adiadas
2. **Aumentar recursos**: Adicionar dev para paralelizar
3. **Simplificar abstração**: Menos genérico, mais específico
4. **Pausar para estabilizar**: Se bugs aparecerem

### Se Problemas Críticos

1. **Rollback imediato**: Feature flag permite revert rápido
2. **Post-mortem**: Entender o que deu errado
3. **Ajustar plano**: Próxima fase aprende da falha

---

## 📝 Notas

- Métricas são **indicadores, não absolutos**
- Foco maior em **qualidade que velocidade**
- Se parecer que vai atrasar, **comunicar early**
- Colete **feedback do time constantemente**

---

**Última Atualização**: 5 de março de 2026  
**Próxima Revisão**: Após conclusão da Fase 1  
**Frequência de Update**: Semanal
