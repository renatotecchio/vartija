import { useCallback, useEffect, useState } from "react";
import { api, setUnauthorizedHandler, type Summary } from "./api";
import Login from "./components/Login";
import Devices from "./components/Devices";
import Subnets from "./components/Subnets";
import Events from "./components/Events";
import { Button } from "./components/ui";

type Tab = "devices" | "subnets" | "events";

const tabs: { id: Tab; label: string }[] = [
  { id: "devices", label: "Dispositivos" },
  { id: "subnets", label: "Sub-redes" },
  { id: "events", label: "Eventos" },
];

function Shield() {
  return (
    <svg aria-hidden viewBox="0 0 24 24" className="h-6 w-6 text-teal-600" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
      <circle cx="12" cy="11" r="3" />
    </svg>
  );
}

export default function App() {
  const [user, setUser] = useState<string | null | undefined>(undefined);
  const [tab, setTab] = useState<Tab>("devices");
  const [summary, setSummary] = useState<Summary | null>(null);
  // Bumped after a scan or a change, so the open tab reloads
  const [refresh, setRefresh] = useState(0);
  const bump = useCallback(() => setRefresh((n) => n + 1), []);

  useEffect(() => {
    setUnauthorizedHandler(() => setUser(null));
    api.me().then((r) => setUser(r.user)).catch(() => setUser(null));
  }, []);

  const loadSummary = useCallback(() => {
    api.summary().then(setSummary).catch(() => {});
  }, []);

  useEffect(() => {
    if (!user) return;
    loadSummary();
    const t = setInterval(loadSummary, 15000);
    return () => clearInterval(t);
  }, [user, loadSummary, refresh]);

  // While a scan runs, poll faster and reload the tab when it ends
  useEffect(() => {
    if (!summary?.scanning) return;
    const t = setInterval(() => {
      api.summary().then((s) => {
        setSummary(s);
        if (!s.scanning) bump();
      });
    }, 2000);
    return () => clearInterval(t);
  }, [summary?.scanning, bump]);

  if (user === undefined) return null;
  if (user === null) return <Login onLogin={(u) => setUser(u)} />;

  const scanAll = async () => {
    try {
      await api.scanAll();
    } catch {
      /* already running: the summary shows it */
    }
    setTimeout(loadSummary, 300);
  };

  const logout = async () => {
    await api.logout().catch(() => {});
    setUser(null);
  };

  return (
    <div className="min-h-screen">
      <header className="border-b border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
          <div className="flex items-center gap-2">
            <Shield />
            <span className="text-lg font-semibold tracking-tight">Vartija</span>
          </div>
          <nav className="order-3 flex w-full gap-1 overflow-x-auto sm:order-none sm:w-auto">
            {tabs.map((t) => (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className={`rounded-md px-3 py-1.5 text-sm font-medium whitespace-nowrap ${
                  tab === t.id
                    ? "bg-slate-100 text-slate-900 dark:bg-slate-800 dark:text-white"
                    : "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white"
                }`}
              >
                {t.label}
                {t.id === "devices" && summary && summary.new > 0 && (
                  <span className="ml-1.5 rounded-full bg-amber-500 px-1.5 py-0.5 text-xs font-semibold text-white">{summary.new}</span>
                )}
              </button>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <Button variant="primary" onClick={scanAll} disabled={summary?.scanning}>
              {summary?.scanning ? "Varrendo…" : "Varrer agora"}
            </Button>
            <span className="hidden text-sm text-slate-500 md:inline dark:text-slate-400">{user}</span>
            <Button variant="ghost" onClick={logout}>
              Sair
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-6">
        {tab === "devices" && <Devices refresh={refresh} onChange={loadSummary} />}
        {tab === "subnets" && <Subnets refresh={refresh} onScan={() => setTimeout(loadSummary, 300)} />}
        {tab === "events" && <Events refresh={refresh} />}
      </main>
    </div>
  );
}
