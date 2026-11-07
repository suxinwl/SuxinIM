import { authorizeResource } from "@/im/resources";
import type { MomentMeta, WorkMoments } from "@/types/moment";
import { getResourceSessionVersion } from "@/im/resource-session";

export const momentMediaSource = (meta: MomentMeta, field: "original" | "thumb") => meta.readSources?.[field] ?? meta[field];

// Keep upload/published URLs unchanged. Bind only the viewer's read copy.
export async function authorizeMomentMedia(moment: WorkMoments): Promise<WorkMoments> {
  if (!moment?.content?.metas) return moment;
  const scope = getResourceSessionVersion();
  const bind = async (source: string) => {
    try { return source ? await authorizeResource(source) : source; }
    catch {
      if (scope !== getResourceSessionVersion()) throw new Error("Resource session changed");
      return ""; // One missing asset cannot hide every other post.
    }
  };
  const metas = await Promise.all(moment.content.metas.map(async meta => {
    const original = momentMediaSource(meta, "original"), thumb = momentMediaSource(meta, "thumb");
    return { ...meta, original: await bind(original), thumb: await bind(thumb), readSources: { original, thumb } };
  }));
  if (scope !== getResourceSessionVersion()) throw new Error("Resource session changed");
  return { ...moment, content: { ...moment.content, metas } };
}

// Thumbnail transformations are not signed/supported by this S3 endpoint.
// Reuse its image rather than appending a second '?' or altering SigV4 query.
export function publishedMomentMedia(url: string, width?: number, height?: number) {
  return { original: url, thumb: url, width, height };
}
