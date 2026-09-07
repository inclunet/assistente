import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AppErrorBoundary } from './AppErrorBoundary';

function ComponenteQueQuebra(): JSX.Element {
  throw new Error('laço de renderização');
}

describe('AppErrorBoundary', () => {
  let erroDoConsole: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    // O React sempre reporta no console o erro que um boundary capturou.
    erroDoConsole = vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    erroDoConsole.mockRestore();
  });

  it('troca a árvore quebrada pela tela de erro', () => {
    render(
      <AppErrorBoundary>
        <ComponenteQueQuebra />
      </AppErrorBoundary>,
    );

    expect(screen.getByRole('heading', { name: 'O aplicativo encontrou um erro' })).toBeInTheDocument();
    expect(screen.getByText('laço de renderização')).toBeInTheDocument();
  });

  it('guarda a árvore de componentes, que é o que nomeia o culpado', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      configurable: true,
    });

    render(
      <AppErrorBoundary>
        <ComponenteQueQuebra />
      </AppErrorBoundary>,
    );

    await userEvent.click(screen.getByText('Detalhes técnicos'));

    expect(screen.getByText(/ComponenteQueQuebra/)).toBeInTheDocument();
  });

  it('deixa a árvore passar quando não há erro', () => {
    render(
      <AppErrorBoundary>
        <p>conteúdo normal</p>
      </AppErrorBoundary>,
    );

    expect(screen.getByText('conteúdo normal')).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'O aplicativo encontrou um erro' })).not.toBeInTheDocument();
  });
});
