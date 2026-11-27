import { useEffect, useState } from "react";
import { api, type VartijaEvent } from "../api";
import { absolute, relative } from "../format";
import { Badge, Card, Empty, ErrorBox } from "./ui";

const labels: Record<VartijaEvent["type"], { text: string; tone: "new" | "known" | "muted" | "warn" | "error" }> = {
  new_device: { text: "novo", tone: "new" },
  ip_changed: { text: "IP mudou", tone: "muted" },
  acknowledged: { text: "conhecido", tone: "known" },
  baseline: { text: "linha de base", tone: "warn" },
  scan_failed: { text: "falha", tone: "error" },
};

export default function Events({ refresh }: { refresh: number }) {
  const [events, setEvents] = useState<VartijaEvent[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .events()
      .then((e) => {
        setEvents(e);
        setError("");
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Erro ao carregar"));
  }, [refresh]);

  return (
    <div className="space-y-4">
      <ErrorBox message={error} />
      <Card className="overflow-hidden">
        {events === null ? (
          <Empty>Carregando…</Empty>
        ) : events.length === 0 ? (
          <Empty>Nenhum evento ainda.</Empty>
        ) : (
          <ul className="divide-y divide-slate-100 dark:divide-slate-800">
            {events.map((e) => (
              <li key={e.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2.5 text-sm">
                <span className="w-28 shrink-0 text-xs text-slate-500 dark:text-slate-400" title={absolute(e.at)}>
                  {relative(e.at)}
                </span>
                <Badge tone={labels[e.type]?.tone ?? "muted"}>{labels[e.type]?.text ?? e.type}</Badge>
                <span className="min-w-0 flex-1 break-words">{e.message}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
