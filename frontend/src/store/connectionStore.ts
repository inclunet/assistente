import { create } from 'zustand';
import type { ConnectionState, ConnectionStatusPayload } from '../types/connection';

interface ConnectionStoreState {
  /** Último status recebido do backend, ou null se nenhum ainda. */
  status: ConnectionStatusPayload | null;
  /** Estado atual derivado (inclui `unknown` antes da primeira verificação). */
  state: ConnectionState;
  setStatus: (status: ConnectionStatusPayload) => void;
  reset: () => void;
}

/**
 * Store global do status de conexão com a API LLM. É alimentada por uma única
 * assinatura do evento `llm:connection-status` (ver useConnectionStatusListener)
 * para que o indicador da topbar persista o estado entre remounts.
 */
export const useConnectionStore = create<ConnectionStoreState>((set) => ({
  status: null,
  state: 'unknown',
  setStatus: (status) => set({ status, state: status.state }),
  reset: () => set({ status: null, state: 'unknown' }),
}));
