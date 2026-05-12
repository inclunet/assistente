---
title: "Skills — Templates"
weight: 6
---

# Skill Templates — Contexto disponível

Algumas skills são processadas com Go `text/template` antes de serem inseridas no prompt. Isso permite usar condicionais e interpolar dados do perfil ativo.

## Variáveis

No template, o objeto raiz (`.`) expõe:

- `.ProfileSlug` (string): slug do perfil ativo (quando disponível).
- `.ToolCallingEnabled` (bool): `true` quando o conjunto final de ferramentas habilitadas para a conversa não está vazio.
- `.EnabledToolCount` (int): número de ferramentas habilitadas.
- `.EnabledTools` ([]string): nomes das ferramentas habilitadas.
- `.Profile` (*profiles.Profile): struct completa do perfil ativo (campos do JSON).
- `.Surface` (*chat.SurfaceInfo): superfície que originou o envio, quando disponível.

## Surface

Quando `.Surface` existir, ela expõe:

- `.Surface.Type` (string): tipo da aba/superfície (`editor`, `terminal`, `tasklist`, `chat`)
- `.Surface.Title` (string): título visível da aba
- `.Surface.State` (`map[string]any`): espelho de `WorkspaceTab.state`
- `.Surface.Context` (`map[string]any`): contexto transitório do envio atual

## Funções auxiliares

Além das variáveis, o engine já suporta:

- `{{ now }}`: timestamp atual (RFC3339).
- `{{ include "namespace/arquivo" }}`: inclui conteúdo de outro arquivo dentro de `.assistente/skills/<namespace>/...`.

## Exemplo

```gotemplate
{{- if .ToolCallingEnabled -}}
Use a tool `text_edit`.
{{- else -}}
Responda com um bloco ```editor_patch```.
{{- end -}}
```
