// Stub sync store for hosted viewer (no local sync engine).

import * as api from "../api/client.js";

type SyncCallback = () => void;

class SyncStore {
  syncing = $state(false);
  isDesktop = $state(false);
  serverVersion: { version: string; commit: string; build_date: string } | null =
    $state(null);

  private syncCompleteCallbacks: SyncCallback[] = [];
  private sessionWatchCleanup: (() => void) | null = null;

  loadStatus() {}
  loadStats(_opts?: Record<string, unknown>) {}

  async loadVersion() {
    try {
      this.serverVersion = await api.getVersion();
    } catch {
      // ignore
    }
  }

  checkForUpdate() {}
  startPolling() {}
  stopPolling() {}
  triggerSync() {}

  watchSession(
    _id: string,
    _onChange: () => void,
  ) {
    this.unwatchSession();
  }

  unwatchSession() {
    if (this.sessionWatchCleanup) {
      this.sessionWatchCleanup();
      this.sessionWatchCleanup = null;
    }
  }

  onSyncComplete(cb: SyncCallback) {
    this.syncCompleteCallbacks.push(cb);
  }
}

export const sync = new SyncStore();
