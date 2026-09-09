import { describe, expect, it } from 'vitest';

import en from '../locales/en';
import es from '../locales/es';
import ptBR from '../locales/pt-BR';

describe('mensagens localizadas do limite de conteúdo', () => {
  it.each([
    ['pt-BR', ptBR.translation.chat.validation.messageTooLarge],
    ['en', en.translation.chat.validation.messageTooLarge],
    ['es', es.translation.chat.validation.messageTooLarge],
  ])('%s comunica bytes e KiB, sem chamar o limite de caracteres', (_locale, message) => {
    expect(message).toContain('{{sizeBytes}}');
    expect(message).toContain('{{maxKiB}} KiB');
    expect(message.toLocaleLowerCase()).not.toMatch(/caracter|character/);
  });

  it.each([
    ['pt-BR', ptBR.translation.editor.chatModal.prepareSelectionTooLarge],
    ['en', en.translation.editor.chatModal.prepareSelectionTooLarge],
    ['es', es.translation.editor.chatModal.prepareSelectionTooLarge],
  ])('%s preserva como caracteres o limite independente da seleção do editor', (_locale, message) => {
    expect(message).toContain('{{max}}');
    expect(message.toLocaleLowerCase()).toMatch(/caracter|character/);
    expect(message).not.toMatch(/\b(bytes?|KiB)\b/i);
  });
});
