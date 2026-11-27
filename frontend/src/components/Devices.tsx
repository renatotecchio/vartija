import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type Device, type DeviceStatus, type Subnet } from "../api";
import { absolute, relative, sourceLabel } from "../format";
import { Badge, Button, Card, Empty, ErrorBox, inputClass } from "./ui";
import DeviceDialog from "./DeviceDialog";

type Filter = DeviceStatus | "";

const filters: { id: Filter; label: string }[] = [
  { id: "new", label: "Novos" },
  { id: "known", label: "Conhecidos" },
  { id: "", label: "Todos" },
];

function initialFilter(): Filter {
  const s = new URLSearchParams(window.location.search).get("status");
  return s === "known" || s === "all" ? (s === "all" ? "" : s) : "new";
}

export default function Devices({ refresh, onChange }: { refresh: number; onChange: () => void }) {
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
      setError(e instanceof Error ? e.message : "Erro ao carregar");
    } finally {
      setLoading(false);
    }
  }, [filter, search, subnetId]);

  useEffect(() => {
    const t = setTimeout(load, search ? 250 : 0);
    return () => clearTimeout(t);
  }, [load, refresh, search]);

  useEffect(() => {
    const t = setInterval(load, 30000);
    return () => clearInterval(t);
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
      setError(e instanceof Error ? e.message : "Erro");
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
    if (!confirm(`Marcar todos os ${newIds.length} dispositivos novos como conhecidos?`)) return;
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
              {f.label}
            </button>
          ))}
        </div>
        <input
          className={`${inputClass} max-w-xs`}
          placeholder="Buscar MAC, IP, nome, notas…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Buscar"
        />
        {subnets.length > 1 && (
          <select className={`${inputClass} w-auto`} value={subnetId} onChange={(e) => setSubnetId(Number(e.target.value))} aria-label="Sub-rede">
            <option value={0}>Todas as sub-redes</option>
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
              Marcar {selected.size} como conhecidos
            </Button>
          )}
          {filter === "new" && newIds.length > 0 && selected.size === 0 && (
            <Button onClick={markAllNew}>Marcar todos os novos como conhecidos</Button>
          )}
        </div>
      </div>

      <ErrorBox message={error} />

      <Card className="overflow-hidden">
        {loading ? (
          <Empty>Carregando…</Empty>
        ) : devices.length === 0 ? (
          <Empty>
            {filter === "new"
              ? subnets.length === 0
                ? "Nenhuma sub-rede cadastrada ainda. Adicione uma na aba Sub-redes."
                : "Nenhum dispositivo novo. Tudo revisado."
              : "Nenhum dispositivo encontrado."}
          </Empty>
        ) : (
          <>
            {/* Desktop: table */}
            <table className="hidden w-full text-sm md:table">
              <thead className="border-b border-slate-200 bg-slate-50 text-left text-xs font-semibold tracking-wide text-slate-500 uppercase dark:border-slate-800 dark:bg-slate-900/60 dark:text-slate-400">
                <tr>
                  <th className="w-10 px-3 py-2">
                    <input type="checkbox" checked={allSelected} onChange={toggleAll} aria-label="Selecionar todos" />
                  </th>
                  <th className="px-3 py-2">Dispositivo</th>
                  <th className="px-3 py-2">MAC</th>
                  <th className="px-3 py-2">IP</th>
                  <th className="px-3 py-2">Sub-rede</th>
                  <th className="px-3 py-2">Visto</th>
                  <th className="px-3 py-2 text-right">Ações</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {devices.map((d) => (
                  <tr key={d.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/40">
                    <td className="px-3 py-2">
                      <input type="checkbox" checked={selected.has(d.id)} onChange={() => toggle(d.id)} aria-label={`Selecionar ${d.display_name}`} />
                    </td>
                    <td className="px-3 py-2">
                      <NameCell d={d} onEdit={() => setEditing(d)} />
                    </td>
                    <td className="px-3 py-2">
                      <MacCell d={d} />
                    </td>
                    <td className="px-3 py-2 font-mono text-xs tabular-nums">{d.ip}</td>
                    <td className="px-3 py-2 text-slate-600 dark:text-slate-300">{subnetName.get(d.subnet_id) ?? "—"}</td>
                    <td className="px-3 py-2 text-slate-600 dark:text-slate-300">
                      <span title={`Visto pela última vez: ${absolute(d.last_seen)}\nPrimeira vez: ${absolute(d.first_seen)}`}>{relative(d.last_seen)}</span>
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
                  <input type="checkbox" className="mt-1" checked={selected.has(d.id)} onChange={() => toggle(d.id)} aria-label={`Selecionar ${d.display_name}`} />
                  <div className="min-w-0 flex-1 space-y-1">
                    <NameCell d={d} onEdit={() => setEditing(d)} />
                    <MacCell d={d} />
                    <div className="text-xs text-slate-500 dark:text-slate-400">
                      <span className="font-mono">{d.ip}</span> · {subnetName.get(d.subnet_id) ?? "—"} · {relative(d.last_seen)}
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
  const secondary = d.nickname && d.hostname ? d.hostname : "";
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-2">
        <button onClick={onEdit} className="truncate text-left font-medium hover:text-teal-700 hover:underline dark:hover:text-teal-400" title="Editar nome e notas">
          {d.nickname || d.hostname || <span className="text-slate-400 italic">sem nome</span>}
        </button>
        {d.status === "new" ? <Badge tone="new">novo</Badge> : null}
      </div>
      {(secondary || d.hostname_source) && (
        <div className="truncate text-xs text-slate-500 dark:text-slate-400">
          {secondary}
          {d.hostname && d.hostname_source && (
            <span className="text-slate-400 dark:text-slate-500">
              {secondary ? " · " : ""}
              {d.nickname ? "" : "nome via "}
              {sourceLabel[d.hostname_source] ?? d.hostname_source}
            </span>
          )}
        </div>
      )}
      {d.notes && <div className="truncate text-xs text-slate-500 italic dark:text-slate-400">{d.notes}</div>}
    </div>
  );
}

function MacCell({ d }: { d: Device }) {
  return (
    <div className="flex items-center gap-2">
      <span className="font-mono text-xs">{d.mac}</span>
      {d.random_mac && (
        <Badge tone="warn" title="Endereço administrado localmente: celulares e notebooks criam um MAC privado por rede">
          aleatório
        </Badge>
      )}
    </div>
  );
}

function RowActions({ d, onEdit, after }: { d: Device; onEdit: () => void; after: (fn: () => Promise<unknown>) => void }) {
  return (
    <div className="inline-flex gap-1.5">
      {d.status === "new" ? (
        <Button variant="primary" onClick={() => after(() => api.updateDevice(d.id, { status: "known" }))}>
          Conhecido
        </Button>
      ) : (
        <Button variant="ghost" onClick={() => after(() => api.updateDevice(d.id, { status: "new" }))} title="Voltar para a lista de novos">
          Reabrir
        </Button>
      )}
      <Button onClick={onEdit}>Editar</Button>
    </div>
  );
}
