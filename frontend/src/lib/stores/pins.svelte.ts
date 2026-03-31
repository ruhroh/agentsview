// Stub pins store for hosted viewer.

class PinsStore {
  isPinned(_messageId: string): boolean {
    return false;
  }

  async togglePin(
    _sessionId: string,
    _messageId: string,
    _ordinal: number,
  ) {}

  loadForSession(_sessionId: string) {}
  clearSession() {}
}

export const pins = new PinsStore();
