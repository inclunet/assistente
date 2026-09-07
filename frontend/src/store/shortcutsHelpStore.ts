import { create } from 'zustand';

/**
 * Estado global do painel de atalhos de teclado (KeyboardShortcutsHelp).
 *
 * O painel é único e compartilhado: pode ser aberto pelo atalho global Ctrl+?,
 * pela tecla `?` dentro de superfícies de chat ou por um item de menu. Nenhuma
 * dessas origens deve renderizar a própria instância — todas alternam este
 * estado e o painel é montado uma única vez (na Topbar).
 */
interface ShortcutsHelpState {
  isOpen: boolean;
  open: () => void;
  close: () => void;
  toggle: () => void;
}

export const useShortcutsHelpStore = create<ShortcutsHelpState>((set) => ({
  isOpen: false,
  open: () => set({ isOpen: true }),
  close: () => set({ isOpen: false }),
  toggle: () => set((state) => ({ isOpen: !state.isOpen })),
}));
