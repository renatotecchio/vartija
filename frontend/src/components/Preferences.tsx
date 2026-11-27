import { useI18n, type Lang } from "../i18n";
import { useTheme, type Theme } from "../theme";

const order: Theme[] = ["system", "light", "dark"];

function ThemeIcon({ theme }: { theme: Theme }) {
  const common = { "aria-hidden": true, viewBox: "0 0 24 24", className: "h-4 w-4", fill: "none", stroke: "currentColor", strokeWidth: 2, strokeLinecap: "round" as const, strokeLinejoin: "round" as const };
  if (theme === "light")
    return (
      <svg {...common}>
        <circle cx="12" cy="12" r="4" />
        <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
      </svg>
    );
  if (theme === "dark")
    return (
      <svg {...common}>
        <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
      </svg>
    );
  return (
    <svg {...common}>
      <rect x="2" y="3" width="20" height="14" rx="2" />
      <path d="M8 21h8M12 17v4" />
    </svg>
  );
}

const btn =
  "inline-flex h-8 items-center justify-center gap-1 rounded-md border border-slate-300 bg-white px-2 text-xs font-semibold text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200 dark:hover:bg-slate-800";

/** Language (EN/PT) and theme (system → light → dark) switches. */
export default function Preferences() {
  const { lang, setLang, t } = useI18n();
  const { theme, setTheme } = useTheme();
  const next = order[(order.indexOf(theme) + 1) % order.length];
  const other: Lang = lang === "pt" ? "en" : "pt";
  const themeName = t(`theme.${theme}`);

  return (
    <div className="flex items-center gap-1.5">
      <button
        type="button"
        className={btn}
        onClick={() => setLang(other)}
        title={`${t("header.language")}: ${lang === "pt" ? "Português" : "English"}`}
        aria-label={`${t("header.language")}: ${lang === "pt" ? "Português" : "English"}`}
      >
        <span className={lang === "en" ? "text-teal-600 dark:text-teal-400" : "text-slate-400"}>EN</span>
        <span className="text-slate-300 dark:text-slate-600">/</span>
        <span className={lang === "pt" ? "text-teal-600 dark:text-teal-400" : "text-slate-400"}>PT</span>
      </button>
      <button
        type="button"
        className={`${btn} w-8 px-0`}
        onClick={() => setTheme(next)}
        title={`${t("theme.label")}: ${themeName}`}
        aria-label={`${t("theme.label")}: ${themeName}`}
      >
        <ThemeIcon theme={theme} />
      </button>
    </div>
  );
}
