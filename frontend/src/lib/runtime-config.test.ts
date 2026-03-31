import { afterEach, describe, expect, it } from "vitest";

import { getClerkPublishableKey } from "./runtime-config.js";

describe("getClerkPublishableKey", () => {
  const originalClerkKey =
    import.meta.env.VITE_CLERK_PUBLISHABLE_KEY;

  afterEach(() => {
    document.head.innerHTML = "";
    import.meta.env.VITE_CLERK_PUBLISHABLE_KEY = originalClerkKey;
  });

  it("prefers runtime meta config over Vite env", () => {
    import.meta.env.VITE_CLERK_PUBLISHABLE_KEY = "pk_vite_123";
    document.head.innerHTML =
      '<meta name="agentsview-clerk-publishable-key" content="pk_runtime_456">';

    expect(getClerkPublishableKey()).toBe("pk_runtime_456");
  });

  it("falls back to Vite env when runtime meta is absent", () => {
    import.meta.env.VITE_CLERK_PUBLISHABLE_KEY = "pk_vite_123";

    expect(getClerkPublishableKey()).toBe("pk_vite_123");
  });
});
