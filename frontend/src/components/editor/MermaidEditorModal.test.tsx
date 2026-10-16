import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, render, screen, fireEvent } from '@testing-library/react';
import { MermaidEditorModal } from './MermaidEditorModal';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const state = vi.hoisted(() => ({ topID: 'mermaid', focus: vi.fn(), edit: vi.fn(), selection: vi.fn(), delayed: false, mounts: [] as Array<() => void> }));
vi.mock('../../lib/modalRegistry', () => ({ getModalRegistrySnapshot: () => ({ topID: state.topID }) }));
vi.mock('../ui/Modal', () => ({
  useModalId: () => 'mermaid',
  useModalIsTopmost: () => isTopmost,
  Modal: ({ children, isOpen }: { children: React.ReactNode; isOpen: boolean }) => isOpen ? <div data-modal-id="mermaid">{children}</div> : null,
}));
const isTopmost = () => state.topID === 'mermaid';

vi.mock('../ui/CodeEditor', async () => {
  const { useLayoutEffect } = await import('react');
  const range = { startLineNumber: 1, startColumn: 1, endLineNumber: 1, endColumn: 1 };
  const model = { getLineCount: () => 1, getLineMaxColumn: () => 10, getFullModelRange: () => range };
  const editor = { getModel: () => model, getSelection: () => range, executeEdits: state.edit, focus: state.focus, setSelection: state.selection };
  return { CodeEditor: ({ onMount, onChange, value }: { onMount: (editor: unknown, monaco: unknown) => void; onChange: (code: string) => void; value: string }) => {
    useLayoutEffect(() => { const mount = () => onMount(editor, {}); state.mounts.push(mount); if (!state.delayed) mount(); }, []);
    return <textarea data-testid="code-editor" value={value} onChange={event => onChange(event.target.value)} />;
  } };
});

let frames: FrameRequestCallback[];
beforeEach(() => {
  frames = []; state.topID = 'mermaid'; state.delayed = false; state.mounts = [];
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { frames.push(callback); return frames.length; });
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {});
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.clearAllMocks(); });
function flushFrames() { act(() => frames.splice(0).forEach(callback => callback(0))); }
const defaults = { commandOwnerKey: 'owner-a', onModalIdChange: vi.fn(), onDraftChange: vi.fn(), onCommand: vi.fn(), onCancel: vi.fn(), initialCode: 'graph TD;', isOpen: true };

vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: () => <div data-testid="preview" />,
}));

describe('MermaidEditorModal', () => {
  it('solicita apply pelo comando e chama onCancel', () => {
    const onApply = vi.fn();
    const onCancel = vi.fn();

    render(
      <MermaidEditorModal
        isOpen={true}
        initialCode="graph TD;"
        commandOwnerKey="owner-a"
        onModalIdChange={vi.fn()}
        onDraftChange={vi.fn()}
        onCommand={onApply}
        onCancel={onCancel}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'editor.mermaid.applyShortcut' }));
    expect(onApply).toHaveBeenCalledWith('apply', 'graph TD;');

    fireEvent.click(screen.getByRole('button', { name: 'common.cancel' }));
    expect(onCancel).toHaveBeenCalled();
  });

  it('coloca Aplicar antes de Cancelar no DOM (AEP-0090)', () => {
    render(
      <MermaidEditorModal
        isOpen={true}
        initialCode="graph TD;"
        commandOwnerKey="owner-a"
        onModalIdChange={vi.fn()}
        onDraftChange={vi.fn()}
        onCommand={vi.fn()}
        onCancel={vi.fn()}
      />
    );

    const actions = document.querySelector('[data-dialog-actions]');
    expect(actions).not.toBeNull();
    expect(Array.from(actions!.querySelectorAll('button')).map((b) => b.textContent)).toEqual([
      'editor.mermaid.applyShortcut',
      'common.cancel',
    ]);
  });

  it.each(['cancel', 'close', 'unmount', 'owner'] as const)('invalida rAF pendente em %s', mode => {
    const consume = vi.fn();
    const view = render(<MermaidEditorModal {...defaults} initialInsertText="x" onConsumeInsertText={consume} />);
    const oldFrames = frames.splice(0);
    if (mode === 'cancel') fireEvent.click(screen.getByRole('button', { name: 'common.cancel' }));
    if (mode === 'close') view.rerender(<MermaidEditorModal {...defaults} isOpen={false} />);
    if (mode === 'unmount') view.unmount();
    if (mode === 'owner') view.rerender(<MermaidEditorModal {...defaults} commandOwnerKey="owner-b" />);
    act(() => oldFrames.forEach(callback => callback(0)));
    expect(state.focus).not.toHaveBeenCalled(); expect(state.edit).not.toHaveBeenCalled(); expect(consume).not.toHaveBeenCalled();
  });

  it('reabertura não recebe inserção antiga e consome a nova uma única vez', () => {
    const consume = vi.fn();
    const view = render(<MermaidEditorModal {...defaults} initialInsertText="a" onConsumeInsertText={consume} />);
    view.rerender(<MermaidEditorModal {...defaults} isOpen={false} />);
    view.rerender(<MermaidEditorModal {...defaults} initialInsertText="b" onConsumeInsertText={consume} />);
    flushFrames();
    expect(state.edit).toHaveBeenCalledTimes(1);
    expect(state.edit.mock.calls[0][1][0].text).toBe('b'); expect(consume).toHaveBeenCalledTimes(1);
    view.rerender(<MermaidEditorModal {...defaults} initialInsertText="b" onConsumeInsertText={() => consume()} />);
    flushFrames(); expect(state.edit).toHaveBeenCalledTimes(1);
  });

  it.each([{ key: 's', ctrlKey: true }, { key: 's', metaKey: true }, { key: 'Enter', ctrlKey: true }])('atalho $key usa comando e recusa repeat/IME/consumido/modal sobreposto', shortcut => {
    const onCommand = vi.fn(); render(<MermaidEditorModal {...defaults} onCommand={onCommand} />);
    const editor = screen.getByTestId('code-editor');
    for (const guard of [{ repeat: true }, { isComposing: true }, { keyCode: 229 }]) fireEvent.keyDown(editor, { ...shortcut, ...guard });
    const consumed = new KeyboardEvent('keydown', { ...shortcut, bubbles: true, cancelable: true }); consumed.preventDefault(); fireEvent(editor, consumed);
    state.topID = 'other'; fireEvent.keyDown(editor, shortcut); flushFrames();
    expect(state.focus).not.toHaveBeenCalled(); expect(onCommand).not.toHaveBeenCalled();
    state.topID = 'mermaid'; fireEvent.keyDown(editor, shortcut);
    expect(onCommand).toHaveBeenCalledExactlyOnceWith('apply', 'graph TD;', { version: 1, code: shortcut.key === 's' ? 'KeyS' : 'Enter', modifiers: 'metaKey' in shortcut ? ['Meta'] : ['Control'] });
  });

  it('publica rascunho e remove somente quando a capacidade existe', () => {
    const onDraftChange = vi.fn(); const onCommand = vi.fn();
    const view = render(<MermaidEditorModal {...defaults} onDraftChange={onDraftChange} onCommand={onCommand} />);
    expect(screen.queryByRole('button', { name: 'editor.mermaid.removeBlock' })).toBeNull();
    fireEvent.change(screen.getByTestId('code-editor'), { target: { value: 'novo' } });
    expect(onDraftChange).toHaveBeenCalledExactlyOnceWith('novo');
    view.rerender(<MermaidEditorModal {...defaults} canRemove onCommand={onCommand} />);
    fireEvent.click(screen.getByRole('button', { name: 'editor.mermaid.removeBlock' }));
    expect(onCommand).toHaveBeenCalledExactlyOnceWith('remove', 'novo');
  });

  it('publica identidade e limpa a identidade anterior ao trocar owner ou desmontar', () => {
    const onModalIdChange = vi.fn();
    const view = render(<MermaidEditorModal {...defaults} onModalIdChange={onModalIdChange} />);
    expect(onModalIdChange.mock.calls).toEqual([['mermaid']]);
    view.rerender(<MermaidEditorModal {...defaults} commandOwnerKey="owner-b" onModalIdChange={onModalIdChange} />);
    expect(onModalIdChange.mock.calls).toEqual([['mermaid'], [null], ['mermaid']]);
    view.unmount(); expect(onModalIdChange).toHaveBeenLastCalledWith(null);
  });

  it('AltGraph não emite comando e tentativa bloqueada não propaga ao fundo', () => {
    const onCommand = vi.fn(); const background = vi.fn();
    render(<div onKeyDown={background}><MermaidEditorModal {...defaults} onCommand={onCommand} /></div>);
    const event = new KeyboardEvent('keydown', { key: 's', ctrlKey: true, bubbles: true, cancelable: true });
    Object.defineProperty(event, 'getModifierState', { value: (modifier: string) => modifier === 'AltGraph' });
    fireEvent(screen.getByTestId('code-editor'), event);
    expect(onCommand).not.toHaveBeenCalled(); expect(background).not.toHaveBeenCalled();
  });

  it('aguarda Monaco lazy e aplica inserção somente uma vez após onMount', () => {
    state.delayed = true; const consume = vi.fn();
    render(<MermaidEditorModal {...defaults} initialInsertText="x" onConsumeInsertText={consume} />);
    flushFrames(); expect(state.edit).not.toHaveBeenCalled();
    act(() => state.mounts[0]()); flushFrames();
    expect(state.focus).toHaveBeenCalledTimes(1); expect(state.edit).toHaveBeenCalledTimes(1); expect(consume).toHaveBeenCalledTimes(1);
    act(() => state.mounts[0]()); flushFrames(); expect(state.edit).toHaveBeenCalledTimes(1);
  });

  it('ignora onMount atrasado da abertura anterior e após desmontar', () => {
    state.delayed = true;
    const view = render(<MermaidEditorModal {...defaults} initialInsertText="a" />);
    const oldMount = state.mounts[0];
    view.rerender(<MermaidEditorModal {...defaults} isOpen={false} />);
    view.rerender(<MermaidEditorModal {...defaults} initialInsertText="b" />);
    act(() => oldMount()); flushFrames(); expect(state.edit).not.toHaveBeenCalled();
    const currentMount = state.mounts[1];
    act(() => currentMount()); flushFrames(); expect(state.edit).toHaveBeenCalledTimes(1);
    expect(state.edit.mock.calls[0][1][0].text).toBe('b');
    view.unmount(); act(() => currentMount()); flushFrames(); expect(state.edit).toHaveBeenCalledTimes(1);
  });
});
