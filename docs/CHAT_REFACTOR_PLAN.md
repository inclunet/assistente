# Plano de Refatoração do Chat

Objetivo
- Garantir atualização de UI em tempo real, sem espelhos reativos conflitantes.
- Simplificar fluxo de streaming e de threads com snapshots imutáveis.
- Centralizar acessibilidade (aria-live/TTS) e reduzir efeitos colaterais.

Escopo
- Frontend: `frontend/src/pages/chat/Chat.svelte`, `frontend/src/components/chat/wrappers/ChatContainer.svelte`, `frontend/src/components/chat/core/ChatHistory.svelte`, `frontend/src/components/chat/core/input/ChatInput.svelte`.
- Serviço: `frontend/src/lib/chat/message-service.js`.

Etapas (com critérios de aceite)
1. Fonte Única de Estado (Chat.svelte)
   - Remover mirrors locais de stores e subscriptions manuais.
   - Consumir diretamente `messageService.stores` no template e repassar ao `ChatContainer`.
   - Aceite: envio exibe placeholder e resposta; `$messages`/threads propagam sem duplicações.

2. Threads Imutáveis (MessageService)
   - Consolidar `_applyThreads(...)` em todos os caminhos de atualização (placeholders, chunks, internal/tool/agent, reload).
   - Derivar `messages` exclusivamente de `conversationData.threads` a cada snapshot.
   - Aceite: qualquer atualização dispara novo snapshot e re-render imediato; logs não mostram "placeholder já existe" em duplicidade.

3. Simplificação do Contêiner (ChatContainer)
   - Usar diretamente stores/propriedades; remover clones extras após etapa 1/2.
   - Manter API de expansão (paths), lazy-load e foco mínimo.
   - Aceite: alternar entre modo threaded e plano sem oscilações; render coerente.

4. Acessibilidade & Áudio (aria-live/TTS)
   - Centralizar anúncios (mensagens novas, streamingStarted/Ended, toolsExecution/Results) em um util simples.
   - Garantir limpeza do input e foco pós-envio; manter sons sincronizados.
   - Aceite: leitores anunciam em português; TTS/sons não duplicam.

5. Observabilidade e limpeza
   - Manter logs de diagnóstico durante migração; remover após validação.
   - Aceite: console limpo (sem WARN redundantes), apenas eventos relevantes.

6. Validação funcional
   - Cenários: enviar texto; enviar mídia; streaming longo; chamadas de ferramentas; multi-aba.
   - Aceite: histórico atualiza continuamente; input limpa; aria-live anuncia; sem travamentos.

Riscos e Mitigações
- Mudanças em stores podem afetar componentes dependentes: mitigar com passos menores e testes manuais.
- Lazy-load de filhos pode introduzir estados incorretos: validar paths e restaurar foco.

Rollback
- Caso surjam regressões críticas, reverter para o estado anterior usando Git.

Notas de Implementação
- Respeitar imutabilidade: gerar novos arrays/objetos ao atualizar stores.
- Evitar side-effects em componentes; confinar lógica de estado no `MessageService`.

Checklist de Tarefas
- [ ] Etapa 1: Fonte única em `Chat.svelte`.
- [ ] Etapa 2: `_applyThreads` unificado e `messages` derivados de `threads`.
- [ ] Etapa 3: Simplificação `ChatContainer`.
- [ ] Etapa 4: Centralizar aria-live/TTS.
- [ ] Etapa 5: Limpeza de logs.
- [ ] Etapa 6: Validação final.
