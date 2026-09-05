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
}

export interface ResourceEditOptions {
  tab?: ProfileEditSection;
  caller?: WorkspaceNavigationCaller;
}

interface NavigationState {
  pendingEdit: ResourceEditRequest | null;
  requestResourceEdit: (
    resource: EditableResource,
    id: string,
    action?: 'edit' | 'new',
    options?: ResourceEditOptions,
  ) => void;
  consumeResourceEdit: (resource: EditableResource) => ResourceEditRequest | null;
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
        timestamp: Date.now(),
      },
    });
  },

  consumeResourceEdit: (resource) => {
    const { pendingEdit } = get();
    if (!pendingEdit || pendingEdit.resource !== resource) return null;
    const staleMs = 5000;
    if (Date.now() - pendingEdit.timestamp > staleMs) {
      set({ pendingEdit: null });
      return null;
    }
    set({ pendingEdit: null });
    return pendingEdit;
  },

  clearPendingEdit: () => set({ pendingEdit: null }),
}));
