import { getClerkPublishableKey } from "./runtime-config.js";

export function isClerkConfigured(): boolean {
  return getClerkPublishableKey() !== "";
}
