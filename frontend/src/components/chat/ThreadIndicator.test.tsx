import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ThreadIndicator } from './ThreadIndicator';

describe('ThreadIndicator', () => {
  it('nao renderiza quando nao ha filhos', () => {
    const { container } = render(
      <ThreadIndicator childCount={0} isExpanded={false} onToggle={() => {}} />
    );

    expect(container.firstChild).toBeNull();
  });

  it('chama toggle ao clicar', () => {
    const onToggle = vi.fn();
    render(
      <ThreadIndicator childCount={2} isExpanded={false} onToggle={onToggle} />
    );

    fireEvent.click(screen.getByRole('button'));
    expect(onToggle).toHaveBeenCalled();
  });

  it('entra na ordem de Tab apenas durante a leitura', () => {
    const { rerender } = render(
      <ThreadIndicator childCount={2} isExpanded={false} onToggle={() => {}} />,
    );
    expect(screen.getByRole('button')).toHaveAttribute('tabindex', '-1');

    rerender(
      <ThreadIndicator
        childCount={2}
        isExpanded={false}
        tabNavigationEnabled
        onToggle={() => {}}
      />,
    );
    expect(screen.getByRole('button')).toHaveAttribute('tabindex', '0');
  });
});
