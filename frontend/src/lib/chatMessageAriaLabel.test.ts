import { describe, expect, it } from 'vitest';
import { buildChatMessageAriaLabel } from './chatMessageAriaLabel';

describe('buildChatMessageAriaLabel', () => {
  const localized = {
    responding: 'Respondendo...',
    reasoning: 'Raciocínio',
    textEditApplied: 'Aplicou uma alteração no texto via ferramenta.',
    noTextContent: 'Sem conteúdo textual.',
    playAudioHint: 'Pressione Espaço para reproduzir áudio.',
  };

  it('usa "Respondendo..." durante streaming quando não há conteúdo', () => {
    const s = buildChatMessageAriaLabel({
      roleLabel: 'Assistente',
      role: 'assistant',
      displayContent: '',
      isStreaming: true,
      timePrefix: 'recebido',
      relativeTime: 'agora',
      isReasoningExpanded: false,
      localized,
    });

    expect(s).toContain('Assistente: Respondendo...');
    expect(s).not.toContain('Sem conteúdo textual');
  });

  it('após finalizar, usa somente rótulos amigáveis quando não há conteúdo textual', () => {
    const s = buildChatMessageAriaLabel({
      roleLabel: 'Assistente',
      role: 'assistant',
      displayContent: '',
      isStreaming: false,
      toolLabels: ['Editando arquivo: notas.md', 'Buscando na web'],
      timePrefix: 'recebido',
      relativeTime: 'há 1 min',
      isReasoningExpanded: false,
      localized,
    });

    expect(s).toContain('Editando arquivo: notas.md. Buscando na web');
    expect(s).not.toContain('text_edit');
  });

  it('após finalizar, descreve text_edit sem nomes disponíveis', () => {
    const s = buildChatMessageAriaLabel({
      roleLabel: 'Assistente',
      role: 'assistant',
      displayContent: '',
      isStreaming: false,
      toolLabels: ['Usando uma ferramenta'],
      toolCallsHasTextEdit: true,
      timePrefix: 'recebido',
      relativeTime: 'há 1 min',
      isReasoningExpanded: false,
      localized,
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
      localized,
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
      localized,
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
      localized,
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
      localized,
    });
    expect(u).not.toContain('Pressione Espaço para reproduzir áudio');
  });
});
