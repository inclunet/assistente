/** @vitest-environment jsdom */
import { afterEach, describe, expect, it, vi } from 'vitest';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { scheduleQuestionnaireFocusRestore } from './questionnaireFocusRestore';

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

    scheduleQuestionnaireFocusRestore(() => questionnaireOpen, target, () => null, scheduleFrame);
    questionnaireOpen = true;
    callbacks.shift()?.(0);

    expect(document.activeElement).not.toBe(target);
  });

  it('consulta o ModalRegistry no frame e preserva o foco do modal que abriu depois', () => {
    const target = document.createElement('button');
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.append(target, overlay);

    scheduleQuestionnaireFocusRestore(() => false, target, () => null, scheduleFrame);
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

    scheduleQuestionnaireFocusRestore(() => false, target, () => null, scheduleFrame);

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

    scheduleQuestionnaireFocusRestore(() => false, target, () => null, scheduleFrame);
    callbacks.shift()?.(0);

    expect(document.activeElement).toBe(target);
  });
});
