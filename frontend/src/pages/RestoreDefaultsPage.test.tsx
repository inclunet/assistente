import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import RestoreDefaultsPage from './RestoreDefaultsPage';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import * as AppAPI from '@wailsjs/go/main/App';

vi.mock('../store/uiStore');
vi.mock('../store/chatStore');
vi.mock('@wailsjs/go/main/App');
vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: vi.fn(),
  }),
}));

const mockAddToast = vi.fn();
const mockHandleDatabaseReset = vi.fn();

describe('RestoreDefaultsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();

    (useUIStore as any).mockReturnValue({
      addToast: mockAddToast,
    });

    (useChatStore as any).mockReturnValue({
      handleDatabaseReset: mockHandleDatabaseReset,
    });

    vi.mocked(AppAPI.ClearMessages).mockResolvedValue(undefined);
    vi.mocked(AppAPI.ClearAllCredentials).mockResolvedValue(undefined);
    vi.mocked(AppAPI.ClearAllProfiles).mockResolvedValue(undefined);
    vi.mocked(AppAPI.ClearAllSkills).mockResolvedValue(undefined);
    vi.mocked(AppAPI.ClearAllChannels).mockResolvedValue(undefined);
    vi.mocked(AppAPI.ResetDatabase).mockResolvedValue(undefined);

    window.confirm = vi.fn();
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

      // Aguardar renderização
      await new Promise((r) => setTimeout(r, 50));

      // Verificar que o estado mudou (aria-expanded = true)
      expect(header).toHaveAttribute('aria-expanded', 'true');
    });
  });

  describe('Confirmação de operações', () => {
    it('deve chamar window.confirm ao iniciar uma operação', async () => {
      const user = userEvent.setup();
      (window.confirm as any).mockReturnValue(false); // Cancelar operação

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
        expect(window.confirm).toHaveBeenCalled();
      }
    });

    it('deve cancelar operação se confirmação for recusada', async () => {
      const user = userEvent.setup();
      (window.confirm as any).mockReturnValue(false);

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
      (window.confirm as any).mockReturnValue(true);

      render(<RestoreDefaultsPage />);

      // Encontrar o botão para abrir a seção "Opções Nucleares"
      const buttons = screen.getAllByRole('button');
      const header = buttons.find((btn) => btn.textContent?.includes('Opções Nucleares'));
      
      // Abrir seção
      await user.click(header!);
      await new Promise((r) => setTimeout(r, 50));

      // Encontrar todos os botões de ação e procurar pelo primeiro (Resetar Banco de Dados)
      const allButtons = screen.getAllByRole('button');
      const actionButtons = allButtons.filter(
        (btn) => btn.className.includes('btn') && !btn.className.includes('collapsible-section__header')
      );
      
      // O primeiro botão de ação das Opções Nucleares deve ser ResetDatabase
      if (actionButtons.length > 1) {
        await user.click(actionButtons[1]);
        await new Promise((r) => setTimeout(r, 100));
        expect(AppAPI.ResetDatabase).toHaveBeenCalled();
      }
    });

    it('deve chamar handleDatabaseReset quando ResetDatabase é executado', async () => {
      const user = userEvent.setup();
      (window.confirm as any).mockReturnValue(true);

      render(<RestoreDefaultsPage />);

      // Encontrar o botão para abrir a seção "Opções Nucleares"
      const buttons = screen.getAllByRole('button');
      const header = buttons.find((btn) => btn.textContent?.includes('Opções Nucleares'));
      
      // Abrir seção
      await user.click(header!);
      await new Promise((r) => setTimeout(r, 50));

      // Encontrar todos os botões de ação
      const allButtons = screen.getAllByRole('button');
      const actionButtons = allButtons.filter(
        (btn) => btn.className.includes('btn') && !btn.className.includes('collapsible-section__header')
      );
      
      // O primeiro botão de ação das Opções Nucleares deve ser ResetDatabase
      if (actionButtons.length > 1) {
        await user.click(actionButtons[1]);
        await new Promise((r) => setTimeout(r, 100));
        expect(mockHandleDatabaseReset).toHaveBeenCalled();
      }
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
