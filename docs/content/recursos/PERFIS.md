---
title: "Perfis e Delegação"
weight: 9
---

# Perfis e Delegação

Perfis isolam provedores, voz, skills e contexto por finalidade. A delegação autorizada permite que um perfil atue em nome de outro com limites explícitos.

## Perfis

Cada perfil guarda provedor LLM, voz, skills habilitadas e contexto. Troque em **Configurações → Perfis**; o workspace e as conversas seguem o perfil ativo. Perfis são descobríveis via catálogo e podem ser delegados.

As descrições ajudam o assistente a escolher o perfil adequado:

- **Padrão** atende pesquisa, análise, escrita, organização e solicitações gerais ou ainda pouco definidas.
- **Programação** atende implementação, correção de bugs, depuração, refatoração, testes e revisão de software.

O assistente pode consultar a lista de perfis durante uma conversa. Se uma especialização pontual bastar, ele pode propor um subagente sem alterar o perfil principal. Se a mudança for útil para os próximos turnos, pode propor a troca persistente. O aplicativo sempre pede sua autorização antes de usar outro perfil.

## Delegação

Um perfil pode delegar para outro com escopo e expiração. A delegação é explícita, revogável e auditável — útil para automações que precisam agir como o usuário sem compartilhar credenciais. Veja AEP-0101 para o contrato completo.
