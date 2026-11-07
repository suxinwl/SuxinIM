import { Alert, Button, Input, Modal, Space, Spin, Switch, Typography } from "antd";
import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  DEFAULT_SCREENSHOT_SHORTCUT,
  formatScreenshotShortcut,
  shortcutFromKeyboard,
} from "@/utils/screenshot-shortcut";

export type ScreenshotShortcutState = {
  enabled: boolean;
  accelerator: string;
  registered: boolean;
  persistent: boolean;
  error?: "conflict" | "storage";
  defaultAccelerator: string;
};

type ShortcutResult = {
  ok: boolean;
  code?: "invalid" | "reserved" | "conflict" | "storage" | "busy";
  state: ScreenshotShortcutState;
};

const invoke = <T,>(operation: string, data?: unknown) => {
  if (!window.electronAPI) return Promise.reject(new Error("desktop-required"));
  return window.electronAPI.ipcInvoke<T>("puty-screenshot-shortcut", operation, data);
};

export default function ScreenshotShortcutSettings({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Modal
      title={t("screenshotShortcut.title")}
      open={open}
      footer={null}
      closable={false}
      maskClosable={false}
      keyboard={false}
      destroyOnClose
      centered
      width={520}
    >
      {open && <ScreenshotShortcutSettingsContent onClose={onClose} />}
    </Modal>
  );
}

// Kept independent of account/chat stores so this dialog can be exercised in isolation.
export function ScreenshotShortcutSettingsContent({
  onClose,
}: {
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [state, setState] = useState<ScreenshotShortcutState>();
  const [enabled, setEnabled] = useState(true);
  const [accelerator, setAccelerator] = useState(DEFAULT_SCREENSHOT_SHORTCUT);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [beginning, setBeginning] = useState(false);
  const [ending, setEnding] = useState(false);
  const [recording, setRecording] = useState(false);
  const [error, setError] = useState("");
  const alive = useRef(true);
  const recordRevision = useRef(0);
  const recordingToken = useRef<string>();
  const endPending = useRef<Promise<void>>();
  const endedTokens = useRef(new Set<string>());
  const inputRef = useRef<import("antd").InputRef>(null);

  useEffect(() => {
    if (recording && !beginning) inputRef.current?.focus();
  }, [recording, beginning]);

  const endRecording = useCallback((): Promise<void> => {
    recordRevision.current += 1;
    const token = recordingToken.current;
    if (alive.current) setRecording(false);
    if (endPending.current) return endPending.current;
    if (!token) return endPending.current || Promise.resolve();
    if (alive.current) setEnding(true);
    const pending = (async () => {
      try {
        const result = await invoke<ShortcutResult>("end-record", { token });
        if (!result?.ok) throw new Error("end-record");
        if (recordingToken.current === token) recordingToken.current = undefined;
      } catch (error) {
        // Retain the token for a safe retry if the reply/IPC failed.
        if (alive.current && recordingToken.current === token) setRecording(true);
        throw error;
      }
    })();
    endPending.current = pending;
    const completed = () => {
      if (endPending.current === pending) {
        endPending.current = undefined;
        if (alive.current) setEnding(false);
      }
    };
    void pending.then(completed, completed);
    return pending;
  }, []);

  useEffect(() => {
    alive.current = true;
    const load = async () => {
      try {
        const value = await invoke<ScreenshotShortcutState>("get");
        if (!alive.current) return;
        if (
          !value ||
          typeof value.enabled !== "boolean" ||
          typeof value.accelerator !== "string"
        ) {
          throw new Error("invalid-state");
        }
        setState(value);
        setEnabled(value.enabled);
        setAccelerator(value.accelerator);
      } catch {
        if (alive.current) setError("loadFailed");
      } finally {
        if (alive.current) setLoading(false);
      }
    };
    void load();
    const stop = window.electronAPI?.subscribe(
      "puty-screenshot-shortcut-recording-ended",
      (value: { token?: string }) => {
        if (typeof value?.token !== "string") return;
        endedTokens.current.add(value.token);
        if (endedTokens.current.size > 16) {
          endedTokens.current.delete(endedTokens.current.values().next().value!);
        }
        if (recordingToken.current === value.token) {
          recordingToken.current = undefined;
          recordRevision.current += 1;
          setRecording(false);
        }
      },
    );
    const blur = () => {
      void endRecording().catch(() => {
        if (alive.current) setError("recordFailed");
      });
    };
    window.addEventListener("blur", blur);
    return () => {
      alive.current = false;
      stop?.();
      window.removeEventListener("blur", blur);
      // Also invalidates an in-flight begin; its eventual token is ended below.
      void endRecording().catch(() => undefined);
    };
  }, [endRecording]);

  async function beginRecording() {
    if (loading || saving || beginning || ending || recording || !state?.persistent)
      return;
    const revision = ++recordRevision.current;
    setBeginning(true);
    setError("");
    try {
      const result = await invoke<ShortcutResult & { token?: string }>("begin-record");
      if (!result?.ok || !result.token) {
        if (alive.current && revision === recordRevision.current)
          setError(result?.code || "recordFailed");
        return;
      }
      if (
        !alive.current ||
        revision !== recordRevision.current ||
        endedTokens.current.has(result.token)
      ) {
        await invoke("end-record", { token: result.token });
        return;
      }
      recordingToken.current = result.token;
      setRecording(true);
      inputRef.current?.focus();
    } catch {
      if (alive.current && revision === recordRevision.current)
        setError("recordFailed");
    } finally {
      if (alive.current) setBeginning(false);
    }
  }

  async function finishRecording(value: string) {
    try {
      await endRecording();
      if (alive.current) {
        setAccelerator(value);
        setError("");
      }
    } catch {
      if (alive.current) setError("recordFailed");
    }
  }

  async function close() {
    if (saving) return;
    try {
      await endRecording();
      onClose();
    } catch {
      if (alive.current) setError("recordFailed");
    }
  }

  async function save() {
    if (!state?.persistent || loading || saving || beginning || ending) return;
    setSaving(true);
    setError("");
    try {
      await endRecording();
      const result = await invoke<ShortcutResult>("set", { enabled, accelerator });
      if (!alive.current) return;
      if (!result?.ok) {
        if (result?.state) setState(result.state);
        setError(result?.code || "saveFailed");
        return;
      }
      if (!result.state?.persistent) {
        setState(result.state);
        setError("storage");
        return;
      }
      onClose();
    } catch {
      if (alive.current) setError("saveFailed");
    } finally {
      if (alive.current) setSaving(false);
    }
  }

  const disabled = loading || saving || beginning || ending || !state?.persistent;
  return (
    <Spin spinning={loading}>
      <div
        onKeyDownCapture={(event) => {
          if (!recordingToken.current || endPending.current) return;
          event.preventDefault();
          event.stopPropagation();
          if (event.key === "Escape") {
            void endRecording().catch(() => setError("recordFailed"));
            return;
          }
          if (["Control", "Shift", "Alt", "Meta", "AltGraph"].includes(event.key))
            return;
          if (event.repeat || event.nativeEvent.isComposing) return;
          const value = shortcutFromKeyboard(event);
          if (!value) {
            setError("invalid");
            return;
          }
          void finishRecording(value);
        }}
      >
        <Space direction="vertical" size="middle" className="w-full">
          <div className="flex items-center justify-between">
            <Typography.Text strong>{t("screenshotShortcut.enabled")}</Typography.Text>
            <Switch
              checked={enabled}
              disabled={disabled || recording}
              onChange={setEnabled}
            />
          </div>
          <Typography.Text type="secondary">
            {t("screenshotShortcut.behavior")}
          </Typography.Text>
          {state && !state.persistent && (
            <Alert type="error" showIcon message={t("screenshotShortcut.storage")} />
          )}
          {!error &&
            state?.persistent &&
            state.enabled &&
            !state.registered &&
            !recording && (
              <Alert
                type="warning"
                showIcon
                message={t("screenshotShortcut.conflict")}
              />
            )}
          {error && (
            <Alert type="error" showIcon message={t(`screenshotShortcut.${error}`)} />
          )}
          <div>
            <label
              className="mb-2 block font-medium"
              htmlFor="puty-screenshot-shortcut"
            >
              {t("screenshotShortcut.combination")}
            </label>
            <Input
              ref={inputRef}
              id="puty-screenshot-shortcut"
              readOnly
              value={
                recording
                  ? t("screenshotShortcut.pressKeys")
                  : formatScreenshotShortcut(accelerator)
              }
              aria-label={t("screenshotShortcut.combination")}
              aria-live="polite"
              status={recording ? "warning" : undefined}
              disabled={disabled}
            />
          </div>
          <Typography.Text type="secondary">
            {t("screenshotShortcut.hint")}
          </Typography.Text>
          <Space wrap>
            <Button
              disabled={disabled}
              type={recording ? "primary" : "default"}
              loading={beginning}
              onClick={() =>
                recording
                  ? void endRecording().catch(() => setError("recordFailed"))
                  : void beginRecording()
              }
            >
              {t(
                recording
                  ? "screenshotShortcut.stopRecording"
                  : "screenshotShortcut.record",
              )}
            </Button>
            <Button
              disabled={disabled || recording}
              onClick={() => {
                setAccelerator(
                  state?.defaultAccelerator || DEFAULT_SCREENSHOT_SHORTCUT,
                );
                setError("");
              }}
            >
              {t("screenshotShortcut.restoreDefault")}
            </Button>
          </Space>
          {recording && (
            <Alert type="info" showIcon message={t("screenshotShortcut.recording")} />
          )}
          <div className="mt-2 flex justify-end gap-2">
            <Button disabled={saving} onClick={() => void close()}>
              {t("screenshotShortcut.cancel")}
            </Button>
            <Button
              type="primary"
              disabled={disabled || recording}
              loading={saving}
              onClick={() => void save()}
            >
              {t("screenshotShortcut.save")}
            </Button>
          </div>
        </Space>
      </div>
    </Spin>
  );
}
