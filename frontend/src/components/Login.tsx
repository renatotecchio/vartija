import { useState, type FormEvent } from "react";
import { api, ApiError } from "../api";
import { useI18n } from "../i18n";
import Preferences from "./Preferences";
import { Button, Card, ErrorBox, Shield, inputClass } from "./ui";

export default function Login({ onLogin }: { onLogin: (user: string) => void }) {
  const { t } = useI18n();
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
      if (err instanceof ApiError && err.status === 401) setError(t("login.invalid"));
      else if (err instanceof ApiError && err.status === 429) setError(t("login.tooMany"));
      else setError(err instanceof Error ? err.message : t("login.failed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-screen flex-col items-center justify-center px-4">
      <div className="mb-3 flex w-full max-w-sm justify-end">
        <Preferences />
      </div>
      <Card className="w-full max-w-sm p-6">
        <div className="mb-1 flex items-center gap-2">
          <Shield />
          <h1 className="text-xl font-semibold">Vartija</h1>
        </div>
        <p className="mb-6 text-sm text-slate-500 dark:text-slate-400">{t("app.tagline")}</p>
        <form onSubmit={submit} className="space-y-4">
          <label className="block">
            <span className="mb-1 block text-sm font-medium">{t("login.user")}</span>
            <input className={inputClass} autoComplete="username" value={user} onChange={(e) => setUser(e.target.value)} required autoFocus />
          </label>
          <label className="block">
            <span className="mb-1 block text-sm font-medium">{t("login.password")}</span>
            <input className={inputClass} type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </label>
          <ErrorBox message={error} />
          <Button variant="primary" type="submit" disabled={busy} className="w-full">
            {busy ? t("login.submitting") : t("login.submit")}
          </Button>
        </form>
      </Card>
    </div>
  );
}
