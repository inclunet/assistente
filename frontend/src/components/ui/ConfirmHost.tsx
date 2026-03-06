import { ConfirmDialog } from './ConfirmDialog';
import { useConfirmStore } from '../../store/confirmStore';

export function ConfirmHost() {
  const active = useConfirmStore((s) => s.active);
  const confirm = useConfirmStore((s) => s.confirm);
  const cancel = useConfirmStore((s) => s.cancel);

  return (
    <ConfirmDialog
      isOpen={!!active}
      title={active?.title ?? ''}
      message={active?.message ?? ''}
      confirmText={active?.confirmText}
      cancelText={active?.cancelText}
      variant={active?.variant}
      onConfirm={confirm}
      onCancel={cancel}
    />
  );
}
