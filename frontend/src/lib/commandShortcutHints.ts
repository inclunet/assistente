import { useMemo } from 'react';
import { create } from 'zustand';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { resolveLocalCommandContextualBinding, type LocalCommandKeyContext, type LocalCommandContextualBinding, type LocalCommandKeyboardMap } from './commandLocalKeyboard';
import { ReadProfileContext } from './commandContextProviders';
import { formatCommandKeyboardTrigger, serializeCommandKeyboardTrigger, type CommandKeyboardTrigger } from './commandShortcut';

// Presentation only: the accepted keyboard projection remains the authority.
// Never load another map here or substitute product defaults on failure.
const useProjection = create<{ map: LocalCommandKeyboardMap | null }>(() => ({ map: null }));

export function publishCommandShortcutHints(map: LocalCommandKeyboardMap | null): void {
  useProjection.setState({ map: map ? structuredClone(map) : null });
}

function formatHint(trigger: CommandKeyboardTrigger): string {
  return formatCommandKeyboardTrigger(trigger)
    .replace(/\bControl\b/g, 'Ctrl').replace(/\bKey([A-Z])\b/g, '$1').replace(/\bDigit([0-9])\b/g, '$1');
}

function contextualPrefix(trigger: CommandKeyboardTrigger): string {
  const prefix = trigger.version === 1 ? trigger : { version: 1 as const, ...trigger.steps[0] };
  return serializeCommandKeyboardTrigger(prefix);
}

function v1ContextualEntry(map: LocalCommandKeyboardMap, trigger: CommandKeyboardTrigger): LocalCommandContextualBinding | undefined {
  const prefix = contextualPrefix(trigger);
  return map.contextualBindings?.find(entry => entry.shortcut.version === 1 && contextualPrefix(entry.shortcut) === prefix);
}

function v1ContextualState(map: LocalCommandKeyboardMap, trigger: CommandKeyboardTrigger, surfaceType?: string, context?: LocalCommandKeyContext): 'none' | 'selected' | 'barrier' | 'no-match' {
  const entry = v1ContextualEntry(map, trigger);
  if (!entry) return 'none';
  if (!surfaceType) return 'barrier';
  const selected = resolveLocalCommandContextualBinding(entry, surfaceType, context);
  return selected.branch ? 'selected' : selected.barrier ? 'barrier' : 'no-match';
}

function sequenceAvailable(map: LocalCommandKeyboardMap, trigger: CommandKeyboardTrigger, surfaceType?: string, context?: LocalCommandKeyContext): boolean {
  if (trigger.version !== 2) return true;
  const state = v1ContextualState(map, trigger, surfaceType, context);
  return state === 'none' || state === 'no-match';
}

export function commandShortcutHints(map: LocalCommandKeyboardMap | null, surfaceType?: string, context?: LocalCommandKeyContext): ReadonlyMap<string, string> {
  const result = new Map<string, string>();
  if (!map) return result;
  const contextual = new Set((map.contextualBindings ?? [])
    .filter(entry => entry.shortcut.version === 1)
    .map(entry => contextualPrefix(entry.shortcut)));
  const bindings = map.bindings.filter(binding => !(binding.shortcut.version === 1 && contextual.has(contextualPrefix(binding.shortcut))) &&
    sequenceAvailable(map, binding.shortcut, surfaceType, context));
  for (const entry of map.contextualBindings ?? []) {
    if (entry.shortcut.version === 2) {
      const state = v1ContextualState(map, entry.shortcut, surfaceType, context);
      if (state !== 'none' && state !== 'no-match') continue;
    }
    // Unknown surface cannot announce a fallback that execution would refuse.
    const branch = surfaceType ? resolveLocalCommandContextualBinding(entry, surfaceType, context).branch : null;
    if (branch) bindings.push(branch);
  }
  for (const binding of bindings) {
    const label = formatHint(binding.shortcut);
    const previous = result.get(binding.commandId);
    if (!previous) result.set(binding.commandId, label);
    else if (!previous.split(', ').includes(label)) result.set(binding.commandId, `${previous}, ${label}`);
  }
  return result;
}

function useCurrentProjection(): LocalCommandKeyboardMap | null {
  const map = useProjection(state => state.map);
  const owner = useAuthStore(state => state.isAuthenticated && state.user
    ? JSON.stringify([state.user.userId, state.user.sessionId]) : null);
  const workspaceID = useWorkspaceStore(state => state.workspace?.id);
  return map && owner === JSON.stringify([map.ownerId, map.sessionId]) && workspaceID === map.workspaceId ? map : null;
}

function useHintContext(surfaceType?: string): LocalCommandKeyContext | undefined {
  const workspace = useWorkspaceStore(state => state.workspace);
  return useMemo(() => {
    const tab = workspace?.tabs?.find(item => item.id === workspace.activeTabId);
    if (!tab || tab.type !== surfaceType) return undefined;
    const profile = ReadProfileContext()?.slug;
    return { surfaceId: tab.id, surfaceType: tab.type, ...(profile ? { profile } : {}) };
  }, [workspace, surfaceType]);
}

export function useCommandShortcutHints(surfaceType?: string): (commandID: string) => string | undefined {
  const map = useCurrentProjection();
  const context = useHintContext(surfaceType);
  return useMemo(() => {
    const hints = commandShortcutHints(map, surfaceType, context);
    return (commandID: string) => hints.get(commandID);
  }, [map, surfaceType, context]);
}

export function useCommandShortcutHint(commandID: string, surfaceType?: string): string | undefined {
  return useCommandShortcutHints(surfaceType)(commandID);
}

/** A menu opened by a sequence advertises the prefix, not the final action. */
export function commandSequencePrefixHint(map: LocalCommandKeyboardMap | null, commandIDs: readonly string[], surfaceType?: string, context?: LocalCommandKeyContext): string | undefined {
  const prefixes = new Set<string>();
  for (const binding of map?.bindings ?? []) {
    if (binding.shortcut.version === 2 && commandIDs.includes(binding.commandId)) {
      const prefix = { version: 1 as const, ...binding.shortcut.steps[0] };
      if (!map || !sequenceAvailable(map, binding.shortcut, surfaceType, context)) continue;
      prefixes.add(formatHint(prefix));
    }
  }
  for (const entry of map?.contextualBindings ?? []) {
    if (entry.shortcut.version === 2) {
      const state = v1ContextualState(map!, entry.shortcut, surfaceType, context);
      if (state !== 'none' && state !== 'no-match') continue;
    }
    const branch = surfaceType ? resolveLocalCommandContextualBinding(entry, surfaceType, context).branch : null;
    if (branch?.shortcut.version === 2 && commandIDs.includes(branch.commandId)) {
      prefixes.add(formatHint({ version: 1, ...branch.shortcut.steps[0] }));
    }
  }
  return prefixes.size ? [...prefixes].join(', ') : undefined;
}

export function useCommandSequencePrefixHint(commandIDs: readonly string[], surfaceType?: string): string | undefined {
  return commandSequencePrefixHint(useCurrentProjection(), commandIDs, surfaceType, useHintContext(surfaceType));
}
