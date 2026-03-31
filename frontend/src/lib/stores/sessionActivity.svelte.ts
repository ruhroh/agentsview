// Stub session activity store for hosted viewer.

class SessionActivityStore {
  firstVisibleTimestamp: string | null = $state(null);

  reload(_sessionId: string) {}
  invalidate() {}
  clear() {}
}

export const sessionActivity = new SessionActivityStore();
