# AEP-0051 — Skills: Persistência, System Skills e Portabilidade

**Status**: Draft / decisão adiada  
**Criado em**: 2026-05-05  
**Relacionado**: AEP-0046 (UUIDv7), AEP-0047 (Importação e Exportação), AEP-0050 (Profiles DB)

---

## Resumo

Esta AEP documenta a avaliação de migrar skills de `SKILL.md` no filesystem para SQLite/GORM.

A decisão atual é **não implementar a migração para banco agora**. O desenho de banco fica registrado para uma decisão futura, mas a implementação imediata deve priorizar:

- system skills imutáveis, sempre alinhadas ao default do app
- import/export de skills pela interface e pela CLI
- suporte aos dois formatos: `SKILL.md` padrão e JSON portátil do Assistente
- preservação da compatibilidade com arquivos complementares de skills

Skills continuam sendo um recurso naturalmente baseado em arquivos: o formato `SKILL.md` é interoperável com outros softwares, pode ter arquivos de apoio no mesmo diretório e hoje é usado pelo prompt por meio de caminhos lidos com `read_file`.

---

## Motivação

O banco foi considerado porque poderia trazer:

1. consultas por metadados sem parsear todos os arquivos
2. integridade referencial futura com profiles
3. import/export via mecanismo portátil já usado por mensagens, providers e tasklists
4. versionamento e proteção de skills internas do sistema

Após revisar o código atual, a migração completa para banco ainda não se justifica:

1. **Contrato baseado em path**: `available_skills` informa um caminho de `SKILL.md` para o modelo ler com `read_file`. Se o conteúdo virar DB, ainda será necessário materializar arquivos ou mudar esse contrato.

2. **Arquivos complementares**: skills podem ter templates, exemplos e outros arquivos no diretório. Mover só o `SKILL.md` para o banco cria uma fonte híbrida; mover tudo para o banco aumenta complexidade e reduz interoperabilidade.

3. **Formato interoperável**: `SKILL.md` é o formato natural para compartilhar skills com outros softwares. O banco não deve virar pré-requisito para importação/exportação.

4. **Benefício de query ainda baixo**: os filtros atuais são simples (`auto_load`, invocável por usuário/modelo) e o volume de skills tende a ser pequeno.

5. **Profiles ainda não dependem disso**: enquanto profiles continuam em arquivo, a vantagem de FK entre profiles e skills ainda é futura.

---

## Decisão Atual

### D1 — Não migrar skills para banco nesta etapa

A migração para SQLite fica **adiada**. A AEP deixa o modelo de dados registrado, mas ele não deve ser implementado até haver necessidade concreta.

Critérios mínimos para reabrir a decisão:

- profiles migrados para banco e precisando referenciar skills com integridade
- volume de skills tornando parse/listagem em filesystem um gargalo real
- necessidade de sincronização multi-dispositivo ou multi-usuário
- contrato de prompt deixando de depender de `read_file(Path)`
- decisão clara sobre arquivos complementares em DB, cache ou filesystem

### D2 — System skills devem ser imutáveis

As skills embutidas do app devem ser tratadas como **system skills**:

- sempre iguais ao default distribuído com o app
- não editáveis e não removíveis pela interface, API ou CLI
- atualizadas automaticamente quando o app mudar o default
- customização deve ser feita por cópia/duplicação para uma skill de usuário

Mesmo sem banco, a regra de produto é a mesma: o usuário não altera a skill de sistema diretamente.

### D3 — Importação automática de `~/.assistente/skills` só em uma migração futura para DB

Se a migração para banco for implementada no futuro:

- skills existentes em `~/.assistente/skills` devem ser importadas automaticamente
- após importação bem-sucedida, o diretório original deve ser movido para backup ou removido do caminho ativo
- isso evita reimportação em startups futuros
- conflitos com system skills não podem sobrescrever a system skill local
- em conflito, o importador deve renomear a skill importada ou registrar conflito recuperável

Enquanto a persistência continuar em filesystem, não há importação automática para banco.

### D4 — Import/export é prioridade antes do banco

Deve ser possível importar e exportar skills por:

- interface gráfica
- CLI
- formato `SKILL.md` ou diretório padrão de skill
- formato JSON portátil do Assistente

O formato `SKILL.md` é o formato de interoperabilidade. O JSON portátil é o formato de backup/migração do Assistente, alinhado à AEP-0047.

### D5 — CLI deve cobrir skills

Comandos planejados:

```bash
asst skills list
asst skills export <slug> --format skill --out ./minha-skill/
asst skills export <slug> --format json --out skill.json
asst skills import ./minha-skill/ --format auto --strategy rename
asst skills import skill.json --format json --strategy overwrite
```

O comando geral de dados também deve aceitar skills quando o formato portátil for estendido:

```bash
asst data export --skills --out backup.json
asst data export --skill-slug coding --out coding.json
asst data import backup.json
```

---

## Modelo de Dados Futuro

Este modelo **não está aprovado para implementação agora**. Ele registra o desenho caso a migração para banco volte a fazer sentido.

### Tabela `skills`

```sql
CREATE TABLE skills (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  version TEXT NOT NULL,
  description TEXT NOT NULL,

  display_name TEXT,
  author TEXT,
  author_email TEXT,
  author_url TEXT,
  license TEXT,
  repository TEXT,
  homepage TEXT,

  keywords_json TEXT,
  audience_json TEXT,
  platforms_json TEXT,
  languages_json TEXT,
  frameworks_json TEXT,

  category TEXT,
  subcategory TEXT,
  type TEXT,
  difficulty TEXT,
  min_version TEXT,
  max_version TEXT,

  disable_model_invocation BOOLEAN NOT NULL DEFAULT false,
  user_invocable BOOLEAN NULL,
  argument_hint TEXT,
  skill_context TEXT,
  agent TEXT,
  model TEXT,
  auto_load BOOLEAN NOT NULL DEFAULT false,

  content TEXT NOT NULL,

  kind TEXT NOT NULL DEFAULT 'user',
  readonly BOOLEAN NOT NULL DEFAULT false,

  source_path TEXT,
  cache_path TEXT,
  support_dir TEXT,

  metadata_json TEXT,
  filesystem_json TEXT,
  network_json TEXT,
  tools_json TEXT,
  input_json TEXT,
  output_json TEXT,
  behavior_json TEXT,
  triggers_json TEXT,
  hooks_json TEXT,
  dependencies_json TEXT,
  mcp_json TEXT,

  created_at DATETIME,
  updated_at DATETIME
);
```

Campos principais:

- `slug`: identificador usado por profiles e `/slash`
- `kind`: `system`, `user` ou `imported`
- `readonly`: bloqueia update/delete para system skills
- `content`: corpo Markdown sem frontmatter
- `cache_path`: `SKILL.md` materializado caso o prompt continue dependendo de `read_file`
- `support_dir`: diretório de arquivos complementares
- `metadata_json`: snapshot completo para roundtrip e compatibilidade com campos futuros

Índices previstos:

```sql
CREATE UNIQUE INDEX idx_skills_slug ON skills(slug);
CREATE INDEX idx_skills_kind ON skills(kind);
CREATE INDEX idx_skills_auto_load ON skills(auto_load);
CREATE INDEX idx_skills_user_invocable ON skills(user_invocable);
```

### Tabela `skill_tools`

```sql
CREATE TABLE skill_tools (
  id TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  relation TEXT NOT NULL,
  created_at DATETIME,
  updated_at DATETIME,
  FOREIGN KEY(skill_id) REFERENCES skills(id) ON DELETE CASCADE,
  UNIQUE(skill_id, tool_name, relation)
);
```

`relation` pode representar:

- `allowed`
- `denied`
- `bash_allowed`
- `bash_denied`

### Tabela `skill_migration_state`

```sql
CREATE TABLE skill_migration_state (
  id TEXT PRIMARY KEY,
  migration_key TEXT NOT NULL UNIQUE,
  source_path TEXT NOT NULL,
  backup_path TEXT,
  completed_at DATETIME,
  details_json TEXT,
  created_at DATETIME,
  updated_at DATETIME
);
```

Usada apenas em uma migração futura para garantir idempotência e impedir reimportações.

---

## JSON Portátil Futuro

Quando skills entrarem no formato portátil da AEP-0047, o envelope deve seguir o mesmo padrão existente:

```json
{
  "version": 2,
  "exportedAt": "2026-05-05T15:55:00Z",
  "appVersion": "x.y.z",
  "options": {},
  "resources": {
    "skills": [
      {
        "id": "0196abc0-1111-7000-9000-aaaaaaaaaaaa",
        "slug": "minha-skill",
        "metadata": {
          "name": "minha-skill",
          "version": "1.0.0",
          "description": "Skill personalizada"
        },
        "content": "Markdown sem frontmatter",
        "kind": "user",
        "supportFiles": []
      }
    ]
  }
}
```

System skills podem ser exportadas, mas importá-las nunca deve sobrescrever uma system skill local. Em conflito com system skill, a estratégia padrão deve ser `rename`.

---

## Escopo Recomendado Agora

Implementar antes de qualquer migração para banco:

1. separar semanticamente system skills de skills do usuário
2. bloquear edição/remoção direta de system skills
3. permitir duplicar system skill para customização
4. adicionar import/export via UI e CLI em `SKILL.md`
5. adicionar import/export de skills ao JSON portátil
6. manter arquivos complementares no filesystem

Esse escopo resolve portabilidade e proteção de defaults sem introduzir a complexidade de uma fonte híbrida DB + filesystem.

---

## Riscos

| Risco | Impacto | Mitigação |
|---|---|---|
| DB virar cache caro de `SKILL.md` | Complexidade sem ganho real | Adiar DB até haver necessidade concreta |
| Perder interoperabilidade com outros softwares | Alto | Manter `SKILL.md` como formato principal |
| System skill sofrer drift | Médio | Tratar system skills como imutáveis e duplicáveis |
| Import/export duplicar skills por slug | Médio | Estratégias explícitas: `skip`, `overwrite`, `rename` |
| Migração futura reimportar arquivos antigos | Médio | Mover/remover diretório original após import bem-sucedido |

---

## Conclusão

A migração para banco **não deve ser implementada agora**. As decisões sobre system skills, import/export e proteção de defaults fazem sentido independentemente do backend de persistência e devem ser tratadas primeiro.

O modelo de banco fica documentado para uma fase futura, condicionada a sinais claros de necessidade.
