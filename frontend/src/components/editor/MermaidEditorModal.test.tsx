import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MermaidEditorModal } from './MermaidEditorModal';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../ui/Modal', () => ({
  Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock('../ui/CodeEditor', () => ({
  CodeEditor: () => <div data-testid="code-editor" />,
}));

vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: () => <div data-testid="preview" />,
}));

describe('MermaidEditorModal', () => {
  it('chama onApply e onCancel', () => {
    const onApply = vi.fn();
    const onCancel = vi.fn();

    render(
      <MermaidEditorModal
        isOpen={true}
        initialCode="graph TD;"
        onApply={onApply}
        onCancel={onCancel}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'editor.mermaid.applyShortcut' }));
    expect(onApply).toHaveBeenCalledWith('graph TD;');

    fireEvent.click(screen.getByRole('button', { name: 'common.cancel' }));
    expect(onCancel).toHaveBeenCalled();
  });
});
