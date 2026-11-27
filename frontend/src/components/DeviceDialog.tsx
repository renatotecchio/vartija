import { useEffect, useRef, useState, type FormEvent } from "react";
import { api, type Device } from "../api";
import { absolute, sourceLabel } from "../format";
import { useI18n } from "../i18n";
import { Badge, Button, ErrorBox, inputClass } from "./ui";

export default function DeviceDialog({
  device,
  subnet,
  onClose,
  onSaved,
}: {
  device: Device;
  subnet?: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t, locale } = useI18n();
  const ref = useRef<HTMLDialogElement>(null);
  const [nickname, setNickname] = useState(device.nickname);
  const [notes, setNotes] = useState(device.notes);
  const [known, setKnown] = useState(device.status === "known");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    ref.current?.showModal();
  }, []);

  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api.updateDevice(device.id, { nickname, notes, status: known ? "known" : "new" });
      onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("dialog.saveError"));
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!confirm(t("dialog.forgetConfirm"))) return;
    setBusy(true);
    try {
      await api.deleteDevice(device.id);
      onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("dialog.deleteError"));
      setBusy(false);
    }
  };

  const label = "text-slate-500 dark:text-slate-400";

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      className="m-auto w-[calc(100%-2rem)] max-w-lg rounded-lg border border-slate-200 bg-white p-0 text-slate-900 shadow-xl backdrop:bg-slate-900/50 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-100"
    >
      <form onSubmit={save} className="space-y-4 p-5">
        <div>
          <h2 className="text-lg font-semibold break-words">{device.display_name}</h2>
          <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
            <dt className={label}>{t("dialog.mac")}</dt>
            <dd className="flex items-center gap-2 font-mono text-xs">
              {device.mac}
              {device.random_mac && <Badge tone="warn">{t("devices.random")}</Badge>}
            </dd>
            <dt className={label}>{t("dialog.vendor")}</dt>
            <dd className="break-words">{device.vendor || "—"}</dd>
            <dt className={label}>{t("dialog.ip")}</dt>
            <dd className="font-mono text-xs">{[device.ip, ...(device.extra_ips ?? [])].join(", ")}</dd>
            <dt className={label}>{t("dialog.hostname")}</dt>
            <dd className="break-words">
              {device.hostname || "—"}
              {device.hostname_source && <span className="text-slate-400"> ({sourceLabel[device.hostname_source] ?? device.hostname_source})</span>}
            </dd>
            <dt className={label}>{t("dialog.subnet")}</dt>
            <dd>{subnet ?? "—"}</dd>
            <dt className={label}>{t("dialog.firstSeen")}</dt>
            <dd>{absolute(device.first_seen, locale)}</dd>
            <dt className={label}>{t("dialog.lastSeen")}</dt>
            <dd>{absolute(device.last_seen, locale)}</dd>
          </dl>
        </div>

        <label className="block">
          <span className="mb-1 block text-sm font-medium">{t("dialog.name")}</span>
          <input
            className={inputClass}
            value={nickname}
            maxLength={100}
            onChange={(e) => setNickname(e.target.value)}
            placeholder={device.hostname || t("dialog.namePlaceholder")}
            autoFocus
          />
          <span className="mt-1 block text-xs text-slate-500 dark:text-slate-400">{t("dialog.nameHint")}</span>
        </label>

        <label className="block">
          <span className="mb-1 block text-sm font-medium">{t("dialog.notes")}</span>
          <textarea className={`${inputClass} min-h-20`} value={notes} maxLength={1000} onChange={(e) => setNotes(e.target.value)} />
        </label>

        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={known} onChange={(e) => setKnown(e.target.checked)} />
          {t("dialog.known")}
        </label>

        <ErrorBox message={error} />

        <div className="flex flex-wrap items-center gap-2 pt-2">
          <Button type="button" variant="danger" onClick={remove} disabled={busy}>
            {t("dialog.forget")}
          </Button>
          <div className="ml-auto flex gap-2">
            <Button type="button" onClick={() => ref.current?.close()} disabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" disabled={busy}>
              {t("common.save")}
            </Button>
          </div>
        </div>
      </form>
    </dialog>
  );
}
