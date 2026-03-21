import { create } from 'zustand';

export type EditableResource =
  | 'profiles'
  | 'providers'
  | 'credentials'
  | 'allowlists'
  | 'skills'
  | 'mcp'
  | 'channels'
  | 'tasklists';

export interface ResourceEditRequest {
  resource: EditableResource;
  id: string;
  action: 'edit' | 'new';
  timestamp: number;
}

interface NavigationState {
  pendingEdit: ResourceEditRequest | null;
  requestResourceEdit: (resource: EditableResource, id: string, action?: 'edit' | 'new') => void;
  consumeResourceEdit: (resource: EditableResource) => ResourceEditRequest | null;
  clearPendingEdit: () => void;
}

export const useNavigationStore = create<NavigationState>((set, get) => ({
  pendingEdit: null,

  requestResourceEdit: (resource, id, action = 'edit') => {
    set({
      pendingEdit: { resource, id, action, timestamp: Date.now() },
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
