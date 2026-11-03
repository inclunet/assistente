/** Cota compartilhada por sessão autenticada, somente para conteúdo dispensável. */
export class TimelineCacheBudget {
  private readonly lru = new Map<object, { bytes: number; evict: () => void }>();
  private bytes = 0;

  constructor(readonly maxItems = 240, readonly maxBytes = 8 * 1024 * 1024) {
    if (!Number.isSafeInteger(maxItems) || maxItems < 0 || !Number.isSafeInteger(maxBytes) || maxBytes < 0) {
      throw new Error('Invalid timeline cache budget');
    }
  }

  get usage(): { items: number; bytes: number } {
    return { items: this.lru.size, bytes: this.bytes };
  }

  pin(key: object): void {
    const entry = this.lru.get(key);
    if (entry) this.bytes -= entry.bytes;
    this.lru.delete(key);
  }

  release(key: object, bytes: number, evict: () => void): void {
    const previous = this.lru.get(key);
    this.bytes += bytes - (previous?.bytes ?? 0);
    this.lru.set(key, { bytes, evict });
    while (this.lru.size > this.maxItems || this.bytes > this.maxBytes) {
      const oldest = this.lru.entries().next().value;
      if (!oldest) break;
      this.pin(oldest[0]);
      oldest[1].evict();
    }
  }
}
