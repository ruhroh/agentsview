import {
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

vi.mock("../api/client.js", () => ({
  getUsageSummary: vi.fn().mockResolvedValue({
    from: "2024-01-01",
    to: "2024-01-31",
    totals: {
      inputTokens: 0,
      outputTokens: 0,
      cacheCreationTokens: 0,
      cacheReadTokens: 0,
      totalCost: 0,
    },
    daily: [],
    projectTotals: [],
    modelTotals: [],
    agentTotals: [],
    sessionCounts: {
      total: 0,
      byProject: {},
      byAgent: {},
    },
    cacheStats: {
      cacheReadTokens: 0,
      cacheCreationTokens: 0,
      uncachedInputTokens: 0,
      outputTokens: 0,
      hitRate: 0,
      savingsVsUncached: 0,
    },
  }),
  getUsageTopSessions: vi.fn().mockResolvedValue([]),
}));

const TOGGLES_KEY = "usage-toggles";

function installStorage(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial));
  const storage = {
    getItem: vi.fn((key: string) => data.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      data.set(key, value);
    }),
    removeItem: vi.fn((key: string) => {
      data.delete(key);
    }),
    clear: vi.fn(() => {
      data.clear();
    }),
  };
  Object.defineProperty(globalThis, "localStorage", {
    value: storage,
    configurable: true,
    writable: true,
  });
  return storage;
}

async function loadStore() {
  vi.resetModules();
  return import("./usage.svelte.js");
}

describe("UsageStore group-by linking", () => {
  beforeEach(() => {
    installStorage();
    localStorage.removeItem(TOGGLES_KEY);
    vi.clearAllMocks();
  });

  it("normalizes legacy split groupBy values onto shared state", async () => {
    localStorage.setItem(
      TOGGLES_KEY,
      JSON.stringify({
        timeSeries: { groupBy: "agent", view: "lines" },
        attribution: { groupBy: "model", view: "list" },
      }),
    );

    const { usage } = await loadStore();

    expect(usage.toggles.timeSeries.groupBy).toBe("agent");
    expect(usage.toggles.attribution.groupBy).toBe("agent");
    expect(usage.toggles.timeSeries.view).toBe("lines");
    expect(usage.toggles.attribution.view).toBe("list");
  });

  it("syncs attribution selector when time-series selector changes", async () => {
    const { usage } = await loadStore();

    usage.setTimeSeriesGroupBy("model");

    expect(usage.toggles.timeSeries.groupBy).toBe("model");
    expect(usage.toggles.attribution.groupBy).toBe("model");
    expect(JSON.parse(localStorage.getItem(TOGGLES_KEY) || "{}")).toMatchObject({
      timeSeries: { groupBy: "model" },
      attribution: { groupBy: "model" },
    });
  });

  it("syncs time-series selector when attribution selector changes", async () => {
    const { usage } = await loadStore();

    usage.setAttributionGroupBy("agent");

    expect(usage.toggles.timeSeries.groupBy).toBe("agent");
    expect(usage.toggles.attribution.groupBy).toBe("agent");
    expect(JSON.parse(localStorage.getItem(TOGGLES_KEY) || "{}")).toMatchObject({
      timeSeries: { groupBy: "agent" },
      attribution: { groupBy: "agent" },
    });
  });
});
