import { useEffect, useState } from "react";
import { api, type VartijaEvent } from "../api";
import { absolute, relative } from "../format";
import { useI18n, type Key, type T } from "../i18n";
import { Badge, Card, Empty, ErrorBox } from "./ui";

const tones: Record<VartijaEvent["type"], "new" | "known" | "muted" | "warn" | "error"> = {
  new_device: "new",
  ip_changed: "muted",
  acknowledged: "known",
  baseline: "warn",
  scan_failed: "error",
};

// Fields each sentence needs; an event without them (older rows) shows the
// message the server stored
const required: Record<VartijaEvent["type"], string[]> = {
  new_device: ["name", "ip", "cidr"],
  ip_changed: ["name", "old_ip", "ip"],
  acknowledged: ["name"],
  baseline: ["cidr", "count"],
  scan_failed: ["cidr", "error"],
};

function message(e: VartijaEvent, t: T): string {
  const d = e.data;
  if (!d || !required[e.type]?.every((k) => k in d)) return e.message;
  const key: Key = e.type === "new_device" && d.vendor ? "events.msg.new_device_vendor" : (`events.msg.${e.type}` as Key);
  return t(key, d);
}

export default function Events({ refresh }: { refresh: number }) {
  const { t, locale } = useI18n();
  const [events, setEvents] = useState<VartijaEvent[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .events()
      .then((e) => {
        setEvents(e);
        setError("");
      })
      .catch((e) => setError(e instanceof Error ? e.message : t("common.loadError")));
  }, [refresh, t]);

  return (
    <div className="space-y-4">
      <ErrorBox message={error} />
      <Card className="overflow-hidden">
        {events === null ? (
          <Empty>{t("common.loading")}</Empty>
        ) : events.length === 0 ? (
          <Empty>{t("events.empty")}</Empty>
        ) : (
          <ul className="divide-y divide-slate-100 dark:divide-slate-800">
            {events.map((e) => (
              <li key={e.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2.5 text-sm">
                <span className="w-32 shrink-0 text-xs text-slate-500 dark:text-slate-400" title={absolute(e.at, locale)}>
                  {relative(e.at, locale)}
                </span>
                <Badge tone={tones[e.type] ?? "muted"}>{t(`events.type.${e.type}` as Key)}</Badge>
                <span className="min-w-0 flex-1 break-words">{message(e, t)}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
