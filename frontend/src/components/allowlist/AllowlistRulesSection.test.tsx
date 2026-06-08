import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { AllowlistRulesSection } from './AllowlistRulesSection';
import { allowlist } from '../../../wailsjs/go/models';

const buildItem = (overrides: Partial<allowlist.Allowlist> = {}): allowlist.Allowlist => new allowlist.Allowlist({
  name: '',
  description: '',
  auto_approve: [],
  always_deny: [],
  default_action: 'confirm',
  ...overrides,
});

describe('AllowlistRulesSection', () => {
  it('renderiza regras em múltiplas linhas', () => {
    const onRulesChange = vi.fn();
    render(
        <AllowlistRulesSection
          item={buildItem({
            auto_approve: ['ls', 'git status'],
            always_deny: ['rm -rf /'],
          })}
        onRulesChange={onRulesChange}
      />
    );

    expect(screen.getByLabelText('Auto Approve (um pattern por linha)')).toHaveValue(
      'ls\ngit status'
    );
    expect(screen.getByLabelText('Always Deny (um pattern por linha)')).toHaveValue('rm -rf /');
  });

  it('normaliza regras removendo espaços e linhas vazias', async () => {
    const onRulesChange = vi.fn();
    render(
        <AllowlistRulesSection
          item={buildItem({ auto_approve: [], always_deny: [] })}
        onRulesChange={onRulesChange}
      />
    );

    fireEvent.change(screen.getByLabelText('Auto Approve (um pattern por linha)'), {
      target: { value: '  ls  \n\n git status  \n' },
    });

    expect(onRulesChange).toHaveBeenCalledWith('auto_approve', ['ls', 'git status']);
  });

  it('atualiza lista de sempre negar', async () => {
    const onRulesChange = vi.fn();
    render(
        <AllowlistRulesSection
          item={buildItem({ auto_approve: [], always_deny: [] })}
        onRulesChange={onRulesChange}
      />
    );

    fireEvent.change(screen.getByLabelText('Always Deny (um pattern por linha)'), {
      target: { value: 'shutdown\nreboot' },
    });

    expect(onRulesChange).toHaveBeenCalledWith('always_deny', ['shutdown', 'reboot']);
  });

  it('exibe dicas de ajuda das regras', () => {
    const onRulesChange = vi.fn();
    render(
        <AllowlistRulesSection
          item={buildItem({ auto_approve: [], always_deny: [] })}
        onRulesChange={onRulesChange}
      />
    );

    expect(
      screen.getByText('Comandos aprovados sem confirmação. Use * no final para prefix match.')
    ).toBeInTheDocument();
    expect(
      screen.getByText('Comandos sempre bloqueados, mesmo que estejam em Auto Approve.')
    ).toBeInTheDocument();
  });
});
