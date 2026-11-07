interface RouteTitleMeta {
  locale?: string;
  title?: string;
}

/** Plugin menus may arrive after the host was compiled, without a bundled translation. */
export function routeTitle(
  meta: RouteTitleMeta | undefined,
  translate: (key: string) => string,
  hasTranslation: (key: string) => boolean,
): string {
  const key = meta?.locale;
  if (!key) return meta?.title || '';
  const translated = translate(key);
  // Existing dictionaries contain flat dotted keys: te() can miss keys that t() resolves.
  return hasTranslation(key) || translated !== key ? translated : meta?.title || key;
}
