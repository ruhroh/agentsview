import * as api from "../api/client.js";

class SharedStore {
  ids: Set<string> = $state(new Set());
  private loaded = false;
  private loading: Promise<void> | null = null;
  private mutationVersion = 0;
  private refreshId = 0;
  private queues: Map<string, Promise<void>> = new Map();

  async load() {
    if (this.loaded) return;
    if (this.loading) return this.loading;
    this.loading = this.doLoad();
    return this.loading;
  }

  private async doLoad() {
    const mutVer = this.mutationVersion;
    const rid = ++this.refreshId;
    try {
      const res = await api.listShared();
      if (this.mutationVersion === mutVer && this.refreshId === rid) {
        this.ids = new Set(res.session_ids);
      }
      this.loaded = true;
    } catch {
      // Server unavailable; keep empty state.
    } finally {
      this.loading = null;
    }
  }

  isShared(sessionId: string): boolean {
    return this.ids.has(sessionId);
  }

  toggle(sessionId: string) {
    if (this.ids.has(sessionId)) {
      this.unshare(sessionId);
    } else {
      this.share(sessionId);
    }
  }

  share(sessionId: string) {
    if (this.ids.has(sessionId)) return;
    const next = new Set(this.ids);
    next.add(sessionId);
    this.ids = next;
    this.mutationVersion++;
    this.enqueue(sessionId, () => api.shareSession(sessionId));
  }

  unshare(sessionId: string) {
    if (!this.ids.has(sessionId)) return;
    const next = new Set(this.ids);
    next.delete(sessionId);
    this.ids = next;
    this.mutationVersion++;
    this.enqueue(sessionId, () => api.unshareSession(sessionId));
  }

  private enqueue(
    sessionId: string,
    op: () => Promise<unknown>,
  ) {
    const prev = this.queues.get(sessionId) ?? Promise.resolve();
    const chain: Promise<void> = prev
      .then(() => op(), () => op())
      .then(() => {}, () => {})
      .then(() => {
        if (this.queues.get(sessionId) === chain) {
          this.queues.delete(sessionId);
        }
        this.reconcileIfIdle();
      });
    this.queues.set(sessionId, chain);
  }

  private reconcileIfIdle() {
    if (this.queues.size > 0) return;
    const mutVer = this.mutationVersion;
    const rid = ++this.refreshId;
    api.listShared().then((res) => {
      if (this.mutationVersion === mutVer && this.refreshId === rid) {
        this.ids = new Set(res.session_ids);
      }
    }).catch(() => {
      // Server unavailable; keep optimistic state.
    });
  }

  get count(): number {
    return this.ids.size;
  }
}

export function createSharedStore(): SharedStore {
  return new SharedStore();
}

export const shared = createSharedStore();
