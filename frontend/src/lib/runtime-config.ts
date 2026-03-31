const CLERK_PUBLISHABLE_KEY_META =
  'meta[name="agentsview-clerk-publishable-key"]';

function readMetaContent(selector: string): string {
  if (typeof document === "undefined") return "";
  const content = document
    .querySelector<HTMLMetaElement>(selector)
    ?.content;
  return content?.trim() ?? "";
}

export function getClerkPublishableKey(): string {
  return (
    readMetaContent(CLERK_PUBLISHABLE_KEY_META) ||
    import.meta.env.VITE_CLERK_PUBLISHABLE_KEY?.trim() ||
    ""
  );
}
