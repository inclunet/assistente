import { useEffect, useRef } from 'react';
import { useNavigationStore, type EditableResource, type ResourceEditRequest } from '../store/navigationStore';

/**
 * Hook for pages to consume pending resource-edit requests from deep links.
 * Calls `onEdit(id, request)` or `onNew(request)` once after mount or when a
 * new request arrives, preserving metadata such as the target tab and caller.
 */
export function useResourceEditRequest(
  resource: EditableResource,
  callbacks: {
    onEdit: (id: string, request: ResourceEditRequest) => void;
    onNew?: (request: ResourceEditRequest) => void;
    ready?: boolean;
  },
): void {
  const consumeResourceEdit = useNavigationStore((s) => s.consumeResourceEdit);
  const pending = useNavigationStore((s) => s.pendingEdit);
  const processedRef = useRef<number>(0);
  const ready = callbacks.ready ?? true;

  useEffect(() => {
    if (!ready) return;
    if (!pending || pending.resource !== resource) return;
    if (pending.timestamp <= processedRef.current) return;

    const request: ResourceEditRequest | null = consumeResourceEdit(resource);
    if (!request) return;

    processedRef.current = request.timestamp;

    if (request.action === 'new' && callbacks.onNew) {
      callbacks.onNew(request);
    } else if (request.action === 'edit' && request.id) {
      callbacks.onEdit(request.id, request);
    }
  }, [pending, ready, resource, consumeResourceEdit, callbacks]);
}
