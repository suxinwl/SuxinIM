import { isLocalFileURL, toLocalFileURL } from "./local-files";
import { requiresResourceGrant } from "./resources";

export function previewImageSource(image: { url: string; thumbUrl?: string }, native: boolean, cache: Record<string, string>) {
  if (requiresResourceGrant(image.url)) return image.url;
  if (image.thumbUrl && requiresResourceGrant(image.thumbUrl)) return image.thumbUrl;
  if (!native || isLocalFileURL(image.url) || !image.thumbUrl) return image.url;
  return cache[image.thumbUrl] ? toLocalFileURL(cache[image.thumbUrl]) : image.thumbUrl;
}
