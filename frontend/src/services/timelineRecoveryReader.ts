import type { apidto } from '@wailsjs/go/models';
import type { TimelineRecoveryReference } from '../types/chatTimelineEvents';

export interface TimelineRecoveryTransport {
  readRecovery(windowId: string, reference: TimelineRecoveryReference, cursor: string): Promise<apidto.TimelineRecoveryPage>;
  reserveRecovery(windowId: string, reference: TimelineRecoveryReference, version: string): Promise<apidto.TimelineRecoveryReservation>;
  releaseRecovery(windowId: string, reference: TimelineRecoveryReference, version: string): Promise<void>;
}

export interface TimelineRecoveryHandle {
  /** Uma página por chamada; undefined encerra a leitura, mas não libera a reserva. */
  next(): Promise<apidto.TimelineRecoveryPage | undefined>;
  close(): Promise<void>;
}

const encoder = new TextEncoder();
const maxBytes = 8 * 1024 * 1024;
const validVersion = (value: unknown): value is string => typeof value === 'string' &&
  /^(0|[1-9][0-9]{0,19})$/.test(value) && BigInt(value) <= 18446744073709551615n;
const sameReference = (a: TimelineRecoveryReference | undefined, b: TimelineRecoveryReference) => !!a &&
  a.projectionId === b.projectionId && a.incarnation === b.incarnation &&
  a.turnId === b.turnId && a.executionId === b.executionId;
const invalid = () => new Error('Invalid timeline recovery response');

interface Position { iteration: number; generation: string; part: number; field: number; offset: number }

/** Guarda somente a primeira página e metadados limitados de validação.
 * Não elege conteúdo visual nem confirma ownership de um item inteiro.
 */
export class TimelineRecoveryReader implements TimelineRecoveryHandle {
  private readonly reference: TimelineRecoveryReference;
  private first?: apidto.TimelineRecoveryPage;
  private version?: string;
  private cursor = '';
  private pages = 0;
  private bytes = 0;
  private last?: Position;
  private readonly cursors = new Set<string>();
  private pending?: Promise<unknown>;
  private reading = false;
  private closed = false;
  private attempted = false;
  private closing?: Promise<void>;

  constructor(
    private readonly transport: TimelineRecoveryTransport,
    private readonly conversationId: string,
    private readonly windowId: string,
    reference: TimelineRecoveryReference,
    private readonly active: () => boolean,
    private readonly closeWindow: () => Promise<void>,
    private readonly released: () => void,
  ) {
    this.reference = Object.freeze({ ...reference });
    const ids = [conversationId, reference.projectionId, reference.incarnation, reference.turnId, reference.executionId];
    if (ids.some(id => typeof id !== 'string' || !id || id.length > 1024) ||
      ids.reduce((size, id) => size + encoder.encode(id).length, 0) > 1024) throw invalid();
  }

  async open(): Promise<void> {
    try {
      this.assertReadable();
      const page = await this.call(() => this.transport.readRecovery(this.windowId, this.reference, ''));
      this.assertReadable();
      this.validate(page);
      this.version = page.contentVersion;
      // Uma rejeição não prova que o servidor deixou de reservar.
      this.attempted = true;
      const receipt = await this.call(() => this.transport.reserveRecovery(this.windowId, this.reference, this.version!));
      this.assertReadable();
      if (!receipt || receipt.conversationId !== this.conversationId ||
        !sameReference(receipt.reference, this.reference) || receipt.contentVersion !== this.version) throw invalid();
      this.first = page;
    } catch (error) { await this.fail(error); }
  }

  async next(): Promise<apidto.TimelineRecoveryPage | undefined> {
    this.assertReadable();
    if (this.reading || !this.version) throw new Error('Timeline recovery reader busy or not initialized');
    this.reading = true;
    try {
      if (this.first) {
        const page = this.first;
        this.first = undefined;
        return page;
      }
      if (!this.cursor) return undefined;
      if (this.pages >= 256) throw invalid();
      const page = await this.call(() => this.transport.readRecovery(this.windowId, this.reference, this.cursor));
      this.assertReadable();
      this.validate(page);
      return page;
    } catch (error) {
      return await this.fail(error);
    } finally { this.reading = false; }
  }

  invalidate(): void { this.closed = true; this.first = undefined; }

  windowClosed(): void {
    this.invalidate();
    this.attempted = false;
    this.cursors.clear();
    this.released();
  }

  close(): Promise<void> {
    this.invalidate();
    return this.closing ??= (async () => {
      if (!this.active()) {
        await this.closeWindow();
      } else {
        // Se Close coincide com Reserve, não enviar Release antes da aquisição.
        await this.pending?.catch(() => {});
        if (!this.active()) await this.closeWindow();
        else if (this.attempted) await this.call(() =>
          this.transport.releaseRecovery(this.windowId, this.reference, this.version!));
      }
      this.attempted = false;
      this.cursors.clear();
      this.released();
    })().catch(error => {
      this.closing = undefined; // retry explícito conserva referência e versão.
      throw error;
    });
  }

  private async call<T>(action: () => Promise<T>): Promise<T> {
    const pending = Promise.resolve().then(action);
    this.pending = pending;
    try { return await pending; }
    finally { if (this.pending === pending) this.pending = undefined; }
  }

  private assertReadable() {
    if (this.closed || !this.active()) throw new Error('Timeline recovery capability closed');
  }

  private async fail(error: unknown): Promise<never> {
    try { await this.close(); }
    catch (cleanup) { throw Object.assign(new Error('Timeline recovery cleanup failed'), { errors: [error, cleanup] }); }
    throw error;
  }

  private validate(page: apidto.TimelineRecoveryPage) {
    if (!page || page.conversationId !== this.conversationId || !sameReference(page.reference, this.reference) ||
      page.found !== true || !validVersion(page.contentVersion) || (this.version && page.contentVersion !== this.version) ||
      !Array.isArray(page.chunks) || page.chunks.length > 128 ||
      typeof page.nextCursor !== 'string' || page.nextCursor.length > 16 * 1024 ||
      !/^[A-Za-z0-9_-]*$/.test(page.nextCursor) || ++this.pages > 256) throw invalid();
    let size = 0;
    for (const chunk of page.chunks) {
      if (!chunk || !Number.isSafeInteger(chunk.iteration) || !Number.isSafeInteger(chunk.part) || chunk.part < 0 ||
        !Number.isSafeInteger(chunk.byteOffset) || chunk.byteOffset < 0 || !validVersion(chunk.generation) ||
        (chunk.kind !== 'reasoning' && chunk.kind !== 'text') || typeof chunk.text !== 'string' || !chunk.text ||
        chunk.text.length > 64 * 1024 || /[\uD800-\uDFFF]/u.test(chunk.text)) throw invalid();
      const field = chunk.kind === 'reasoning' ? 0 : 1;
      const previous = this.last;
      const same = previous && chunk.iteration === previous.iteration && chunk.part === previous.part && field === previous.field;
      if (previous && (chunk.iteration < previous.iteration ||
        (chunk.iteration === previous.iteration && (chunk.generation !== previous.generation ||
          chunk.part < previous.part || (chunk.part === previous.part && field < previous.field))))) throw invalid();
      if (chunk.byteOffset !== (same ? previous.offset : 0)) throw invalid();
      const length = encoder.encode(chunk.text).length;
      size += length;
      if (size > 64 * 1024) throw invalid();
      this.last = { iteration: chunk.iteration, generation: chunk.generation, part: chunk.part, field, offset: chunk.byteOffset + length };
    }
    this.bytes += size;
    if (this.bytes > maxBytes || (page.nextCursor && this.cursors.has(page.nextCursor))) throw invalid();
    if (page.nextCursor) this.cursors.add(page.nextCursor);
    this.cursor = page.nextCursor;
  }
}
