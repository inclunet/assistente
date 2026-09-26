import { describe, expect, it } from 'vitest';
import { LOCAL_UI_COMMAND_IDS, isLocalUICommand } from './commandLocalUI';

describe('local_ui command projection', () => {
  it('mantém a allowlist fechada de navegação e efeitos locais', () => {
    expect(LOCAL_UI_COMMAND_IDS).toHaveLength(63);
    expect(isLocalUICommand('command_settings.create.open')).toBe(true);
    expect(isLocalUICommand('workspace.tab.go_to')).toBe(true);
    expect(isLocalUICommand('tasklist.task.create.open')).toBe(true);
    expect(isLocalUICommand('tasklists.clear')).toBe(false);
    for (const id of ['chat.focus.input', 'chat.focus.messages', 'chat.message.read.open', 'chat.message.menu.open', 'chat.message.reasoning.toggle', 'chat.message.thread.expand', 'chat.message.thread.collapse']) {
      expect(isLocalUICommand(id)).toBe(true);
    }
    expect(isLocalUICommand('navigation.landmark.next')).toBe(true);
    expect(isLocalUICommand('navigation.landmark.previous')).toBe(true);
    expect(isLocalUICommand('navigation.landmark.default')).toBe(true);
    expect(isLocalUICommand('editor.mermaid.open')).toBe(true);
    expect(isLocalUICommand('editor.mermaid.apply')).toBe(false);
    expect(isLocalUICommand('editor.mermaid.remove')).toBe(false);
    expect(isLocalUICommand('chat.message.edit.open')).toBe(true);
    expect(isLocalUICommand('chat.message.pin.toggle')).toBe(false);
    expect(isLocalUICommand('chat.message.delete')).toBe(false);
    expect(isLocalUICommand('chat.message.copy')).toBe(false);
    expect(isLocalUICommand('chat.pinned.open')).toBe(true);
    expect(isLocalUICommand('chat.tokens.open')).toBe(true);
    expect(isLocalUICommand('chat.conversation.clear')).toBe(false);
    expect(isLocalUICommand('editor.table.cell.next')).toBe(true);
    expect(isLocalUICommand('editor.table.cell.previous')).toBe(true);
    expect(isLocalUICommand('editor.format.table.insert')).toBe(false);
    expect(isLocalUICommand('workspace.tab.next')).toBe(true);
    expect(isLocalUICommand('navigation.settings.open')).toBe(true);
    expect(isLocalUICommand('navigation.palette.open')).toBe(true);
    expect(isLocalUICommand('navigation.data.export.open')).toBe(true);
    expect(isLocalUICommand('navigation.data.import.open')).toBe(true);
    expect(isLocalUICommand('navigation.menu.open')).toBe(true);
    expect(isLocalUICommand('help.shortcuts.show')).toBe(true);
    expect(isLocalUICommand('workspace.panel.focus')).toBe(true);
    expect(isLocalUICommand('workspace.tab.close')).toBe(false);
    expect(isLocalUICommand('workspace.tab.chat.create')).toBe(false);
    expect(isLocalUICommand('navigation.future.open')).toBe(false);
  });

});
