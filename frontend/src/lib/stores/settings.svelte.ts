// Stub settings store for hosted viewer.

class SettingsStore {
  needsAuth = $state(false);

  load() {}
}

export const settings = new SettingsStore();
