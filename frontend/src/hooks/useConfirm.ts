import type { ConfirmOptions } from '../store/confirmStore';
import { requestConfirm } from '../store/confirmStore';

export function useConfirm() {
  return (options: ConfirmOptions) => requestConfirm(options);
}
