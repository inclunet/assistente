import { create } from 'zustand';

export type EditableResource =
  | 'profiles'
  | 'providers'
  | 'credentials'
  | 'allowlists'
  | 'skills'
  | 'mcp'
  | 'channels'
  | 'memories'
  | 'tasklists';

/** Seções públicas do editor de perfil aceitas pela navegação e por deep links. */
export type ProfileEditSection = 'voice';

export interface WorkspaceNavigationCaller {
  kind: 'workspace';
  tabId: string;
  surfaceId: string;
  surfaceType: 'page' | 'embedded' | 'modal';
  conversationId: string | null;
}

export interface ResourceEditRequest {
  resource: EditableResource;
  id: string;
  action: 'edit' | 'new';
  tab?: ProfileEditSection;
  caller?: WorkspaceNavigationCaller;
  timestamp: number;
  /**
   * Valores iniciais para pré-preencher o formulário de criação.
   * Restrito a campos não-sensíveis (ex.: pattern/type de credencial) —
   * segredos (token, senhas) nunca trafegam aqui.
   */
  initial?: Record<string, string>;
}

export interface ResourceEditOptions {
  tab?: ProfileEditSection;
  caller?: WorkspaceNavigationCaller;
  initial?: Record<string, string>;
}

export function isResourceEditRequestFresh(request: ResourceEditRequest): boolean {
  return Date.now() - request.timestamp <= 5000;
}

interface NavigationState {
  pendingEdit: ResourceEditRequest | null;
  requestResourceEdit: (
    resource: EditableResource,
    id: string,
    action?: 'edit' | 'new',
    options?: ResourceEditOptions,
  ) => void;
  // An already received request may wait for page data beyond its arrival TTL.
  consumeResourceEdit: (resource: EditableResource, receivedRequest?: ResourceEditRequest) => ResourceEditRequest | null;
  clearPendingEdit: () => void;
}

export const useNavigationStore = create<NavigationState>((set, get) => ({
  pendingEdit: null,

  requestResourceEdit: (resource, id, action = 'edit', options) => {
    set({
      pendingEdit: {
        resource,
        id,
        action,
        ...(options?.tab ? { tab: options.tab } : {}),
        ...(options?.caller ? { caller: options.caller } : {}),
        ...(options?.initial ? { initial: options.initial } : {}),
        timestamp: Date.now(),
      },
    });
  },

  consumeResourceEdit: (resource, receivedRequest) => {
    const { pendingEdit } = get();
    if (!pendingEdit || pendingEdit.resource !== resource) return null;
    if (receivedRequest !== pendingEdit && !isResourceEditRequestFresh(pendingEdit)) {
      set({ pendingEdit: null });
      return null;
    }
    set({ pendingEdit: null });
    return pendingEdit;
  },

  clearPendingEdit: () => set({ pendingEdit: null }),
}));
