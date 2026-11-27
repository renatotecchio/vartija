import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type Subnet } from "../api";
import { absolute, relative } from "../format";
import { Badge, Button, Card, Empty, ErrorBox, inputClass } from "./ui";

const viaLabel: Record<string, string> = {
  arp: "varredura ARP",
  mikrotik: "tabela ARP do roteador",
};

export default function Subnets({ refresh, onScan }: { refresh: number; onScan: () => void }) {
  const [subnets, setSubnets] = useState<Subnet[]>([]);
  const [cidr, setCidr] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      setSubnets(await api.subnets());
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Erro ao carregar");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load, refresh]);

  const run = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      setError("");
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Erro");
    }
  };

  const add = (e: FormEvent) => {
    e.preventDefault();
    run(async () => {
      await api.createSubnet(cidr, name);
      setCidr("");
      setName("");
      onScan();
    });
  };

  const rename = (s: Subnet) => {
    const n = prompt("Nome da sub-rede", s.name);
    if (n !== null) run(() => api.updateSubnet(s.id, { name: n }));
  };

  const remove = (s: Subnet) => {
    if (confirm(`Parar de monitorar ${s.cidr}? Os dispositivos já encontrados continuam na lista.`)) run(() => api.deleteSubnet(s.id));
  };

  return (
    <div className="space-y-4">
      <Card className="p-4">
        <form onSubmit={add} className="flex flex-wrap items-end gap-3">
          <label className="block">
            <span className="mb-1 block text-sm font-medium">Sub-rede (CIDR)</span>
            <input className={`${inputClass} w-48 font-mono`} placeholder="192.168.1.0/24" value={cidr} onChange={(e) => setCidr(e.target.value)} required />
          </label>
          <label className="block min-w-48 flex-1">
            <span className="mb-1 block text-sm font-medium">Nome</span>
            <input className={inputClass} placeholder="Ex.: LAN de casa" value={name} maxLength={100} onChange={(e) => setName(e.target.value)} />
          </label>
          <Button type="submit" variant="primary">
            Adicionar
          </Button>
        </form>
        <p className="mt-3 text-xs text-slate-500 dark:text-slate-400">
          A primeira varredura de cada sub-rede é a linha de base: todos os MACs entram como novos, sem um alerta por dispositivo. Depois disso, cada MAC novo gera alerta.
        </p>
      </Card>

      <ErrorBox message={error} />

      <Card className="overflow-hidden">
        {loading ? (
          <Empty>Carregando…</Empty>
        ) : subnets.length === 0 ? (
          <Empty>Nenhuma sub-rede cadastrada.</Empty>
        ) : (
          <ul className="divide-y divide-slate-100 dark:divide-slate-800">
            {subnets.map((s) => (
              <li key={s.id} className="flex flex-wrap items-start gap-3 p-4">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-sm font-semibold">{s.cidr}</span>
                    {s.name && <span className="text-sm text-slate-600 dark:text-slate-300">{s.name}</span>}
                    {!s.enabled && <Badge tone="muted">pausada</Badge>}
                    {!s.baselined && s.enabled && <Badge tone="new">aguardando primeira varredura</Badge>}
                  </div>
                  <div className="text-xs text-slate-500 dark:text-slate-400">
                    {s.via ? (
                      <>Via {viaLabel[s.via] ?? s.via}</>
                    ) : (
                      <span className="text-red-600 dark:text-red-400">Nenhuma interface ou roteador cobre esta sub-rede</span>
                    )}
                    {s.last_scan_at && (
                      <>
                        {" · "}
                        <span title={absolute(s.last_scan_at)}>última varredura {relative(s.last_scan_at)}</span>
                        {!s.last_scan_error && <> · {s.last_scan_found} dispositivos</>}
                      </>
                    )}
                  </div>
                  {s.last_scan_error && <div className="text-xs text-red-600 dark:text-red-400">Erro: {s.last_scan_error}</div>}
                </div>
                <div className="flex flex-wrap gap-1.5">
                  <Button
                    onClick={() =>
                      run(async () => {
                        await api.scanSubnet(s.id);
                        onScan();
                      })
                    }
                    disabled={!s.enabled}
                  >
                    Varrer
                  </Button>
                  <Button variant="ghost" onClick={() => run(() => api.updateSubnet(s.id, { enabled: !s.enabled }))}>
                    {s.enabled ? "Pausar" : "Retomar"}
                  </Button>
                  <Button variant="ghost" onClick={() => rename(s)}>
                    Renomear
                  </Button>
                  <Button variant="danger" onClick={() => remove(s)}>
                    Remover
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
