import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import RestoreDefaultsPage from './RestoreDefaultsPage';
import * as AppAPI from '@wailsjs/go/app/App';
import * as SettingsAPI from '@wailsjs/go/wailsapi/Settings';
import { app } from '@wailsjs/go/models';

const emptyCleanupResult = () =>
  app.CleanupLegacyChannelJSONResult.createFrom({
    dryRun: true,
    eligible: [],
    removed: [],
    skipped: [],
    errors: [],
    warnings: [],
  });

const { mockAddToast, mockHandleDatabaseReset, mockAnnounce } = vi.hoisted(() => ({
  mockAddToast: vi.fn(),
  mockHandleDatabaseReset: vi.fn(),
  mockAnnounce: vi.fn(),
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) =>
      ({
        'restore.pageTitle': 'Restaurar Padrões',
        'restore.description': 'Gerencie a restauração e limpeza de dados do assistente',
        'restore.sections.appearance': '🎨 Aparência',
        'restore.sections.quickActions': '⚡ Operações Rápidas',
        'restore.sections.granular': '🎛️ Limpeza Granular',
        'restore.sections.nuclear': '💣 Opções Nucleares',
        'restore.sections.security': '🔐 Segurança - Senha Mestre',
        'restore.aria.appearance': 'Aparência - escolha o tema visual',
        'restore.aria.selectTheme': 'Selecionar tema',
        'restore.aria.quickActions': 'Operações Rápidas',
        'restore.aria.granular': 'Limpeza Granular',
        'restore.aria.nuclear': 'Opções Nucleares',
        'restore.aria.security': 'Segurança',
        'restore.items.clearMessages': '🗑️ Limpar Mensagens e Conversas',
        'restore.items.clearMessagesDesc':
          'Apaga todas as mensagens e conversas, mantendo perfis e credenciais',
        'restore.items.cleanupLegacyJSON': 'Remover JSON legado de canais',
        'restore.items.cleanupLegacyJSONDesc':
          'Remove channels/*.json e contacts.json do disco após a migração para o banco.',
        'restore.buttons.clear': 'Limpar',
        'restore.buttons.cleanupLegacy': 'Remover JSON legado',
        'restore.toast.cleanupLegacyNone': 'Nenhum arquivo JSON legado elegível para remoção',
        'restore.toast.cleanupLegacyNoneRemoved':
          'Nenhum arquivo JSON legado foi removido (candidatos ausentes ou pulados na confirmação)',
        'restore.toast.cleanupLegacyPartial':
          'Remoção parcial: {{removed}} de {{expected}} arquivo(s) legado(s)',
        'restore.announce.cleanupLegacyNone': 'Nenhum arquivo JSON legado elegível',
        'restore.announce.cleanupLegacyNoneRemoved':
          'Nenhum arquivo JSON legado foi removido; os candidatos sumiram ou foram pulados na confirmação',
        'restore.announce.cleanupLegacyDone': 'JSON legado removido: {{removed}} arquivo(s). Backup em {{backup}}.',
        'restore.announce.cleanupLegacyPartial':
          'Remoção parcial: {{removed}} de {{expected}} arquivo(s). Backup em {{backup}}.',
        'restore.confirm.cleanupLegacyJSONTitle': 'Remover arquivos JSON legados de canais?',
        'restore.confirm.cleanupLegacyJSONMessage': 'Serão removidos {{count}} arquivo(s):\n{{files}}',
        'restore.confirm.lastChanceTitle': 'Confirmação final',
        'restore.confirm.lastChanceMessage': 'Última chance',
        'common.confirm': 'Confirmar',
        'common.cancel': 'Cancelar',
      } as Record<string, string>)[key] ?? key,
  }),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: mockAddToast };
    return selector ? selector(s) : s;
  },
}));
vi.mock('../store/chatStore', () => ({
  useChatStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { handleDatabaseReset: mockHandleDatabaseReset };
    return selector ? selector(s) : s;
  },
}));
vi.mock('@wailsjs/go/app/App');
vi.mock('@wailsjs/go/wailsapi/Settings');
vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: mockAnnounce,
  }),
}));

const mockConfirm = vi.fn().mockResolvedValue(true);

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => mockConfirm,
}));

describe('RestoreDefaultsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();

    vi.mocked(AppAPI.ClearMessages).mockResolvedValue(undefined);
    vi.mocked(SettingsAPI.ClearAllCredentials).mockResolvedValue(undefined);
    vi.mocked(SettingsAPI.ClearAllProfiles).mockResolvedValue(undefined);
    vi.mocked(SettingsAPI.ClearAllSkills).mockResolvedValue(undefined);
    vi.mocked(SettingsAPI.ClearAllChannels).mockResolvedValue(undefined);
    vi.mocked(AppAPI.ResetDatabase).mockResolvedValue(undefined);
    vi.mocked(AppAPI.CleanupLegacyChannelJSON).mockResolvedValue(emptyCleanupResult());

    mockConfirm.mockReset();
    mockConfirm.mockResolvedValue(true);
    mockAnnounce.mockReset();
  });

  describe('Rendering', () => {
    it('deve renderizar o título da página', () => {
      render(<RestoreDefaultsPage />);
      expect(screen.getByText(/Restaurar Padrões/i)).toBeInTheDocument();
    });

    it('deve renderizar a descrição da página', () => {
      render(<RestoreDefaultsPage />);
      expect(screen.getByText(/Gerencie a restauração/i)).toBeInTheDocument();
    });

    it('deve renderizar todas as seções colapsáveis', () => {
      render(<RestoreDefaultsPage />);
      expect(screen.getByText(/Operações Rápidas/i)).toBeInTheDocument();
      expect(screen.getByText(/Limpeza Granular/i)).toBeInTheDocument();
      expect(screen.getByText(/Opções Nucleares/i)).toBeInTheDocument();
      expect(screen.getByText(/Segurança/i)).toBeInTheDocument();
    });

    it('deve renderizar headers de colapsáveis', () => {
      render(<RestoreDefaultsPage />);
      const headers = screen.getAllByRole('button');
      expect(headers.length).toBeGreaterThanOrEqual(4);
    });
  });

  describe('Interação com seções colapsáveis', () => {
    it('deve alterar o estado de uma seção ao clicar no header', async () => {
      const user = userEvent.setup();
      render(<RestoreDefaultsPage />);

      // Encontrar o botão de colapse da seção "Limpeza Granular" (inicialmente fechada)
      const buttons = screen.getAllByRole('button');
      const header = buttons.find((btn) => btn.textContent?.includes('Limpeza Granular'));
      
      expect(header).toBeDefined();
      expect(header).toBeInTheDocument();

      // Verificar estado inicial (aria-expanded = false)
      expect(header).toHaveAttribute('aria-expanded', 'false');

      // Clicar para alternar
      await user.click(header!);

      await waitFor(() => {
        expect(header).toHaveAttribute('aria-expanded', 'true');
      });
    });
  });

  describe('Confirmação de operações', () => {
    it('deve mostrar diálogo de confirmação ao iniciar uma operação', async () => {
      const user = userEvent.setup();
      mockConfirm.mockResolvedValueOnce(false);

      render(<RestoreDefaultsPage />);

      // A seção "Operações Rápidas" começa aberta, então o botão já deve estar visível
      // Procurar pelo primeiro botão com classe 'btn' (que não é de colapse)
      const buttons = screen.getAllByRole('button');
      const actionButton = buttons.find(
        (btn) => btn.className.includes('btn') && !btn.className.includes('collapsible-section__header')
      );

      expect(actionButton).toBeDefined();

      if (actionButton) {
        await user.click(actionButton);
        expect(mockConfirm).toHaveBeenCalled();
      }
    });

    it('deve cancelar operação se confirmação for recusada', async () => {
      const user = userEvent.setup();
      mockConfirm.mockResolvedValue(false);

      render(<RestoreDefaultsPage />);

      // Encontrar o primeiro botão de ação (não de colapse)
      const buttons = screen.getAllByRole('button');
      const actionButton = buttons.find(
        (btn) => btn.className.includes('btn') && !btn.className.includes('collapsible-section__header')
      );

      if (actionButton) {
        await user.click(actionButton);
        // Nenhuma operação Wails foi executada porque confirmação foi recusada
        expect(AppAPI.ClearMessages).not.toHaveBeenCalled();
      }
    });
  });

  describe('Operações específicas', () => {
    it('deve chamar ResetDatabase para opções nucleares quando confirmado', async () => {
      const user = userEvent.setup();

      render(<RestoreDefaultsPage />);

      // Encontrar o botão para abrir a seção "Opções Nucleares"
      const buttons = screen.getAllByRole('button');
      const header = buttons.find((btn) => btn.textContent?.includes('Opções Nucleares'));
      
      // Abrir seção
      await user.click(header!);
      await waitFor(() => {
        expect(header).toHaveAttribute('aria-expanded', 'true');
      });

      // Encontrar todos os botões de ação e procurar pelo primeiro (Resetar Banco de Dados)
      const allButtons = screen.getAllByRole('button');
      const actionButtons = allButtons.filter(
        (btn) => btn.className.includes('btn') && !btn.className.includes('collapsible-section__header')
      );
      
      // O primeiro botão de ação das Opções Nucleares deve ser ResetDatabase
      if (actionButtons.length > 1) {
        await user.click(actionButtons[1]);
        await waitFor(() => {
          expect(AppAPI.ResetDatabase).toHaveBeenCalled();
        });
      }
    });

    it('deve chamar handleDatabaseReset quando ResetDatabase é executado', async () => {
      const user = userEvent.setup();

      render(<RestoreDefaultsPage />);

      // Encontrar o botão para abrir a seção "Opções Nucleares"
      const buttons = screen.getAllByRole('button');
      const header = buttons.find((btn) => btn.textContent?.includes('Opções Nucleares'));
      
      // Abrir seção
      await user.click(header!);
      await waitFor(() => {
        expect(header).toHaveAttribute('aria-expanded', 'true');
      });

      // Encontrar todos os botões de ação
      const allButtons = screen.getAllByRole('button');
      const actionButtons = allButtons.filter(
        (btn) => btn.className.includes('btn') && !btn.className.includes('collapsible-section__header')
      );
      
      // O primeiro botão de ação das Opções Nucleares deve ser ResetDatabase
      if (actionButtons.length > 1) {
        await user.click(actionButtons[1]);
        await waitFor(() => {
          expect(mockHandleDatabaseReset).toHaveBeenCalled();
        });
      }
    });

    it('deve anunciar quando não há JSON legado elegível', async () => {
      const user = userEvent.setup();
      vi.mocked(AppAPI.CleanupLegacyChannelJSON).mockResolvedValue(emptyCleanupResult());

      render(<RestoreDefaultsPage />);

      const buttons = screen.getAllByRole('button');
      const header = buttons.find((btn) => btn.textContent?.includes('Limpeza Granular'));
      await user.click(header!);
      const cleanupBtn = await screen.findByRole('button', { name: /Remover JSON legado/i });
      await user.click(cleanupBtn);

      await waitFor(() => {
        expect(AppAPI.CleanupLegacyChannelJSON).toHaveBeenCalledWith({
          confirm: false,
          noBackup: false,
        });
      });
      expect(mockAnnounce).toHaveBeenCalledWith('Nenhum arquivo JSON legado elegível');
      expect(mockConfirm).not.toHaveBeenCalled();
    });

    it('deve confirmar e remover JSON legado elegível', async () => {
      const user = userEvent.setup();
      vi.mocked(AppAPI.CleanupLegacyChannelJSON)
        .mockResolvedValueOnce(
          app.CleanupLegacyChannelJSONResult.createFrom({
            dryRun: true,
            eligible: [{ path: '/tmp/channels/telegram.json', kind: 'channel', slug: 'telegram', reason: 'ok' }],
            removed: [],
            skipped: [],
            errors: [],
            warnings: [],
          })
        )
        .mockResolvedValueOnce(
          app.CleanupLegacyChannelJSONResult.createFrom({
            dryRun: false,
            eligible: [{ path: '/tmp/channels/telegram.json', kind: 'channel', slug: 'telegram', reason: 'ok' }],
            removed: ['/tmp/channels/telegram.json'],
            backedUpTo: '/tmp/channels.legacy-backup/20260727',
            skipped: [],
            errors: [],
            warnings: [],
          })
        );
      mockConfirm.mockResolvedValue(true);

      render(<RestoreDefaultsPage />);

      const buttons = screen.getAllByRole('button');
      const header = buttons.find((btn) => btn.textContent?.includes('Limpeza Granular'));
      await user.click(header!);
      const cleanupBtn = await screen.findByRole('button', { name: /Remover JSON legado/i });
      await user.click(cleanupBtn);

      await waitFor(() => {
        expect(mockConfirm).toHaveBeenCalledTimes(2);
      });
      expect(AppAPI.CleanupLegacyChannelJSON).toHaveBeenNthCalledWith(2, {
        confirm: true,
        noBackup: false,
      });
      expect(mockAnnounce).toHaveBeenCalledWith(
        expect.stringContaining('JSON legado removido')
      );
    });

    it('deve tratar errors da API como falha', async () => {
      const user = userEvent.setup();
      vi.mocked(AppAPI.CleanupLegacyChannelJSON)
        .mockResolvedValueOnce(
          app.CleanupLegacyChannelJSONResult.createFrom({
            dryRun: true,
            eligible: [{ path: '/tmp/channels/telegram.json', kind: 'channel', slug: 'telegram', reason: 'ok' }],
            removed: [],
            skipped: [],
            errors: [],
            warnings: [],
          })
        )
        .mockRejectedValueOnce(new Error('backup falhou'));
      mockConfirm.mockResolvedValue(true);

      render(<RestoreDefaultsPage />);

      const buttons = screen.getAllByRole('button');
      const header = buttons.find((btn) => btn.textContent?.includes('Limpeza Granular'));
      await user.click(header!);
      const cleanupBtn = await screen.findByRole('button', { name: /Remover JSON legado/i });
      await user.click(cleanupBtn);

      await waitFor(() => {
        expect(mockAddToast).toHaveBeenCalledWith(
          expect.stringContaining('backup falhou'),
          'error'
        );
      });
      expect(mockAnnounce).not.toHaveBeenCalledWith(
        expect.stringContaining('JSON legado removido')
      );
    });

    it('não anuncia sucesso total quando nenhum arquivo foi removido após confirmação', async () => {
      const user = userEvent.setup();
      vi.mocked(AppAPI.CleanupLegacyChannelJSON)
        .mockResolvedValueOnce(
          app.CleanupLegacyChannelJSONResult.createFrom({
            dryRun: true,
            eligible: [{ path: '/tmp/channels/telegram.json', kind: 'channel', slug: 'telegram', reason: 'ok' }],
            removed: [],
            skipped: [],
            errors: [],
            warnings: [],
          })
        )
        .mockResolvedValueOnce(
          app.CleanupLegacyChannelJSONResult.createFrom({
            dryRun: false,
            eligible: [{ path: '/tmp/channels/telegram.json', kind: 'channel', slug: 'telegram', reason: 'ok' }],
            removed: [],
            skipped: [],
            errors: [],
            warnings: [],
          })
        );
      mockConfirm.mockResolvedValue(true);

      render(<RestoreDefaultsPage />);

      const buttons = screen.getAllByRole('button');
      const header = buttons.find((btn) => btn.textContent?.includes('Limpeza Granular'));
      await user.click(header!);
      const cleanupBtn = await screen.findByRole('button', { name: /Remover JSON legado/i });
      await user.click(cleanupBtn);

      await waitFor(() => {
        expect(mockAddToast).toHaveBeenCalledWith(
          expect.stringContaining('Nenhum arquivo JSON legado foi removido'),
          'info',
          undefined,
          undefined,
          expect.objectContaining({ suppressAnnounce: true })
        );
      });
      expect(mockAnnounce).toHaveBeenCalledWith(
        expect.stringContaining('Nenhum arquivo JSON legado foi removido')
      );
      expect(mockAddToast.mock.calls.some((call) => call[1] === 'success')).toBe(false);
    });
  });

  describe('Gerenciamento de estado', () => {
    it('deve renderizar a página sem erros múltiplas vezes', () => {
      const { rerender } = render(<RestoreDefaultsPage />);
      expect(screen.getByText(/Restaurar Padrões/i)).toBeInTheDocument();
      
      rerender(<RestoreDefaultsPage />);
      expect(screen.getByText(/Restaurar Padrões/i)).toBeInTheDocument();
    });
  });
});
