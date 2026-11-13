/** @vitest-environment jsdom */
import { afterEach, describe, expect, it, vi } from 'vitest';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { captureQuestionnaireFocusOrigin, findActiveChatComposer, scheduleQuestionnaireFocusRestore } from './questionnaireFocusRestore';

const callbacks: FrameRequestCallback[] = [];
const scheduleFrame = (callback: FrameRequestCallback) => {
  callbacks.push(callback);
  return callbacks.length;
};

afterEach(() => {
  callbacks.length = 0;
  unregisterOpenModal('later-modal');
  unregisterOpenModal('origin-modal');
  unregisterOpenModal('nested-questionnaire');
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe('scheduleQuestionnaireFocusRestore', () => {
  it('não rouba foco se outro questionário abre antes do frame', () => {
    const target = document.createElement('button');
    document.body.append(target);
    let questionnaireOpen = false;

    scheduleQuestionnaireFocusRestore(() => questionnaireOpen, captureQuestionnaireFocusOrigin(target), () => null, scheduleFrame);
    questionnaireOpen = true;
    callbacks.shift()?.(0);

    expect(document.activeElement).not.toBe(target);
  });

  it('consulta o ModalRegistry no frame e preserva o foco do modal que abriu depois', () => {
    const target = document.createElement('button');
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.append(target, overlay);

    scheduleQuestionnaireFocusRestore(() => false, captureQuestionnaireFocusOrigin(target), () => null, scheduleFrame);
    registerOpenModal('later-modal');
    callbacks.shift()?.(0);

    expect(document.activeElement).not.toBe(target);
  });

  it('restaura no modal de origem quando só o diálogo aninhado fechou', () => {
    const origin = document.createElement('div');
    origin.className = 'modal-overlay';
    origin.dataset.modalId = 'origin-modal';
    const target = document.createElement('button');
    origin.append(target);
    document.body.append(origin);
    registerOpenModal('origin-modal');

    scheduleQuestionnaireFocusRestore(() => false, captureQuestionnaireFocusOrigin(target), () => null, scheduleFrame);

    const nested = document.createElement('div');
    nested.className = 'modal-overlay';
    nested.dataset.modalId = 'nested-questionnaire';
    document.body.append(nested);
    registerOpenModal('nested-questionnaire');
    unregisterOpenModal('nested-questionnaire');
    nested.remove();
    callbacks.shift()?.(0);

    expect(document.activeElement).toBe(target);
  });

  it('restaura o alvo anterior quando não há questionário nem modal concorrente', () => {
    const target = document.createElement('button');
    document.body.append(target);

    scheduleQuestionnaireFocusRestore(() => false, captureQuestionnaireFocusOrigin(target), () => null, scheduleFrame);
    callbacks.shift()?.(0);

    expect(document.activeElement).toBe(target);
  });

  it('usa o campo de envio quando o alvo anterior é o body ou não aceita foco', () => {
    const fallback = document.createElement('textarea');
    const disabled = document.createElement('button');
    disabled.disabled = true;
    document.body.append(disabled, fallback);
    for (const target of [document.body, disabled]) {
      scheduleQuestionnaireFocusRestore(() => false, captureQuestionnaireFocusOrigin(target), () => fallback, scheduleFrame);
      callbacks.shift()?.(0);
      expect(document.activeElement).toBe(fallback);
      fallback.blur();
    }
  });

  it('retorna ao documento da mensagem que continua em modo de leitura', () => {
    const message = document.createElement('div');
    message.setAttribute('data-message-node', '');
    message.setAttribute('role', 'dialog');
    const oldTarget = document.createElement('a');
    oldTarget.href = 'https://example.com';
    message.append(oldTarget);
    document.body.append(message);
    const origin = captureQuestionnaireFocusOrigin(oldTarget);
    const reading = document.createElement('div');
    reading.setAttribute('role', 'document');
    reading.tabIndex = -1;
    oldTarget.replaceWith(reading);
    scheduleQuestionnaireFocusRestore(() => false, origin, () => null, scheduleFrame);
    callbacks.shift()?.(0);
    expect(document.activeElement).toBe(reading);
    expect(message).toHaveAttribute('role', 'dialog');
  });

  it('não sobrescreve foco adquirido depois do fechamento', () => {
    const previous = document.createElement('button');
    const next = document.createElement('button');
    document.body.append(previous, next);
    scheduleQuestionnaireFocusRestore(() => false, captureQuestionnaireFocusOrigin(previous), () => null, scheduleFrame);
    next.focus();
    callbacks.shift()?.(0);
    expect(document.activeElement).toBe(next);
  });

  it('não restaura um controle em painel oculto', () => {
    const panel = document.createElement('div');
    panel.hidden = true;
    const previous = document.createElement('button');
    const fallback = document.createElement('textarea');
    panel.append(previous);
    document.body.append(panel, fallback);
    scheduleQuestionnaireFocusRestore(() => false, captureQuestionnaireFocusOrigin(previous), () => fallback, scheduleFrame);
    callbacks.shift()?.(0);
    expect(document.activeElement).toBe(fallback);
  });

  it('escolhe o compositor da aba ativa, mesmo quando a aba anterior vem primeiro no DOM', () => {
    const hidden = document.createElement('div');
    hidden.hidden = true;
    hidden.innerHTML = '<textarea class="chat-input__textarea"></textarea>';
    const active = document.createElement('div');
    active.dataset.active = 'true';
    active.innerHTML = '<textarea class="chat-input__textarea"></textarea>';
    const other = document.createElement('div');
    other.innerHTML = '<textarea class="chat-input__textarea"></textarea>';
    document.body.append(hidden, other, active);
    expect(findActiveChatComposer()).toBe(active.querySelector('textarea'));
  });

  it('não escolhe arbitrariamente entre compositores visíveis sem aba ativa', () => {
    document.body.innerHTML = '<textarea class="chat-input__textarea"></textarea><textarea class="chat-input__textarea"></textarea>';
    expect(findActiveChatComposer()).toBeNull();
  });
});
