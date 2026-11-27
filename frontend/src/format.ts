/** "3 minutes ago", "há 3 minutos", "yesterday"... in the given locale. */
export function relative(iso: string | null | undefined, locale: string, now = Date.now()): string {
  if (!iso) return "—";
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const diff = (new Date(iso).getTime() - now) / 1000;
  const abs = Math.abs(diff);
  if (abs < 45) return rtf.format(0, "second");
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hour");
  return rtf.format(Math.round(diff / 86400), "day");
}

export function absolute(iso: string | null | undefined, locale: string): string {
  if (!iso) return "";
  return new Intl.DateTimeFormat(locale, { dateStyle: "short", timeStyle: "medium" }).format(new Date(iso));
}

export const sourceLabel: Record<string, string> = {
  dhcp: "DHCP",
  dns: "DNS",
  mdns: "mDNS",
  netbios: "NetBIOS",
};
