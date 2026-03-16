import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MermaidCodeBlockNodeView } from './MermaidCodeBlockNodeView';
import type { NodeViewProps } from '@tiptap/react';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@tiptap/react', () => ({
  NodeViewWrapper: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  NodeViewContent: () => <div data-testid="node-content" />,
}));

vi.mock('../../store/questionnaireUIStore', () => ({
  useQuestionnaireUIStore: () => ({ request: () => Promise.resolve({ cancelled: true, answers: {} }) }),
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: () => ({ addToast: vi.fn() }),
}));

describe('MermaidCodeBlockNodeView', () => {
  it('chama onRequestEditMermaid ao clicar em editar', () => {
    const requestEditSpy = vi.fn();

    render(
      <MermaidCodeBlockNodeView
        node={{ attrs: { language: 'mermaid', mermaidBlockId: 'm1' }, textContent: 'graph TD' } as unknown as NodeViewProps['node']}
        editor={{ commands: { command: vi.fn() } } as unknown as NodeViewProps['editor']}
        getPos={() => 1}
        extension={{ options: { onRequestEditMermaid: requestEditSpy } } as unknown as NodeViewProps['extension']}
        decorations={[]}
        view={{} as NodeViewProps['view']}
        innerDecorations={{} as NodeViewProps['innerDecorations']}
        selected={false}
        updateAttributes={vi.fn()}
        deleteNode={vi.fn()}
        HTMLAttributes={{}}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'editor.mermaid.editDiagram' }));
    expect(requestEditSpy).toHaveBeenCalled();
  });
});
