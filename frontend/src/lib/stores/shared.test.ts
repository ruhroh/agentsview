import { describe, it, expect, beforeEach, vi } from "vitest";
import * as api from "../api/client.js";
import { createSharedStore } from "./shared.svelte.js";

vi.mock("../api/client.js", () => ({
  listShared: vi.fn().mockResolvedValue({ session_ids: [] }),
  shareSession: vi.fn().mockResolvedValue(undefined),
  unshareSession: vi.fn().mockResolvedValue(undefined),
}));

describe("SharedStore", () => {
  let store: ReturnType<typeof createSharedStore>;

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listShared).mockResolvedValue({ session_ids: [] });
    store = createSharedStore();
  });

  it("starts empty", () => {
    expect(store.count).toBe(0);
    expect(store.isShared("abc")).toBe(false);
  });

  it("loads shared IDs from server", async () => {
    vi.mocked(api.listShared).mockResolvedValue({
      session_ids: ["s1", "s2"],
    });
    store = createSharedStore();
    await store.load();

    expect(store.isShared("s1")).toBe(true);
    expect(store.isShared("s2")).toBe(true);
    expect(store.count).toBe(2);
  });

  it("share adds to set optimistically", () => {
    store.share("s1");
    expect(store.isShared("s1")).toBe(true);
    expect(store.count).toBe(1);
  });

  it("share calls API", async () => {
    store.share("s1");
    await vi.waitFor(() => {
      expect(api.shareSession).toHaveBeenCalledWith("s1");
    });
  });

  it("unshare removes from set optimistically", async () => {
    vi.mocked(api.listShared).mockResolvedValue({
      session_ids: ["s1"],
    });
    store = createSharedStore();
    await store.load();

    store.unshare("s1");
    expect(store.isShared("s1")).toBe(false);
    expect(store.count).toBe(0);
  });

  it("unshare calls API", async () => {
    vi.mocked(api.listShared).mockResolvedValue({
      session_ids: ["s1"],
    });
    store = createSharedStore();
    await store.load();

    store.unshare("s1");
    await vi.waitFor(() => {
      expect(api.unshareSession).toHaveBeenCalledWith("s1");
    });
  });

  it("toggle shares then unshares", () => {
    store.toggle("s1");
    expect(store.isShared("s1")).toBe(true);

    store.toggle("s1");
    expect(store.isShared("s1")).toBe(false);
  });

  it("share is idempotent", async () => {
    store.share("s1");
    store.share("s1");
    await vi.waitFor(() => {
      expect(api.shareSession).toHaveBeenCalledTimes(1);
    });
  });

  it("unshare on non-shared is no-op", () => {
    store.unshare("s1");
    expect(api.unshareSession).not.toHaveBeenCalled();
  });

  it("load is idempotent after success", async () => {
    await store.load();
    await store.load();
    expect(api.listShared).toHaveBeenCalledTimes(1);
  });

  it("handles load failure gracefully", async () => {
    vi.mocked(api.listShared).mockRejectedValue(new Error("network"));
    store = createSharedStore();
    await store.load();

    expect(store.count).toBe(0);
  });
});
