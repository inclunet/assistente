import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CollapsibleSection } from './CollapsibleSection';

describe('CollapsibleSection', () => {
  it('renderiza header com título', () => {
    const handleToggle = vi.fn();
    render(
      <CollapsibleSection
        title="Test Section"
        isOpen={false}
        onToggle={handleToggle}
      >
        <div>Content</div>
      </CollapsibleSection>
    );

    expect(screen.getByText('Test Section')).toBeInTheDocument();
  });

  it('renderiza conteúdo quando isOpen = true', () => {
    const handleToggle = vi.fn();
    render(
      <CollapsibleSection
        title="Test Section"
        isOpen={true}
        onToggle={handleToggle}
      >
        <div>Hidden Content</div>
      </CollapsibleSection>
    );

    expect(screen.getByText('Hidden Content')).toBeInTheDocument();
  });

  it('mantém conteúdo oculto quando isOpen = false', () => {
    const handleToggle = vi.fn();
    render(
      <CollapsibleSection
        title="Test Section"
        isOpen={false}
        onToggle={handleToggle}
      >
        <div>Hidden Content</div>
      </CollapsibleSection>
    );

    const content = screen.getByText('Hidden Content');
    expect(content).toBeInTheDocument();
    expect(content).not.toBeVisible();
    expect(screen.getByRole('region', { hidden: true })).toHaveAttribute('hidden');
  });

  it('chama onToggle ao clicar no header', async () => {
    const user = userEvent.setup();
    const handleToggle = vi.fn();
    render(
      <CollapsibleSection
        title="Test Section"
        isOpen={false}
        onToggle={handleToggle}
      >
        <div>Content</div>
      </CollapsibleSection>
    );

    const button = screen.getByRole('button');
    await user.click(button);

    expect(handleToggle).toHaveBeenCalledTimes(1);
  });

  it('renderiza badge quando fornecido', () => {
    const handleToggle = vi.fn();
    render(
      <CollapsibleSection
        title="Test Section"
        isOpen={false}
        onToggle={handleToggle}
        badge="on"
      >
        <div>Content</div>
      </CollapsibleSection>
    );

    expect(screen.getByTestId('badge-on')).toBeInTheDocument();
  });

  it('renderiza badge "off" corretamente', () => {
    const handleToggle = vi.fn();
    render(
      <CollapsibleSection
        title="Test Section"
        isOpen={false}
        onToggle={handleToggle}
        badge="off"
      >
        <div>Content</div>
      </CollapsibleSection>
    );

    expect(screen.getByTestId('badge-off')).toBeInTheDocument();
  });

  it('usa aria-expanded corretamente', () => {
    const handleToggle = vi.fn();
    const { rerender } = render(
      <CollapsibleSection
        title="Test Section"
        isOpen={false}
        onToggle={handleToggle}
      >
        <div>Content</div>
      </CollapsibleSection>
    );

    let button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-expanded', 'false');

    rerender(
      <CollapsibleSection
        title="Test Section"
        isOpen={true}
        onToggle={handleToggle}
      >
        <div>Content</div>
      </CollapsibleSection>
    );

    button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-expanded', 'true');
  });

  it('usa aria-label customizado quando fornecido', () => {
    const handleToggle = vi.fn();
    render(
      <CollapsibleSection
        title="Test Section"
        isOpen={false}
        onToggle={handleToggle}
        ariaLabel="Custom Label"
      >
        <div>Content</div>
      </CollapsibleSection>
    );

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-label', 'Custom Label');
  });
});
