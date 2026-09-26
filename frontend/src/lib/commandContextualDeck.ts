import { isContextualPaletteCommand, isContextualPagePaletteCommand, isContextualPagePaletteSurface } from './commandContextualPalette';
import { isCommandLayerAction } from './commandLayerActions';
import { isEditorMermaidMutation } from './commandEditorMermaid';
import { isLocalUICommand } from './commandLocalUI';
import { createLocalPaletteConditionResolverFromParsed, parseLocalPaletteConditions, resolveLocalPaletteConditionSelectionFromParsed, type LocalCommandPaletteVisualContext } from './commandLocalPaletteConditions';
import type { ContextualPaletteCommandLease } from './commandWorkspaceTabWails';
import type { UICommandBeginResponse } from './commandUIExecution';
import { createCommandUIExecutionWailsPort, type CommandUIExecutionWailsOptions } from './commandUIExecutionWails';
import { waitForWailsBridge } from './waitForWailsBridge';

export function isContextualDeckCommand(id: unknown): id is string {
  return isContextualPaletteCommand(id) || isEditorMermaidMutation(id);
}

export function selectContextualDeckCommand(raw: unknown, context: LocalCommandPaletteVisualContext): string | null {
  return selectContextualDeckSelection(raw, context)?.commandId ?? null;
}

export function selectContextualDeckSelection(raw: unknown, context: LocalCommandPaletteVisualContext): { commandId: string; arguments?: Readonly<Record<string, unknown>> } | null {
  const conditions = parseLocalPaletteConditions(raw);
  if (!conditions?.length || conditions.some(c => !isLocalUICommand(c.commandId) && !isContextualDeckCommand(c.commandId))) return null;
  if (conditions.some(c => isContextualPagePaletteCommand(c.commandId) &&
      (c.bySurfaceId !== undefined || Object.values(c.byProfile ?? {}).some(profile => profile.bySurfaceId !== undefined)))) return null;
  const resolve = createLocalPaletteConditionResolverFromParsed(conditions);
  const matches = conditions.filter(c => resolve(c.commandId, context));
  if (matches.length !== 1) return null;
  const id = matches[0].commandId;
  if (isEditorMermaidMutation(id) && context.surfaceType !== 'editor') return null;
  if (isContextualPagePaletteCommand(id) && !isContextualPagePaletteSurface(id, context.surfaceType)) return null;
  const selection = resolveLocalPaletteConditionSelectionFromParsed(conditions, id, context);
  return selection?.available ? { commandId: id, ...(selection.arguments ? { arguments: selection.arguments } : {}) } : null;
}

type DeckWindow = NonNullable<CommandUIExecutionWailsOptions['target']> & { go?: { app?: { App?: {
  BeginContextualDeckUICommand?: (offerID: string, generation: string,
    observed: ContextualPaletteCommandLease['observed']) => Promise<UICommandBeginResponse>;
  BeginContextualDeckPageUICommand?: (offerID: string, generation: string,
    observed: ContextualPaletteCommandLease['observed']) => Promise<UICommandBeginResponse>;
} } } };

/** Physical source; no palette source, arguments, serial or target cross Begin. */
export function createContextualDeckLease(options: {
  offerId: string; generation: string; commandId: string;
  observed: ContextualPaletteCommandLease['observed'];
  isCurrent(): boolean;
  prepareNativeFileContinuation?: () => (() => boolean) | undefined;
  target?: DeckWindow;
}): ContextualPaletteCommandLease {
  const { offerId, generation, commandId } = options;
  if (isCommandLayerAction(commandId)) throw new Error('deck-layer-requires-backend-execution');
  const observed = Object.freeze({ ...options.observed });
  const expires = Date.now() + 10000;
  let consumed = false;
  return Object.freeze({ source: 'deck' as const, commandId, generation, observed,
    isCurrent: options.isCurrent,
    prepareNativeFileContinuation: options.prepareNativeFileContinuation,
    beginUICommand: async () => {
      if (consumed) throw new Error('deck-offer-consumed');
      consumed = true;
      const target: DeckWindow = options.target ?? window;
      await waitForWailsBridge({ target });
      const app = target.go?.app?.App;
      let pending: Promise<UICommandBeginResponse>;
      if (isContextualPagePaletteCommand(commandId)) {
        const begin = app?.BeginContextualDeckPageUICommand;
        if (typeof begin !== 'function') throw new Error('Contextual Deck page API unavailable');
        if (!isContextualPagePaletteSurface(commandId, observed.surfaceType) || Date.now() >= expires || !options.isCurrent()) throw new Error('deck-offer-stale');
        pending = begin.call(app, offerId, generation, {
          surfaceId: '', surfaceType: observed.surfaceType, appPage: observed.appPage, profile: observed.profile ?? '',
        });
      } else {
        const begin = app?.BeginContextualDeckUICommand;
        if (typeof begin !== 'function') throw new Error('Contextual Deck API unavailable');
        if ((isEditorMermaidMutation(commandId) && observed.surfaceType !== 'editor') || Date.now() >= expires || !options.isCurrent()) throw new Error('deck-offer-stale');
        pending = begin.call(app, offerId, generation, observed);
      }
      const reserved = await pending;
      if (!reserved || reserved.commandId !== commandId || !reserved.ticket || !reserved.invocationId) {
        if (typeof reserved?.ticket === 'string' && reserved.ticket) {
          await createCommandUIExecutionWailsPort({ target }).cancelUICommand(reserved.ticket).catch(() => undefined);
        }
        throw new Error('deck-offer-command-mismatch');
      }
      return reserved;
    },
  });
}
