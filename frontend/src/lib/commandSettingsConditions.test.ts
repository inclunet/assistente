import { describe, expect, it } from 'vitest';
import { commandConditionSupportedByOrigin, commandConditionTargetID, reconcileCommandSurfaceCondition } from './commandSettingsConditions';
import type { CommandCondition } from '../types/commandSettingsTypes';
import { WORKSPACE_MUTATION_COMMAND_IDS } from './commandContextualBackendExecution';
import { CHAT_CLEAR_COMMAND } from './commandChatClear';
import { TERMINAL_INTERRUPT_COMMAND } from './commandTerminalOperation';

const condition = (clauses: CommandCondition['clauses']): CommandCondition => ({ version: 1, clauses });
const tabs = [{ id: 'chat-one', type: 'chat' }, { id: 'editor-one', type: 'editor' }];
describe('condições operacionais por origem', () => {
  it('limita app.page às origens com captura de rota UI confiável', () => {
    expect(commandConditionSupportedByOrigin('keyboard.local', 'app.page', false)).toBe(true);
    expect(commandConditionSupportedByOrigin('palette', 'app.page', true)).toBe(true);
    expect(commandConditionSupportedByOrigin('palette', 'app.page', false, 'tasklists.delete')).toBe(true);
    expect(commandConditionSupportedByOrigin('streamdeck.key', 'app.page', true)).toBe(true);
    expect(commandConditionSupportedByOrigin('streamdeck.key', 'app.page', false, 'profiles.delete')).toBe(true);
    expect(commandConditionSupportedByOrigin('streamdeck.key', 'app.page', false, 'workspace.list')).toBe(false);
    expect(commandConditionSupportedByOrigin('keyboard.global', 'app.page', false)).toBe(false);
  });
  it.each(['editor.mermaid.apply', 'editor.mermaid.remove'])('oferece quatro campos visuais Deck para %s', (id) => {
    for (const field of ['app.focused', 'app.page', 'surface.type', 'surface.id', 'profile']) {
      expect(commandConditionSupportedByOrigin('streamdeck.key', field, false, id)).toBe(true);
    }
    for (const field of ['foreground.process', 'device', 'unknown']) {
      expect(commandConditionSupportedByOrigin('streamdeck.key', field, false, id)).toBe(false);
    }
  });
  it.each(['editor.mermaid.open', 'profiles.create', 'tasklists.update'])('não amplia o Deck visual para %s', (id) => {
    for (const field of ['app.focused', 'surface.type', 'surface.id']) {
      expect(commandConditionSupportedByOrigin('streamdeck.key', field, false, id)).toBe(false);
    }
  });
  it.each(['layer.activate', 'layer.toggle', 'layer.back'])('oferece os quatro campos na paleta e Deck para %s sem ampliar global', (id) => {
    const target = commandConditionTargetID('palette', '', JSON.stringify({ version: 1, selection: id }), true);
    expect(target).toBe(id);
    for (const field of ['app.focused', 'surface.type', 'surface.id', 'profile']) {
      expect(commandConditionSupportedByOrigin('palette', field, false, target)).toBe(true);
      expect(commandConditionSupportedByOrigin('streamdeck.key', field, false, id)).toBe(true);
    }
    for (const field of ['foreground.process', 'device']) {
      expect(commandConditionSupportedByOrigin('palette', field, false, target)).toBe(false);
      expect(commandConditionSupportedByOrigin('streamdeck.key', field, false, id)).toBe(true);
    }
    for (const origin of ['keyboard.global']) {
      for (const field of ['app.focused', 'surface.type', 'surface.id']) {
        expect(commandConditionSupportedByOrigin(origin, field, false, id)).toBe(false);
      }
    }
  });
  it.each(['tasklists.duplicate', 'tasklists.delete', 'tasklists.clear', 'profiles.duplicate', 'profiles.delete', 'profiles.activate'])('limita página %s a foco/tipo/perfil inclusive em supressão', (id) => {
    const target = commandConditionTargetID('palette', '', JSON.stringify({ version: 1, selection: id }), true);
    expect(target).toBe(id);
    for (const field of ['app.focused', 'app.page', 'surface.type', 'profile']) {
      expect(commandConditionSupportedByOrigin('palette', field, false, target)).toBe(true);
      expect(commandConditionSupportedByOrigin('streamdeck.key', field, false, id)).toBe(true);
    }
    for (const field of ['surface.id', 'foreground.process', 'device']) {
      expect(commandConditionSupportedByOrigin('palette', field, false, target)).toBe(false);
      expect(commandConditionSupportedByOrigin('streamdeck.key', field, false, id)).toBe(false);
    }
    expect(commandConditionTargetID('streamdeck.key', '', JSON.stringify({ version: 1, selection: id }), true)).toBe('');
  });
  it.each(['tasklists.create', 'tasklists.update', 'profiles.create', 'profiles.update'])('não amplia comando fora da porta de páginas: %s', (id) => {
    for (const field of ['app.focused', 'surface.type', 'surface.id']) {
      expect(commandConditionSupportedByOrigin('palette', field, false, id)).toBe(false);
    }
  });
  it('limita a ampliação da paleta às mutações do percurso contextual de workspace', () => {
    for (const id of [...WORKSPACE_MUTATION_COMMAND_IDS, CHAT_CLEAR_COMMAND, TERMINAL_INTERRUPT_COMMAND, 'terminal.session.create', 'terminal.session.close', 'chat.message.send', 'chat.message.copy', 'chat.message.delete', 'editor.format.bold', 'editor.file.save']) {
      for (const field of ['app.focused', 'surface.type', 'surface.id', 'profile']) {
        expect(commandConditionSupportedByOrigin('palette', field, false, id)).toBe(true);
      }
      for (const field of ['app.focused', 'surface.type', 'surface.id', 'profile']) {
        expect(commandConditionSupportedByOrigin('streamdeck.key', field, false, id)).toBe(true);
      }
      for (const field of ['foreground.process', 'device']) {
        expect(commandConditionSupportedByOrigin('streamdeck.key', field, false, id)).toBe(false);
      }
      expect(commandConditionSupportedByOrigin('palette', 'foreground.process', false, id)).toBe(false);
    }
    for (const id of ['workspace.list', 'profiles.delete', 'tasklists.create']) {
      expect(commandConditionSupportedByOrigin('palette', 'surface.id', false, id)).toBe(false);
      expect(commandConditionSupportedByOrigin('palette', 'profile', false, id)).toBe(true);
    }
  });
  it('deriva o alvo de supressão somente da seleção válida da paleta', () => {
    expect(commandConditionTargetID('palette', '', '{"version":1,"selection":"navigation.settings.open"}', true)).toBe('navigation.settings.open');
    for (const raw of ['null', '[]', '{', '{"version":2,"selection":"navigation.settings.open"}', '{"version":1,"selection":"navigation.settings.open","extra":true}']) {
      expect(commandConditionTargetID('palette', '', raw, true)).toBe('');
    }
    expect(commandConditionTargetID('streamdeck.key', '', '{"version":1,"selection":"navigation.settings.open"}', true)).toBe('');
    expect(commandConditionTargetID('palette', 'workspace.list', '{}', false)).toBe('workspace.list');
  });
  it('oferece condições visuais na paleta somente para apresentação local', () => {
    for (const field of ['app.focused', 'app.page', 'surface.type', 'surface.id', 'profile']) {
      expect(commandConditionSupportedByOrigin('palette', field, true)).toBe(true);
      expect(commandConditionSupportedByOrigin('streamdeck.key', field, true)).toBe(true);
      expect(commandConditionSupportedByOrigin('keyboard.global', field, true)).toBe(false);
    }
    for (const field of ['app.focused', 'surface.type', 'surface.id']) {
      expect(commandConditionSupportedByOrigin('palette', field, false)).toBe(false);
    }
    for (const field of ['foreground.process', 'device', 'unknown']) {
      expect(commandConditionSupportedByOrigin('palette', field, true)).toBe(false);
      expect(commandConditionSupportedByOrigin('streamdeck.key', field, true)).toBe(false);
    }
    expect(commandConditionSupportedByOrigin('palette', 'profile', false)).toBe(true);
  });
  it('publica fatos físicos somente nas origens com captura autoritativa', () => {
    expect(commandConditionSupportedByOrigin('keyboard.global', 'foreground.process', false)).toBe(true);
    expect(commandConditionSupportedByOrigin('streamdeck.key', 'foreground.process', false)).toBe(true);
    expect(commandConditionSupportedByOrigin('streamdeck.key', 'device', false)).toBe(true);
    for (const source of ['keyboard.local', 'palette', 'keyboard.global']) {
      expect(commandConditionSupportedByOrigin(source, 'device', false)).toBe(false);
    }
    for (const source of ['keyboard.local', 'palette']) {
      expect(commandConditionSupportedByOrigin(source, 'foreground.process', false)).toBe(false);
    }
    expect(commandConditionSupportedByOrigin('streamdeck.key', 'foreground.process', true)).toBe(false);
    expect(commandConditionSupportedByOrigin('streamdeck.key', 'foreground.process', false)).toBe(true);
    expect(commandConditionSupportedByOrigin('keyboard.local', 'profile', true)).toBe(true);
  });
});
describe('reconcileCommandSurfaceCondition', () => {
  it('seleciona tipo canônico da aba e preserva outras restrições', () => {
    const before = condition([{ field: 'app.focused', value: true }, { field: 'surface.type', value: 'editor' }]);
    const next = condition([...before.clauses, { field: 'surface.id', value: 'chat-one' }]);
    expect(reconcileCommandSurfaceCondition(before, next, tabs)).toEqual(condition([
      { field: 'app.focused', value: true }, { field: 'surface.id', value: 'chat-one' }, { field: 'surface.type', value: 'chat' },
    ]));
    expect(next.clauses[1].value).toBe('editor');
  });
  it('preserva a aba ausente ao editar outra condição', () => {
    const before = condition([{ field: 'surface.type', value: 'chat' }, { field: 'surface.id', value: 'absent' }]);
    const next = condition([...before.clauses, { field: 'app.focused', value: true }]);
    expect(reconcileCommandSurfaceCondition(before, next, tabs)).toBe(next);
  });
  it('retira referência incompatível ao usuário trocar ou remover tipo', () => {
    const before = condition([{ field: 'surface.type', value: 'chat' }, { field: 'surface.id', value: 'chat-one' }]);
    const next = condition([{ field: 'surface.type', value: 'editor' }, { field: 'surface.id', value: 'chat-one' }]);
    expect(reconcileCommandSurfaceCondition(before, next, tabs)).toEqual(condition([{ field: 'surface.type', value: 'editor' }]));
    expect(reconcileCommandSurfaceCondition(before, condition([before.clauses[1]]), tabs)).toEqual(condition([]));
  });
});
