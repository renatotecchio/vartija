const rtf = new Intl.RelativeTimeFormat("pt-BR", { numeric: "auto" });
const dtf = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "medium" });

/** "há 3 minutos", "ontem"... */
export function relative(iso: string | null | undefined): string {
  if (!iso) return "—";
  const diff = (new Date(iso).getTime() - Date.now()) / 1000;
  const abs = Math.abs(diff);
  if (abs < 45) return "agora";
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hour");
  return rtf.format(Math.round(diff / 86400), "day");
}

export function absolute(iso: string | null | undefined): string {
  if (!iso) return "";
  return dtf.format(new Date(iso));
}

export const sourceLabel: Record<string, string> = {
  dhcp: "DHCP",
  dns: "DNS",
  mdns: "mDNS",
  netbios: "NetBIOS",
};
