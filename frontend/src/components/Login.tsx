import { useState, type FormEvent } from "react";
import { api } from "../api";
import { Button, Card, ErrorBox, inputClass } from "./ui";

export default function Login({ onLogin }: { onLogin: (user: string) => void }) {
  const [user, setUser] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const r = await api.login(user, password);
      onLogin(r.user);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Falha no login");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm p-6">
        <h1 className="mb-1 text-xl font-semibold">Vartija</h1>
        <p className="mb-6 text-sm text-slate-500 dark:text-slate-400">Sentinela de dispositivos na rede</p>
        <form onSubmit={submit} className="space-y-4">
          <label className="block">
            <span className="mb-1 block text-sm font-medium">Usuário</span>
            <input className={inputClass} autoComplete="username" value={user} onChange={(e) => setUser(e.target.value)} required autoFocus />
          </label>
          <label className="block">
            <span className="mb-1 block text-sm font-medium">Senha</span>
            <input className={inputClass} type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </label>
          <ErrorBox message={error} />
          <Button variant="primary" type="submit" disabled={busy} className="w-full">
            {busy ? "Entrando…" : "Entrar"}
          </Button>
        </form>
      </Card>
    </div>
  );
}
