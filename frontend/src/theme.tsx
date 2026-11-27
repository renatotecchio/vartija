import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";

export type Theme = "system" | "light" | "dark";

const STORAGE_KEY = "vartija.theme";
const media = () => window.matchMedia("(prefers-color-scheme: dark)");

function saved(): Theme {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === "light" || v === "dark" || v === "system") return v;
  } catch {
    /* storage unavailable */
  }
  return "system";
}

/** Sets data-theme on <html>: the "dark" Tailwind variant keys on it. */
function apply(theme: Theme) {
  const dark = theme === "dark" || (theme === "system" && media().matches);
  const root = document.documentElement;
  root.dataset.theme = dark ? "dark" : "light";
  root.style.colorScheme = dark ? "dark" : "light";
}

/** Runs before the first render, so the page never flashes the wrong theme. */
export function applySavedTheme() {
  apply(saved());
}

const Ctx = createContext<{ theme: Theme; setTheme: (t: Theme) => void } | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(saved);

  useEffect(() => {
    apply(theme);
    if (theme !== "system") return;
    const m = media();
    const onChange = () => apply("system");
    m.addEventListener("change", onChange);
    return () => m.removeEventListener("change", onChange);
  }, [theme]);

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t);
    try {
      localStorage.setItem(STORAGE_KEY, t);
    } catch {
      /* storage unavailable */
    }
  }, []);

  return <Ctx.Provider value={{ theme, setTheme }}>{children}</Ctx.Provider>;
}

export function useTheme() {
  const v = useContext(Ctx);
  if (!v) throw new Error("useTheme outside ThemeProvider");
  return v;
}
