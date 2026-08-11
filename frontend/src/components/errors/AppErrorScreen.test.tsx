import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from '../../test/a11yAxe';
import { AppErrorScreen } from './AppErrorScreen';

/**
 * O app inteiro vive dentro de `#root[role="application"]`, que prende o leitor
 * de telas em modo de foco. Estes testes existem porque uma tela de erro que o
 * leitor não lê é uma tela de erro inútil.
 */
function montarRoot(): HTMLElement {
  const root = document.createElement('div');
  root.id = 'root';
  root.setAttribute('role', 'application');
  root.setAttribute('aria-label', 'Assistente IA');
  document.body.appendChild(root);
  return root;
}

describe('AppErrorScreen', () => {
  let root: HTMLElement;
  let writeText: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    root = montarRoot();
    writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });
  });

  afterEach(() => {
    root.remove();
  });

  it('devolve o documento à navegação do leitor de telas enquanto está no ar', () => {
    const { unmount } = render(<AppErrorScreen error={new Error('quebrou')} />, { container: root });

    expect(root.hasAttribute('role')).toBe(false);
    expect(root.hasAttribute('aria-label')).toBe(false);

    unmount();

    expect(root.getAttribute('role')).toBe('application');
    expect(root.getAttribute('aria-label')).toBe('Assistente IA');
  });

  it('leva o foco para o texto do erro, para o leitor começar a ler por ele', () => {
    render(<AppErrorScreen error={new Error('quebrou')} />, { container: root });

    const regiao = screen.getByRole('main');
    expect(document.activeElement).toBe(regiao);
    expect(regiao).toHaveAccessibleName('O aplicativo encontrou um erro');
  });

  it('mostra a mensagem do erro e a árvore de componentes', async () => {
    render(
      <AppErrorScreen error={new Error('quebrou')} componentStack={'\n    at WorkspaceContent'} />,
      { container: root },
    );

    expect(screen.getByText('quebrou')).toBeInTheDocument();

    await userEvent.click(screen.getByText('Detalhes técnicos'));
    expect(screen.getByText(/at WorkspaceContent/)).toBeInTheDocument();
  });

  it('copia os detalhes e avisa em live region', async () => {
    render(<AppErrorScreen error={new Error('quebrou')} componentStack={'\n    at App'} />, { container: root });

    await userEvent.click(screen.getByRole('button', { name: 'Copiar detalhes' }));

    expect(writeText).toHaveBeenCalledTimes(1);
    expect(writeText.mock.calls[0][0]).toContain('quebrou');
    expect(writeText.mock.calls[0][0]).toContain('at App');
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Detalhes copiados para a área de transferência.',
    );
  });

  it('avisa quando a cópia falha, em vez de silenciar', async () => {
    writeText.mockRejectedValue(new Error('sem permissão'));
    render(<AppErrorScreen error={new Error('quebrou')} />, { container: root });

    await userEvent.click(screen.getByRole('button', { name: 'Copiar detalhes' }));

    expect(await screen.findByRole('status')).toHaveTextContent(/Não foi possível copiar/);
  });

  it('descreve erros que não são Error', () => {
    render(<AppErrorScreen error="falha crua" />, { container: root });

    expect(screen.getByText('falha crua')).toBeInTheDocument();
  });

  it('não deixa a mensagem vazia quando o erro não serializa em JSON', () => {
    // JSON.stringify(undefined) === undefined: sem o fallback, a mensagem sumiria.
    render(<AppErrorScreen error={undefined} />, { container: root });

    const heading = screen.getByRole('heading', { level: 1 });
    const mensagem = heading.parentElement?.querySelector('.app-error__message');
    expect(mensagem?.textContent).toBe('undefined');
  });

  it('não tem violações de acessibilidade', async () => {
    const { container } = render(<AppErrorScreen error={new Error('quebrou')} />, { container: root });

    expect(await axe(container)).toHaveNoViolations();
  });
});
