import { describe, expect, it } from 'vitest';
import { buildChatMessageAriaLabel } from './chatMessageAriaLabel';

describe('buildChatMessageAriaLabel', () => {
  it('usa "Respondendo..." durante streaming quando não há conteúdo', () => {
    const s = buildChatMessageAriaLabel({
      roleLabel: 'Assistente',
      role: 'assistant',
      displayContent: '',
      isStreaming: true,
      timePrefix: 'recebido',
      relativeTime: 'agora',
      isReasoningExpanded: false,
    });

    expect(s).toContain('Assistente: Respondendo...');
    expect(s).not.toContain('Sem conteúdo textual');
  });

  it('após finalizar, descreve tool calls quando não há conteúdo textual', () => {
    const s = buildChatMessageAriaLabel({
      roleLabel: 'Assistente',
      role: 'assistant',
      displayContent: '',
      isStreaming: false,
      toolCallsRaw: JSON.stringify([{ function: { name: 'text_edit' } }, { function: { name: 'web_search' } }]),
      timePrefix: 'recebido',
      relativeTime: 'há 1 min',
      isReasoningExpanded: false,
    });

    expect(s).toContain('Executou ferramentas: text_edit, web_search');
  });

  it('após finalizar, usa fallback de text_edit quando toolCallsRaw não parseia', () => {
    const s = buildChatMessageAriaLabel({
      roleLabel: 'Assistente',
      role: 'assistant',
      displayContent: '',
      isStreaming: false,
      toolCallsRaw: '{invalid-json',
      toolCallsHasTextEdit: true,
      timePrefix: 'recebido',
      relativeTime: 'há 1 min',
      isReasoningExpanded: false,
    });

    expect(s).toContain('Aplicou uma alteração no texto via ferramenta');
  });

  it('após finalizar, usa fallback "Sem conteúdo textual" quando não há tool calls', () => {
    const s = buildChatMessageAriaLabel({
      roleLabel: 'Assistente',
      role: 'assistant',
      displayContent: '',
      isStreaming: false,
      timePrefix: 'recebido',
      relativeTime: 'há 1 min',
      isReasoningExpanded: false,
    });

    expect(s).toContain('Sem conteúdo textual');
  });

  it('inclui reasoning quando expandido', () => {
    const s = buildChatMessageAriaLabel({
      roleLabel: 'Assistente',
      role: 'assistant',
      displayContent: 'Ok',
      isStreaming: false,
      reasoning: '**passo** 1',
      isReasoningExpanded: true,
      timePrefix: 'recebido',
      relativeTime: 'agora',
    });

    expect(s).toContain('Raciocínio: passo 1');
  });

  it('inclui dica de áudio apenas para assistant quando não está streaming', () => {
    const a = buildChatMessageAriaLabel({
      roleLabel: 'Assistente',
      role: 'assistant',
      displayContent: 'Oi',
      isStreaming: false,
      timePrefix: 'recebido',
      relativeTime: 'agora',
      isReasoningExpanded: false,
    });
    expect(a).toContain('Pressione Espaço para reproduzir áudio');

    const u = buildChatMessageAriaLabel({
      roleLabel: 'Você',
      role: 'user',
      displayContent: 'Oi',
      isStreaming: false,
      timePrefix: 'enviado',
      relativeTime: 'agora',
      isReasoningExpanded: false,
    });
    expect(u).not.toContain('Pressione Espaço para reproduzir áudio');
  });
});
