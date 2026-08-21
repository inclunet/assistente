import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ReasoningSection } from './ReasoningSection';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}));

describe('ReasoningSection', () => {
  it('nao renderiza sem reasoning', () => {
    render(<ReasoningSection reasoning="" />);
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('dispara toggle ao clicar', () => {
    const onToggle = vi.fn();

    render(
      <ReasoningSection
        reasoning="texto"
        isExpanded={false}
        onToggle={onToggle}
      />
    );

    fireEvent.click(screen.getByRole('button'));
    expect(onToggle).toHaveBeenCalled();
  });

  it('torna o controle focável somente durante a leitura', () => {
    const { rerender } = render(
      <ReasoningSection reasoning="texto" isExpanded={false} />,
    );
    expect(screen.getByRole('button')).toHaveAttribute('tabindex', '-1');

    rerender(
      <ReasoningSection
        reasoning="texto"
        isExpanded={false}
        tabNavigationEnabled
      />,
    );
    expect(screen.getByRole('button')).toHaveAttribute('tabindex', '0');
  });
});
