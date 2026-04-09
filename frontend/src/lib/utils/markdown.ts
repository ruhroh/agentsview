import { Marked } from "marked";
import DOMPurify from "dompurify";
import { LRUCache } from "./cache.js";

const parser = new Marked({
  gfm: true,
  breaks: true,
});

const cache = new LRUCache<string, string>(6000);

function getApiBase(): string {
  const baseEl = document.querySelector("base[href]");
  if (baseEl) {
    const base = new URL(document.baseURI).pathname.replace(/\/$/, "");
    return `${base}/api/v1`;
  }
  return "/api/v1";
}

function resolveAssetURLs(text: string): string {
  return text.replace(
    /asset:\/\/([^\s)]+)/g,
    `${getApiBase()}/assets/$1`,
  );
}

export function renderMarkdown(text: string): string {
  if (!text) return "";

  const cached = cache.get(text);
  if (cached !== undefined) return cached;

  const resolved = resolveAssetURLs(text);

  // Trim trailing whitespace — with breaks:true, trailing
  // newlines become <br> tags that add invisible height.
  const html = parser.parse(resolved.trimEnd()) as string;
  const safe = DOMPurify.sanitize(html);

  cache.set(text, safe);
  return safe;
}
