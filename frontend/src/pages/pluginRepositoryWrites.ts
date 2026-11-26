export const pendingRepositoryWrites = new Map<string, Promise<boolean>>();

export async function trackRepositoryWrite(session: string, pending: Promise<boolean>) {
  pendingRepositoryWrites.set(session, pending);
  try {
    return await pending;
  } finally {
    if (pendingRepositoryWrites.get(session) === pending) pendingRepositoryWrites.delete(session);
  }
}
