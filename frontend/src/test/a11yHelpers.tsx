/**
 * Helpers de teste reaproveitáveis de acessibilidade (a11y).
 *
 * Motivação (AEP-relacionado, PR #144):
 * O enforcement automatizado tem LACUNAS — `axe-core`/jsdom não calcula
 * contraste de `var(--token)`, não conhece listeners globais em
 * `document`/`window`, e o `stylelint` só pega cor hardcoded (não token
 * semanticamente errado). Nos PRs #141/#143 recorreram defeitos como:
 *   - atalho global agindo atrás de um modal aberto;
 *   - atalho exibido na UI sem handler real correspondente;
 *   - contraste por token errado (`--text-inverse` sobre `--bg-overlay`).
 *
 * Estes helpers fecham essas lacunas com testes unitários simples e
 * reaproveitáveis. NÃO fazem varredura repo-wide (evita quebrar o CI por
 * issues preexistentes): cada helper é pontual e demonstrado em 1–2 casos.
 *
 * Este arquivo só é importado por testes (`*.test.tsx`), nunca por runtime.
 */

import type { Mock } from 'vitest';
import { expect } from 'vitest';
import { render } from '@testing-library/react';
import { Modal, isModalOpen } from '../components/ui/Modal';

/* ──────────────────────────────────────────────────────────────
 * 0. Utilitário de disparo de tecla
 * ────────────────────────────────────────────────────────────── */

/**
 * Cria e despacha um `KeyboardEvent` e devolve o evento já despachado
 * (útil para inspecionar `event.defaultPrevented`).
 *
 * O alvo padrão é `document.body` — e NÃO `window`/`document` — porque a
 * maioria dos handlers globais lê `event.target` e chama
 * `target.closest(...)`. `window`/`document` não possuem `closest`, o que
 * lançaria `TypeError` antes do handler chegar à lógica do atalho. Como o
 * evento sobe com `bubbles: true`, listeners registrados em `window`/
 * `document` (inclusive em fase de captura) continuam sendo acionados.
 *
 * @param init   Campos do KeyboardEvent (`key`, `code`, `ctrlKey`, etc.).
 * @param options.target `HTMLElement` alvo do dispatch (default: `document.body`).
 *   Espera-se um `HTMLElement` (e não `window`/`document`) justamente porque os
 *   handlers globais costumam ler `event.target.closest(...)`, método que só
 *   existe em `Element`. Quem realmente precisar despachar em `window`/
 *   `document` deve fazer cast explícito no call site, ciente do risco.
 * @param options.type   Tipo do evento (default: `'keydown'`).
 */
export function dispatchKey(
  init: KeyboardEventInit,
  options: { target?: HTMLElement; type?: 'keydown' | 'keyup' | 'keypress' } = {},
): KeyboardEvent {
  const { target = document.body, type = 'keydown' } = options;
  // `...init` vem PRIMEIRO; `bubbles`/`cancelable` são forçados DEPOIS para
  // que o chamador não consiga sobrescrevê-los. Isso garante que o evento é
  // sempre cancelável (preserva `event.defaultPrevented`) e bubbling
  // (alcança listeners em `window`/`document`), conforme o contrato acima.
  const event = new KeyboardEvent(type, { ...init, bubbles: true, cancelable: true });
  target.dispatchEvent(event);
  return event;
}

/* ──────────────────────────────────────────────────────────────
 * 1. Atalho global ignorado com Modal aberto
 * ────────────────────────────────────────────────────────────── */

export interface OpenModalHandle {
  /** Desmonta o Modal (remove-o de `OPEN_MODAL_STACK`). */
  unmount: () => void;
}

/**
 * Renderiza um `<Modal isOpen>` real, de modo que `isModalOpen()` passe a
 * retornar `true` (o Modal registra-se em `OPEN_MODAL_STACK` no mount).
 *
 * Reusa o `Modal` compartilhado de propósito: é a mesma infraestrutura que
 * o app usa em produção, então o teste valida o contrato real e não um
 * mock. O chamador é responsável por chamar `unmount()` (ou usar o
 * `expectGlobalShortcutIgnoredWhileModalOpen`, que faz isso por você).
 */
export function renderOpenModal(title = 'Modal de teste'): OpenModalHandle {
  const result = render(
    <Modal isOpen onClose={() => {}} title={title}>
      <button type="button">conteúdo do modal</button>
    </Modal>,
  );

  if (!isModalOpen()) {
    result.unmount();
    throw new Error(
      'renderOpenModal: esperava isModalOpen() === true após renderizar o Modal. ' +
        'Verifique se o Modal continua registrando-se em OPEN_MODAL_STACK.',
    );
  }

  return { unmount: result.unmount };
}

export interface GlobalShortcutModalCase {
  /**
   * Dispara a tecla/combinação do atalho global e devolve o `KeyboardEvent`
   * despachado. Use `dispatchKey(...)` ou despache manualmente.
   */
  dispatch: () => KeyboardEvent;
  /**
   * Spy/mock da AÇÃO DE FUNDO que NÃO pode ocorrer com modal aberto
   * (ex.: `setActiveTab`, `addTab`). O helper afirma que não foi chamado.
   */
  backgroundAction: Mock;
  /**
   * Quando `true`, exige que `event.preventDefault()` tenha sido chamado.
   * Use para teclas que devem SEMPRE prevenir o default do browser/OS antes
   * de respeitar o modal (ex.: F1, Ctrl+Shift+I, Ctrl+T, Ctrl+Tab).
   * Default: `false`.
   */
  expectPreventDefault?: boolean;
  /** Título opcional do Modal aberto no cenário. */
  modalTitle?: string;
}

/**
 * Monta um cenário com um `Modal` ABERTO, dispara a tecla de um atalho
 * global e afirma que a ação de fundo NÃO ocorreu (e, opcionalmente, que o
 * `preventDefault` foi chamado). Reaproveitável por qualquer hook/handler de
 * atalho global (basta montar o hook/componente antes de chamar este helper).
 *
 * O Modal é sempre desmontado ao final, mesmo se a asserção falhar.
 *
 * @example
 * renderHook(() => useWorkspaceKeyboardShortcuts());
 * expectGlobalShortcutIgnoredWhileModalOpen({
 *   backgroundAction: setActiveTab,
 *   expectPreventDefault: true,
 *   dispatch: () => dispatchKey({ key: '2', ctrlKey: true }),
 * });
 */
export function expectGlobalShortcutIgnoredWhileModalOpen(testCase: GlobalShortcutModalCase): void {
  const { dispatch, backgroundAction, expectPreventDefault = false, modalTitle } = testCase;

  const modal = renderOpenModal(modalTitle);
  try {
    backgroundAction.mockClear();
    const event = dispatch();

    expect(
      backgroundAction,
      'a ação de fundo do atalho global NÃO deveria ocorrer com um modal aberto',
    ).not.toHaveBeenCalled();

    if (expectPreventDefault) {
      expect(
        event.defaultPrevented,
        'o atalho deveria chamar preventDefault() mesmo respeitando o modal',
      ).toBe(true);
    }
  } finally {
    modal.unmount();
  }
}

/* ──────────────────────────────────────────────────────────────
 * 2. Verificação atalho ↔ descrição
 * ────────────────────────────────────────────────────────────── */

/**
 * Extrai as combinações de atalho exibidas em um painel a partir dos
 * elementos `<kbd>` renderizados (padrão usado pelo `KeyboardShortcutsHelp`).
 */
export function getDisplayedShortcutCombos(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll('kbd'))
    .map((el) => (el.textContent ?? '').trim())
    .filter((text) => text.length > 0);
}

export interface DisplayedShortcutsCheck {
  /** Combinações exibidas na UI (ex.: extraídas via `getDisplayedShortcutCombos`). */
  displayed: string[];
  /**
   * Conjunto canônico de combinações que o app DE FATO trata. É a fonte
   * única de verdade declarada pelo teste/projeto.
   *
   * NOTA sobre a abordagem: enumerar "handlers reais" automaticamente é
   * inviável (estão espalhados por hooks, stores e componentes). Em vez
   * disso, mantém-se uma lista canônica declarada; o helper afirma que TODA
   * combinação exibida pertence a ela, tornando detectável a divergência
   * "exibido-vs-canônico" (ex.: um atalho foi removido/renomeado do código
   * mas continua no painel). Sempre que um atalho for adicionado/alterado no
   * código, a lista canônica DEVE ser atualizada junto — caso contrário este
   * teste falha, sinalizando a regressão.
   */
  canonical: Iterable<string>;
}

/**
 * Afirma que toda combinação EXIBIDA na UI existe na lista CANÔNICA de
 * atalhos tratados. Pega regressões como exibir um atalho que não tem mais
 * handler real (motivação: o `Ctrl+P` que abria seletor de perfil em vez de
 * navegação de abas — a combinação existe, mas o vínculo exibido-vs-real é
 * frágil; esta verificação garante ao menos a EXISTÊNCIA da combinação).
 *
 * Lança (falha o teste) se houver combinação exibida fora da canônica.
 */
export function expectDisplayedShortcutsAreCanonical(check: DisplayedShortcutsCheck): void {
  const canonicalSet = new Set(check.canonical);
  const orphans = check.displayed.filter((combo) => !canonicalSet.has(combo));

  expect(
    orphans,
    `combinações exibidas sem atalho canônico correspondente: ${orphans.join(', ') || '(nenhuma)'}. ` +
      'Atualize a lista canônica ou remova o atalho do painel.',
  ).toEqual([]);
}

/* ──────────────────────────────────────────────────────────────
 * 3. Par de tokens de contraste
 * ────────────────────────────────────────────────────────────── */

/**
 * Contrato `token de texto → fundos permitidos`.
 *
 * Por que isto existe: `axe-core` no jsdom NÃO resolve `var(--token)`, então
 * um par texto/fundo semanticamente errado (mas com cor não-hardcoded) passa
 * batido tanto pelo axe quanto pelo stylelint. Esta tabela codifica quais
 * pares são aceitáveis em TODOS os temas (escuros, Claro, Alto Contraste),
 * conforme o contrato de `theme.css`.
 *
 * Regras-chave:
 *   - `--text-inverse` é a cor de texto para superfícies SÓLIDAS coloridas
 *     (acento e cores semânticas fortes). NÃO deve ser usado sobre fundos de
 *     superfície (`--bg-*`) nem sobre overlays translúcidos (`--bg-overlay`).
 *   - `--text-primary/secondary/muted/code` são para SUPERFÍCIES (`--bg-*`),
 *     não para fundos de acento/semânticos sólidos.
 *
 * NÃO é exaustivo: é o contrato mínimo a ser consultado por componentes novos
 * (e ampliado conscientemente). Não substitui a verificação manual de
 * contraste numérico ao introduzir novos tokens.
 */
export const ALLOWED_CONTRAST_TOKEN_PAIRS: Readonly<Record<string, readonly string[]>> = {
  '--text-primary': ['--bg-base', '--bg-surface', '--bg-elevated', '--bg-hover', '--bg-muted', '--bg-input'],
  '--text-secondary': ['--bg-base', '--bg-surface', '--bg-elevated', '--bg-hover', '--bg-muted', '--bg-input'],
  '--text-muted': ['--bg-base', '--bg-surface', '--bg-elevated', '--bg-hover', '--bg-muted', '--bg-input'],
  '--text-code': ['--bg-base', '--bg-surface', '--bg-elevated', '--bg-hover', '--bg-muted', '--bg-input'],
  '--text-inverse': [
    '--accent',
    '--accent-hover',
    '--accent-strong',
    '--color-success',
    '--color-success-hover',
    '--color-danger',
    '--color-danger-hover',
    '--color-danger-dark',
    '--color-info',
    '--color-info-hover',
    '--color-warning',
    '--color-warning-hover',
    '--color-purple',
  ],
} as const;

/** Normaliza um token para a forma `--nome` (aceita com ou sem o prefixo). */
function normalizeToken(token: string): string {
  const trimmed = token.trim();
  return trimmed.startsWith('--') ? trimmed : `--${trimmed}`;
}

/**
 * Predicado puro: o par (token de texto, token de fundo) é permitido pelo
 * contrato de contraste? Retorna `false` para tokens de texto desconhecidos.
 */
export function isContrastTokenPairAllowed(textToken: string, bgToken: string): boolean {
  const text = normalizeToken(textToken);
  const bg = normalizeToken(bgToken);
  const allowed = ALLOWED_CONTRAST_TOKEN_PAIRS[text];
  return allowed ? allowed.includes(bg) : false;
}

/**
 * Versão de asserção: falha o teste se o par texto/fundo não for permitido.
 * Use em componentes novos para travar a escolha de tokens.
 *
 * @example
 * assertContrastTokenPairAllowed('--text-inverse', '--accent'); // ok
 * assertContrastTokenPairAllowed('--text-inverse', '--bg-overlay'); // falha
 */
export function assertContrastTokenPairAllowed(textToken: string, bgToken: string): void {
  const text = normalizeToken(textToken);
  const bg = normalizeToken(bgToken);

  expect(
    isContrastTokenPairAllowed(text, bg),
    `par de contraste não permitido: ${text} sobre ${bg}. ` +
      `Fundos permitidos para ${text}: ${(ALLOWED_CONTRAST_TOKEN_PAIRS[text] ?? ['(token de texto desconhecido)']).join(', ')}.`,
  ).toBe(true);
}
