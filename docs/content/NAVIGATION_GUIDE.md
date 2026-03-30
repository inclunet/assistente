---
title: "Guia de Navegação"
weight: 50
---

# Guia de Navegação da Documentação

Roteiros de leitura recomendados dependendo do seu perfil.

## Estrutura

```
docs/
├── Home (_index)              ← COMECE AQUI
├── Downloads                  ← Baixar o app
├── Guias/                     ← Build, release, code signing
├── Configuração/              ← Provedores, voz, MCP, Slack
└── Recursos/                  ← Workspaces, editor, terminal, tarefas
```

## Roteiros por Perfil

### Usuário Final

**Objetivo**: Instalar, configurar e usar o Assistente

1. **[Downloads](downloads/)** — Baixe e instale
2. **[Provedores LLM](configuracao/PROVIDER_CONFIGURATION/)** — Configure seu primeiro provedor
3. **[Voz](configuracao/SPEECH_CONFIGURATION/)** — Configure TTS/STT (se desejar)
4. **Home** → Seção "Atalhos Essenciais" — Aprenda os atalhos

### Desenvolvedor

**Objetivo**: Contribuir ou customizar o projeto

1. **Home** → Seção "Stack Técnica" — Entenda a arquitetura
2. **[Release Quickstart](guias/RELEASE_QUICKSTART/)** — Como fazer releases
3. **[Build com Versão](guias/BUILD_WITH_VERSION/)** — Como buildar
4. **[MCP — Exemplos](configuracao/MCP_CONFIG_EXAMPLES/)** — Adicionar ferramentas
5. **[Skills — Templates](configuracao/SKILL_TEMPLATE_CONTEXT/)** — Criar skills
6. **[Deep Links](recursos/DEEP_LINKS/)** — Navegação programática

### Administrador / DevOps

**Objetivo**: Deploy, code signing, automação

1. **[Code Signing](guias/CODE_SIGNING_SETUP/)** — Assinar executáveis
2. **[Versionamento](guias/VERSIONING/)** — Esquema de versões
3. **[Release Debug](guias/RELEASE_DEBUG/)** — Troubleshooting
4. **[Downloads](downloads/)** → Seção "Para Desenvolvedores"

## Documentação Técnica (AEPs)

Propostas de arquitetura e design estão em `aep/` (Assistente Enhancement Proposals). Consulte `aep/README.md` para o índice completo com status de cada proposta.

**Frequência**: Ler tudo antes de começar, usar ARCHITECTURE para code review

---

### 🧪 Para QA / Validação

**Objetivo**: Validar qualidade, rastrear progresso

**Leitura Recomendada**:
1. `COMPONENTIZATION_METRICS.md` (15 min)
   - Dashboard de métricas
   - Checkpoints de validação
   - O que esperar em cada fase

2. `COMPONENTIZATION_QUICK_REFERENCE.md` (10 min)
   - Red flags
   - Testes strategy

**Frequência**: Ler METRICS agora, revisar checkpoint após cada fase

---

### 📊 Para Gerente de Projeto

**Objetivo**: Rastrear timeline, recursos, riscos

**Leitura Recomendada**:
1. `COMPONENTIZATION_EXECUTIVE_SUMMARY.md` (5 min)
   - Timeline e recursos necessários
   - Próximos passos

2. `COMPONENTIZATION_METRICS.md` (15 min)
   - Tracking semanal
   - Phase gates & validação

3. `COMPONENTIZATION_PLAN.md` (10 min)
   - Fases e dependências
   - Possibilidade de paralelizar

**Frequência**: Ler EXECUTIVE_SUMMARY agora, METRICS toda semana

---

## 🎓 Exercícios de Compreensão

### Exercício 1: Impacto (5 min)
**Pergunta**: Quantas linhas serão removidas e qual é o ROI esperado?

**Resposta** (em EXECUTIVE_SUMMARY):
- 4200 linhas removidas
- ROI 2x em 12 meses
- 75% aceleração em novas features

### Exercício 2: Planejamento (10 min)
**Pergunta**: Qual é a sequência de fases e por quê?

**Resposta** (em PLAN):
- Fase 1 primeiro (fundação)
- Fase 2 segundo (maior impacto - CRUD)
- Fases 3-7 subsequentes (impacto decreasing)

### Exercício 3: Implementação (15 min)
**Pergunta**: Como implemento meu primeiro componente?

**Resposta** (em ARCHITECTURE):
- Seguir padrão de Hook
- 8+ testes inclusos
- Usar FormField como exemplo

### Exercício 4: Validação (10 min)
**Pergunta**: Como valido se Fase 1 foi bem-sucedida?

**Resposta** (em METRICS):
- Checklist de conclusão
- Todos testes passando (100%)
- 0 regressões
- Code review aprovado

---

## 🔗 Referência Cruzada de Documentos

### Referências no EXECUTIVE_SUMMARY
```
→ Para detalhes completos: COMPONENTIZATION_PLAN.md
→ Para números exatos: COMPONENTIZATION_METRICS.md
→ Para técnica: COMPONENT_ARCHITECTURE.md
```

### Referências no PLAN
```
→ Para quick start: COMPONENTIZATION_QUICK_REFERENCE.md
→ Para padrões: COMPONENT_ARCHITECTURE.md
→ Para tracking: COMPONENTIZATION_METRICS.md
```

### Referências no ARCHITECTURE
```
→ Para contexto: COMPONENTIZATION_PLAN.md
→ Para métricas: COMPONENTIZATION_METRICS.md
→ Para checklist: COMPONENTIZATION_QUICK_REFERENCE.md
```

### Referências no METRICS
```
→ Para detalhes: COMPONENTIZATION_PLAN.md
→ Para implementação: COMPONENT_ARCHITECTURE.md
→ Para decisão: COMPONENTIZATION_EXECUTIVE_SUMMARY.md
```

---

## ⏱️ Cronograma de Leitura Recomendado

### Dia 1 (Hoje)
- [ ] Ler `README.md` (5 min)
- [ ] Ler `COMPONENTIZATION_EXECUTIVE_SUMMARY.md` (5 min)
- [ ] Tomar decisão de aprovação
- **Total**: 10 min

### Dia 2-3 (Se aprovado)
- [ ] Tech Lead lê `COMPONENTIZATION_PLAN.md` (20 min)
- [ ] Tech Lead lê `COMPONENT_ARCHITECTURE.md` (20 min)
- [ ] PM lê `COMPONENTIZATION_METRICS.md` (15 min)
- **Total**: 55 min

### Dia 4-5 (Preparação Fase 1)
- [ ] Devs leem `QUICK_REFERENCE.md` (10 min)
- [ ] Devs leem `ARCHITECTURE.md` (20 min)
- [ ] Team lê Fase 1 em `PLAN.md` (10 min)
- [ ] Setup de ambiente & CI/CD
- **Total**: 40 min + setup

### Semana 1+
- [ ] Bookmark `QUICK_REFERENCE.md` & `ARCHITECTURE.md`
- [ ] Usar `METRICS.md` para tracking
- [ ] Revisar checkpoint conforme avança
- [ ] Atualizar `README.md` com progresso

---

## 📋 Checklist de Leitura

### Necessário Ler Antes de Começar

- [ ] `README.md` (visão geral)
- [ ] `COMPONENTIZATION_EXECUTIVE_SUMMARY.md` (decisão)

### Se Aprovado - Tech Lead

- [ ] `COMPONENTIZATION_PLAN.md` (estratégia)
- [ ] `COMPONENT_ARCHITECTURE.md` (padrões)
- [ ] `COMPONENTIZATION_METRICS.md` (tracking)

### Se Aprovado - Devs

- [ ] `COMPONENTIZATION_QUICK_REFERENCE.md` (tática)
- [ ] `COMPONENT_ARCHITECTURE.md` (implementação)
- [ ] Fase específica em `PLAN.md`

### Semanal

- [ ] `COMPONENTIZATION_METRICS.md` (progresso)
- [ ] Checkpoint de fase
- [ ] Atualizar relatório

---

## 🔍 Buscar Resposta Rápida

**"Qual é o impacto esperado?"**  
→ `EXECUTIVE_SUMMARY.md` ou `OVERVIEW.txt`

**"Como implemento XX?"**  
→ `ARCHITECTURE.md` + `PLAN.md`

**"É seguro fazer isso?"**  
→ `PLAN.md` seção Risk Mitigation

**"Como valido sucesso?"**  
→ `METRICS.md` checkpoints

**"Próximo passo?"**  
→ `EXECUTIVE_SUMMARY.md` ou `QUICK_REFERENCE.md`

**"Qual é a timeline?"**  
→ `PLAN.md` ou `OVERVIEW.txt`

**"Qual é o ROI?"**  
→ `EXECUTIVE_SUMMARY.md`

---

## 💾 Fazer Download/Impresso

### Essencial (Todos)
- `README.md` (2 pag)
- `COMPONENTIZATION_EXECUTIVE_SUMMARY.md` (2 pag)

### Tech Lead
- `COMPONENTIZATION_PLAN.md` (8 pag)
- `COMPONENT_ARCHITECTURE.md` (6 pag)
- `COMPONENTIZATION_METRICS.md` (5 pag)

### Devs (Bookmark Digital)
- `COMPONENTIZATION_QUICK_REFERENCE.md` (4 pag)
- `COMPONENT_ARCHITECTURE.md` (6 pag)

### Toda Equipe (Parede/Wiki)
- `COMPONENTIZATION_OVERVIEW.txt` (1 pag visual)
- `COMPONENTIZATION_INDEX.md` (6 pag)

---

## ✅ Verificação de Entendimento

Após ler documentação relevante, você deve entender:

- ✅ **O problema**: 65% duplicação no frontend
- ✅ **A solução**: 7 fases de refatoração
- ✅ **O impacto**: 4200 linhas removidas, 75% mais rápido
- ✅ **O tempo**: 15-20 semanas
- ✅ **O risco**: BAIXO (controlado)
- ✅ **O ROI**: 2x em 12 meses
- ✅ **Os padrões**: 5 camadas bem definidas
- ✅ **O tracking**: Métricas semanais

Se não entender algo, revisite o documento relevante ou consulte Tech Lead.

---

## 🎯 Conclusão

Esta é uma **documentação completa, bem-organizada e pronta para execução**.

Cada pessoa pode ler apenas o que precisa para seu papel.

**Próximo passo**: Ler `README.md` e `EXECUTIVE_SUMMARY.md` (10 min total).

---

**Criado**: 5 de março de 2026  
**Status**: ✅ Pronto  
**Versão**: 1.0
