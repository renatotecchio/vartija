import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type Device, type DeviceStatus, type Subnet } from "../api";
import { absolute, relative, sourceLabel } from "../format";
import { useI18n, type Key } from "../i18n";
import { Badge, Button, Card, Empty, ErrorBox, inputClass } from "./ui";
import DeviceDialog from "./DeviceDialog";

type Filter = DeviceStatus | "";

const filters: { id: Filter; label: Key }[] = [
  { id: "new", label: "devices.filter.new" },
  { id: "known", label: "devices.filter.known" },
  { id: "", label: "devices.filter.all" },
];

function initialFilter(): Filter {
  const s = new URLSearchParams(window.location.search).get("status");
  if (s === "known") return "known";
  if (s === "all") return "";
  return "new";
}

export default function Devices({ refresh, onChange }: { refresh: number; onChange: () => void }) {
  const { t, locale } = useI18n();
  const [filter, setFilter] = useState<Filter>(initialFilter);
  const [search, setSearch] = useState("");
  const [subnetId, setSubnetId] = useState(0);
  const [devices, setDevices] = useState<Device[]>([]);
  const [subnets, setSubnets] = useState<Subnet[]>([]);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [editing, setEditing] = useState<Device | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      const [d, s] = await Promise.all([api.devices({ status: filter, q: search, subnet_id: subnetId }), api.subnets()]);
      setDevices(d);
      setSubnets(s);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : t("common.loadError"));
    } finally {
      setLoading(false);
    }
  }, [filter, search, subnetId, t]);

  useEffect(() => {
    const id = setTimeout(load, search ? 250 : 0);
    return () => clearTimeout(id);
  }, [load, refresh, search]);

  useEffect(() => {
    const id = setInterval(load, 30000);
    return () => clearInterval(id);
  }, [load]);

  useEffect(() => setSelected(new Set()), [filter, subnetId]);

  const subnetName = useMemo(() => {
    const m = new Map<number, string>();
    subnets.forEach((s) => m.set(s.id, s.name || s.cidr));
    return m;
  }, [subnets]);

  const changeFilter = (f: Filter) => {
    setFilter(f);
    const url = new URL(window.location.href);
    url.searchParams.set("status", f || "all");
    window.history.replaceState(null, "", url);
  };

  const after = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      setSelected(new Set());
      await load();
      onChange();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("common.error"));
    }
  };

  const newIds = devices.filter((d) => d.status === "new").map((d) => d.id);
  const allSelected = devices.length > 0 && devices.every((d) => selected.has(d.id));
  const toggle = (id: number) =>
    setSelected((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  const toggleAll = () => setSelected(allSelected ? new Set() : new Set(devices.map((d) => d.id)));

  const markAllNew = () => {
    if (!confirm(t("devices.markAllConfirm", { n: newIds.length }))) return;
    after(() => api.acknowledgeAll());
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex rounded-md border border-slate-300 bg-white p-0.5 dark:border-slate-700 dark:bg-slate-900">
          {filters.map((f) => (
            <button
              key={f.id || "all"}
              onClick={() => changeFilter(f.id)}
              className={`rounded px-3 py-1 text-sm font-medium ${
                filter === f.id ? "bg-teal-600 text-white" : "text-slate-600 hover:text-slate-900 dark:text-slate-300 dark:hover:text-white"
              }`}
            >
              {t(f.label)}
            </button>
          ))}
        </div>
        <input className={`${inputClass} max-w-xs`} placeholder={t("devices.search")} value={search} onChange={(e) => setSearch(e.target.value)} aria-label={t("devices.search")} />
        {subnets.length > 1 && (
          <select className={`${inputClass} w-auto`} value={subnetId} onChange={(e) => setSubnetId(Number(e.target.value))} aria-label={t("devices.subnet")}>
            <option value={0}>{t("devices.allSubnets")}</option>
            {subnets.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name ? `${s.name} (${s.cidr})` : s.cidr}
              </option>
            ))}
          </select>
        )}
        <div className="ml-auto flex flex-wrap gap-2">
          {selected.size > 0 && (
            <Button variant="primary" onClick={() => after(() => api.acknowledge([...selected]))}>
              {t("devices.markSelected", { n: selected.size })}
            </Button>
          )}
          {filter === "new" && newIds.length > 0 && selected.size === 0 && <Button onClick={markAllNew}>{t("devices.markAllNew")}</Button>}
        </div>
      </div>

      <ErrorBox message={error} />

      <Card className="overflow-hidden">
        {loading ? (
          <Empty>{t("common.loading")}</Empty>
        ) : devices.length === 0 ? (
          <Empty>{filter === "new" ? (subnets.length === 0 ? t("devices.emptyNoSubnets") : t("devices.emptyNew")) : t("devices.empty")}</Empty>
        ) : (
          <>
            {/* Desktop: table */}
            <table className="hidden w-full text-sm md:table">
              <thead className="border-b border-slate-200 bg-slate-50 text-left text-xs font-semibold tracking-wide text-slate-500 uppercase dark:border-slate-800 dark:bg-slate-900/60 dark:text-slate-400">
                <tr>
                  <th className="w-10 px-3 py-2">
                    <input type="checkbox" checked={allSelected} onChange={toggleAll} aria-label={t("devices.selectAll")} />
                  </th>
                  <th className="px-3 py-2">{t("devices.col.device")}</th>
                  <th className="px-3 py-2">{t("devices.col.mac")}</th>
                  <th className="px-3 py-2">{t("devices.col.ip")}</th>
                  <th className="px-3 py-2">{t("devices.col.subnet")}</th>
                  <th className="px-3 py-2">{t("devices.col.seen")}</th>
                  <th className="px-3 py-2 text-right">{t("devices.col.actions")}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {devices.map((d) => (
                  <tr key={d.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/40">
                    <td className="px-3 py-2">
                      <input type="checkbox" checked={selected.has(d.id)} onChange={() => toggle(d.id)} aria-label={t("devices.select", { name: d.display_name })} />
                    </td>
                    <td className="max-w-72 px-3 py-2">
                      <NameCell d={d} onEdit={() => setEditing(d)} />
                    </td>
                    <td className="px-3 py-2">
                      <MacCell d={d} />
                    </td>
                    <td className="px-3 py-2 font-mono text-xs tabular-nums">
                      <IPCell d={d} />
                    </td>
                    <td className="px-3 py-2 text-slate-600 dark:text-slate-300">{subnetName.get(d.subnet_id) ?? "—"}</td>
                    <td className="px-3 py-2 whitespace-nowrap text-slate-600 dark:text-slate-300">
                      <span title={t("devices.seenTitle", { last: absolute(d.last_seen, locale), first: absolute(d.first_seen, locale) })}>
                        {relative(d.last_seen, locale)}
                      </span>
                    </td>
                    <td className="px-3 py-2 text-right whitespace-nowrap">
                      <RowActions d={d} onEdit={() => setEditing(d)} after={after} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>

            {/* Mobile: cards */}
            <ul className="divide-y divide-slate-100 md:hidden dark:divide-slate-800">
              {devices.map((d) => (
                <li key={d.id} className="flex gap-3 p-3">
                  <input type="checkbox" className="mt-1.5 self-start" checked={selected.has(d.id)} onChange={() => toggle(d.id)} aria-label={t("devices.select", { name: d.display_name })} />
                  <div className="min-w-0 flex-1 space-y-1">
                    <NameCell d={d} onEdit={() => setEditing(d)} />
                    <MacCell d={d} />
                    <div className="text-xs text-slate-500 dark:text-slate-400">
                      <span className="font-mono">
                        <IPCell d={d} />
                      </span>{" "}
                      · {subnetName.get(d.subnet_id) ?? "—"} · {relative(d.last_seen, locale)}
                    </div>
                    <div className="pt-1">
                      <RowActions d={d} onEdit={() => setEditing(d)} after={after} />
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          </>
        )}
      </Card>

      {editing && (
        <DeviceDialog
          device={editing}
          subnet={subnetName.get(editing.subnet_id)}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            load();
            onChange();
          }}
        />
      )}
    </div>
  );
}

function NameCell({ d, onEdit }: { d: Device; onEdit: () => void }) {
  const { t } = useI18n();
  const title = d.nickname || d.hostname;
  // Second line: the hostname under a nickname, and where the name came from
  const details: string[] = [];
  if (d.nickname && d.hostname) details.push(d.hostname);
  if (d.hostname && d.hostname_source) {
    const src = sourceLabel[d.hostname_source] ?? d.hostname_source;
    details.push(d.nickname ? src : t("devices.nameVia", { source: src }));
  }
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-2">
        <button onClick={onEdit} className="truncate text-left font-medium hover:text-teal-700 hover:underline dark:hover:text-teal-400" title={t("devices.editHint")}>
          {title || <span className="font-normal text-slate-400 italic">{d.vendor ? d.vendor : t("devices.noName")}</span>}
        </button>
        {d.status === "new" ? <Badge tone="new">{t("devices.badgeNew")}</Badge> : null}
      </div>
      {details.length > 0 && <div className="truncate text-xs text-slate-500 dark:text-slate-400">{details.join(" · ")}</div>}
      {d.notes && <div className="truncate text-xs text-slate-500 italic dark:text-slate-400">{d.notes}</div>}
    </div>
  );
}

function MacCell({ d }: { d: Device }) {
  const { t } = useI18n();
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-2">
        <span className="font-mono text-xs">{d.mac}</span>
        {d.random_mac && (
          <Badge tone="warn" title={t("devices.randomHint")}>
            {t("devices.random")}
          </Badge>
        )}
      </div>
      {/* Without a name the vendor is already the title of the row */}
      {d.vendor && (d.nickname || d.hostname) && (
        <div className="max-w-56 truncate text-xs text-slate-500 dark:text-slate-400" title={d.vendor}>
          {d.vendor}
        </div>
      )}
    </div>
  );
}

function IPCell({ d }: { d: Device }) {
  const { t } = useI18n();
  if (!d.extra_ips?.length) return <>{d.ip}</>;
  return (
    <span className="whitespace-nowrap" title={t("devices.extraIPs", { ips: d.extra_ips.join(", ") })}>
      {d.ip}{" "}
      <span className="rounded bg-slate-100 px-1 font-sans text-[11px] text-slate-600 dark:bg-slate-800 dark:text-slate-300">+{d.extra_ips.length}</span>
    </span>
  );
}

function RowActions({ d, onEdit, after }: { d: Device; onEdit: () => void; after: (fn: () => Promise<unknown>) => void }) {
  const { t } = useI18n();
  return (
    <div className="inline-flex gap-1.5">
      {d.status === "new" ? (
        <Button variant="primary" onClick={() => after(() => api.updateDevice(d.id, { status: "known" }))}>
          {t("devices.known")}
        </Button>
      ) : (
        <Button variant="ghost" onClick={() => after(() => api.updateDevice(d.id, { status: "new" }))} title={t("devices.reopenHint")}>
          {t("devices.reopen")}
        </Button>
      )}
      <Button onClick={onEdit}>{t("common.edit")}</Button>
    </div>
  );
}
