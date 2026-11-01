export type ForwardTarget = { userID?: string; groupID?: string };
export type ForwardProgress = { primaryDone: boolean; additionalDone: boolean };

// Keep the SDK factories and sends in one ordered operation. A recipient is
// successful only after both the forwarded message and its optional note finish.
export async function forwardBatch<T extends ForwardTarget, M>(options: {
  targets: T[];
  additional: string;
  progress: Map<string, ForwardProgress>;
  createMessage: () => Promise<M>;
  createAdditional: (text: string) => Promise<M>;
  isJoinGroup: (groupID: string) => Promise<boolean>;
  send: (message: M, target: T) => Promise<unknown>;
}) {
  const failures: { target: T; error: unknown }[] = [];
  const successful: T[] = [];
  const seen = new Set<string>();
  for (const target of options.targets) {
    const key = target.groupID ? `group:${target.groupID}` : `user:${target.userID}`;
    if (seen.has(key)) continue;
    seen.add(key);
    const progress = options.progress.get(key) ?? { primaryDone: false, additionalDone: false };
    options.progress.set(key, progress);
    try {
      if (!target.groupID && !target.userID) throw new Error("Forward recipient is unavailable");
      if (target.groupID && !await options.isJoinGroup(target.groupID)) throw new Error("Forward group is unavailable");
      if (!progress.primaryDone) {
        await options.send(await options.createMessage(), target);
        progress.primaryDone = true;
      }
      if (options.additional && !progress.additionalDone) {
        await options.send(await options.createAdditional(options.additional), target);
        progress.additionalDone = true;
      }
      successful.push(target);
    } catch (error) {
      failures.push({ target, error });
    }
  }
  return { successful, failures };
}
