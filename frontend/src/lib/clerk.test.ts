import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { isClerkConfigured } from "./clerk.js";

describe("isClerkConfigured", () => {
  const originalClerkKey =
    import.meta.env.VITE_CLERK_PUBLISHABLE_KEY;

  beforeEach(() => {
    document.head.innerHTML = "";
    import.meta.env.VITE_CLERK_PUBLISHABLE_KEY = "";
  });

  it("returns false when no publishable key is configured", () => {
    expect(isClerkConfigured()).toBe(false);
  });

  it("returns true when env key is set", () => {
    import.meta.env.VITE_CLERK_PUBLISHABLE_KEY = "pk_test_123";
    expect(isClerkConfigured()).toBe(true);
  });

  it("returns true when runtime meta key is set", () => {
    document.head.innerHTML =
      '<meta name="agentsview-clerk-publishable-key" content="pk_runtime_123">';
    expect(isClerkConfigured()).toBe(true);
  });

  afterEach(() => {
    document.head.innerHTML = "";
    import.meta.env.VITE_CLERK_PUBLISHABLE_KEY = originalClerkKey;
  });
});
