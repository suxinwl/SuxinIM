import { createHash } from "node:crypto";
import { reconstructLegacySQLite, type LegacyRawDatabase } from "./legacy";

export const LEGACY_RESOLUTION_POLICY = "PUTY-LEGACY-RESOLUTION-1";
export type LegacyQuarantineReason = "QUARANTINE_NO_CANDIDATE" | "QUARANTINE_SCHEMA" | "QUARANTINE_PAGES" | "QUARANTINE_CONFLICT" | "QUARANTINE_IDENTITY" | "QUARANTINE_SQLITE" | "QUARANTINE_CAPSULE" | "QUARANTINE_LIMIT" | "QUARANTINE_NAME" | "QUARANTINE_KEYS" | "QUARANTINE_SIZE" | "QUARANTINE_BINARY" | "QUARANTINE_HEADER" | "QUARANTINE_GEOMETRY" | "QUARANTINE_MISSING_PAGE" | "QUARANTINE_LENGTH";
// Exact closed reasons only. Raw exceptions, database names and page bytes
// stay in the main process; the outer IPC wrapper emits only the fixed code.
const pageFailureReasons = new Map<string, LegacyQuarantineReason>([
  ["Legacy database identity is unrecognized", "QUARANTINE_NAME"],
  ["Legacy page keys are ambiguous", "QUARANTINE_KEYS"],
  ["Legacy SQLite size is unsupported", "QUARANTINE_SIZE"],
  ["Legacy page is not binary", "QUARANTINE_BINARY"],
  ["Legacy SQLite header is invalid", "QUARANTINE_HEADER"],
  ["Legacy SQLite page geometry is invalid", "QUARANTINE_GEOMETRY"],
  ["Legacy SQLite page is missing; preserve raw database", "QUARANTINE_MISSING_PAGE"],
  ["Legacy SQLite page length is invalid", "QUARANTINE_LENGTH"],
]);
export function legacyQuarantineFailure(reason: LegacyQuarantineReason, cause?: unknown): Error {
  return Object.assign(new Error("Legacy quarantined account recovery failed", { cause }), { legacyQuarantineReason: reason });
}

/** Pure RAM-only resolution. Every database claiming the active account must
 * pass, including malformed siblings. Different histories are never merged. */
export function resolveLegacyQuarantine(raw: unknown, accountID: string, verify: (bytes: Uint8Array, accountID: string) => void): { bytes: Uint8Array; sha256: string; databaseNames: string[] } {
  if (!/^emp_[a-f0-9]{32}$/.test(accountID)) throw legacyQuarantineFailure("QUARANTINE_IDENTITY");
  if (!Array.isArray(raw) || raw.length > 128) throw legacyQuarantineFailure("QUARANTINE_LIMIT");
  let selected: Uint8Array | undefined, total = 0;
  const names: string[] = [], seen = new Set<string>();
  try {
    for (const input of raw) {
      if (!input || typeof input !== "object" || typeof input.name !== "string" || input.name.length > 1024) throw legacyQuarantineFailure("QUARANTINE_SCHEMA");
      // The suffix is deliberately broader than the recognized Absurd-SQL
      // prefix: an unknown current-account sibling must not be silently ignored.
      if (!input.name.endsWith(accountID + ".sqlite")) continue;
      if (seen.has(input.name)) throw legacyQuarantineFailure("QUARANTINE_SCHEMA");
      seen.add(input.name);
      const row = input as LegacyRawDatabase;
      if (row.version !== 2 || !Array.isArray(row.stores) || row.stores.length !== 1 || row.stores[0] !== "data" || !Array.isArray(row.pages) || row.pages.length > 524288) throw legacyQuarantineFailure("QUARANTINE_SCHEMA");
      let current: Uint8Array | undefined;
      try {
        try { current = reconstructLegacySQLite(row.name, row.pages).bytes; }
        catch (error) { throw legacyQuarantineFailure(error instanceof Error ? pageFailureReasons.get(error.message) || "QUARANTINE_PAGES" : "QUARANTINE_PAGES", error); }
        total += current.length;
        if (total > 1024 * 1024 * 1024) throw legacyQuarantineFailure("QUARANTINE_LIMIT");
        try { verify(current, accountID); }
        catch (error) {
          const message = error instanceof Error ? error.message : "";
          const identity = ["Legacy SQLite login identity differs from its declared account", "Legacy history has no verifiable login identity; encrypted quarantine retained", "Legacy declared account is invalid"].includes(message);
          throw legacyQuarantineFailure(identity ? "QUARANTINE_IDENTITY" : "QUARANTINE_SQLITE", error);
        }
        if (selected) {
          // Compare complete bytes as well as the hash; neither an mtime nor a
          // record count establishes which divergent database is authoritative.
          const sha = (bytes: Uint8Array) => createHash("sha256").update(bytes).digest("hex");
          if (sha(selected) !== sha(current) || !Buffer.from(selected.buffer, selected.byteOffset, selected.byteLength).equals(Buffer.from(current.buffer, current.byteOffset, current.byteLength))) throw legacyQuarantineFailure("QUARANTINE_CONFLICT");
        } else { selected = current; current = undefined; }
        names.push(row.name);
      } finally { current?.fill(0); }
    }
    if (!selected) throw legacyQuarantineFailure("QUARANTINE_NO_CANDIDATE");
    const result = { bytes: selected, sha256: createHash("sha256").update(selected).digest("hex"), databaseNames: names.sort() };
    selected = undefined;
    return result;
  } finally { selected?.fill(0); }
}
