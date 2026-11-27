import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type Lang = "en" | "pt";

const en = {
  "app.tagline": "Network device sentinel",
  "nav.devices": "Devices",
  "nav.subnets": "Subnets",
  "nav.events": "Events",
  "header.scan": "Scan now",
  "header.scanning": "Scanning…",
  "header.logout": "Log out",
  "header.language": "Language",
  "theme.label": "Theme",
  "theme.system": "System",
  "theme.light": "Light",
  "theme.dark": "Dark",

  "login.user": "User",
  "login.password": "Password",
  "login.submit": "Log in",
  "login.submitting": "Logging in…",
  "login.invalid": "Invalid user or password",
  "login.tooMany": "Too many failed logins. Try again later.",
  "login.failed": "Login failed",

  "common.loading": "Loading…",
  "common.error": "Error",
  "common.loadError": "Could not load",
  "common.cancel": "Cancel",
  "common.save": "Save",
  "common.edit": "Edit",

  "devices.filter.new": "New",
  "devices.filter.known": "Known",
  "devices.filter.all": "All",
  "devices.search": "Search MAC, IP, name, vendor, notes…",
  "devices.allSubnets": "All subnets",
  "devices.subnet": "Subnet",
  "devices.markSelected": "Mark {n} as known",
  "devices.markAllNew": "Mark all new as known",
  "devices.markAllConfirm": "Mark all {n} new devices as known?",
  "devices.emptyNoSubnets": "No subnets yet. Add one in the Subnets tab.",
  "devices.emptyNew": "No new devices. Everything is reviewed.",
  "devices.empty": "No devices found.",
  "devices.col.device": "Device",
  "devices.col.mac": "MAC",
  "devices.col.ip": "IP",
  "devices.col.subnet": "Subnet",
  "devices.col.seen": "Seen",
  "devices.col.actions": "Actions",
  "devices.selectAll": "Select all",
  "devices.select": "Select {name}",
  "devices.noName": "no name",
  "devices.badgeNew": "new",
  "devices.nameVia": "name via {source}",
  "devices.random": "random",
  "devices.randomHint": "Locally administered address: phones and laptops make up a private MAC per network",
  "devices.seenTitle": "Last seen: {last}\nFirst seen: {first}",
  "devices.known": "Known",
  "devices.reopen": "Reopen",
  "devices.reopenHint": "Back to the new list",
  "devices.editHint": "Edit name and notes",
  "devices.extraIPs": "Also answers on: {ips}",

  "dialog.mac": "MAC",
  "dialog.ip": "IP",
  "dialog.hostname": "Hostname",
  "dialog.vendor": "Vendor",
  "dialog.subnet": "Subnet",
  "dialog.firstSeen": "First seen",
  "dialog.lastSeen": "Last seen",
  "dialog.name": "Name",
  "dialog.namePlaceholder": "E.g. Living room TV",
  "dialog.nameHint": "Shown instead of the hostname everywhere.",
  "dialog.notes": "Notes",
  "dialog.known": "Known device",
  "dialog.forget": "Forget",
  "dialog.forgetConfirm": "Forget this device? If it shows up again, it comes back as new and raises an alert.",
  "dialog.saveError": "Could not save",
  "dialog.deleteError": "Could not delete",

  "subnets.cidr": "Subnet (CIDR)",
  "subnets.name": "Name",
  "subnets.namePlaceholder": "E.g. Home LAN",
  "subnets.add": "Add",
  "subnets.baselineHint":
    "The first scan of each subnet is the baseline: every MAC comes in as new, without one alert per device. After that, each new MAC raises an alert.",
  "subnets.empty": "No subnets yet.",
  "subnets.paused": "paused",
  "subnets.waiting": "waiting for the first scan",
  "subnets.via": "Via {via}",
  "subnets.via.arp": "ARP sweep",
  "subnets.via.mikrotik": "router ARP table",
  "subnets.uncovered": "No interface or router covers this subnet",
  "subnets.lastScan": "last scan {when}",
  "subnets.found": "{n} devices",
  "subnets.error": "Error: {error}",
  "subnets.scan": "Scan",
  "subnets.pause": "Pause",
  "subnets.resume": "Resume",
  "subnets.rename": "Rename",
  "subnets.remove": "Remove",
  "subnets.renamePrompt": "Subnet name",
  "subnets.removeConfirm": "Stop watching {cidr}? The devices already found stay in the list.",

  "events.empty": "No events yet.",
  "events.type.new_device": "new",
  "events.type.ip_changed": "IP changed",
  "events.type.acknowledged": "known",
  "events.type.baseline": "baseline",
  "events.type.scan_failed": "failure",
  "events.msg.new_device": "New device {name} ({ip}) on {cidr}",
  "events.msg.new_device_vendor": "New device {name} ({ip}, {vendor}) on {cidr}",
  "events.msg.ip_changed": "{name} changed IP: {old_ip} → {ip}",
  "events.msg.acknowledged": "{name} marked as known",
  "events.msg.baseline": "First scan of {cidr}: {count} devices",
  "events.msg.scan_failed": "Scan of {cidr} failed: {error}",
};

export type Key = keyof typeof en;

const pt: Record<Key, string> = {
  "app.tagline": "Sentinela de dispositivos na rede",
  "nav.devices": "Dispositivos",
  "nav.subnets": "Sub-redes",
  "nav.events": "Eventos",
  "header.scan": "Varrer agora",
  "header.scanning": "Varrendo…",
  "header.logout": "Sair",
  "header.language": "Idioma",
  "theme.label": "Tema",
  "theme.system": "Sistema",
  "theme.light": "Claro",
  "theme.dark": "Escuro",

  "login.user": "Usuário",
  "login.password": "Senha",
  "login.submit": "Entrar",
  "login.submitting": "Entrando…",
  "login.invalid": "Usuário ou senha inválidos",
  "login.tooMany": "Muitas tentativas erradas. Tente de novo mais tarde.",
  "login.failed": "Falha no login",

  "common.loading": "Carregando…",
  "common.error": "Erro",
  "common.loadError": "Erro ao carregar",
  "common.cancel": "Cancelar",
  "common.save": "Salvar",
  "common.edit": "Editar",

  "devices.filter.new": "Novos",
  "devices.filter.known": "Conhecidos",
  "devices.filter.all": "Todos",
  "devices.search": "Buscar MAC, IP, nome, fabricante, notas…",
  "devices.allSubnets": "Todas as sub-redes",
  "devices.subnet": "Sub-rede",
  "devices.markSelected": "Marcar {n} como conhecidos",
  "devices.markAllNew": "Marcar todos os novos como conhecidos",
  "devices.markAllConfirm": "Marcar todos os {n} dispositivos novos como conhecidos?",
  "devices.emptyNoSubnets": "Nenhuma sub-rede cadastrada ainda. Adicione uma na aba Sub-redes.",
  "devices.emptyNew": "Nenhum dispositivo novo. Tudo revisado.",
  "devices.empty": "Nenhum dispositivo encontrado.",
  "devices.col.device": "Dispositivo",
  "devices.col.mac": "MAC",
  "devices.col.ip": "IP",
  "devices.col.subnet": "Sub-rede",
  "devices.col.seen": "Visto",
  "devices.col.actions": "Ações",
  "devices.selectAll": "Selecionar todos",
  "devices.select": "Selecionar {name}",
  "devices.noName": "sem nome",
  "devices.badgeNew": "novo",
  "devices.nameVia": "nome via {source}",
  "devices.random": "aleatório",
  "devices.randomHint": "Endereço administrado localmente: celulares e notebooks criam um MAC privado por rede",
  "devices.seenTitle": "Visto pela última vez: {last}\nPrimeira vez: {first}",
  "devices.known": "Conhecido",
  "devices.reopen": "Reabrir",
  "devices.reopenHint": "Voltar para a lista de novos",
  "devices.editHint": "Editar nome e notas",
  "devices.extraIPs": "Também responde em: {ips}",

  "dialog.mac": "MAC",
  "dialog.ip": "IP",
  "dialog.hostname": "Hostname",
  "dialog.vendor": "Fabricante",
  "dialog.subnet": "Sub-rede",
  "dialog.firstSeen": "Primeira vez",
  "dialog.lastSeen": "Última vez",
  "dialog.name": "Nome",
  "dialog.namePlaceholder": "Ex.: TV da sala",
  "dialog.nameHint": "Aparece no lugar do hostname em toda a tela.",
  "dialog.notes": "Notas",
  "dialog.known": "Dispositivo conhecido",
  "dialog.forget": "Esquecer",
  "dialog.forgetConfirm": "Esquecer este dispositivo? Se ele aparecer de novo, volta como novo e gera alerta.",
  "dialog.saveError": "Erro ao salvar",
  "dialog.deleteError": "Erro ao excluir",

  "subnets.cidr": "Sub-rede (CIDR)",
  "subnets.name": "Nome",
  "subnets.namePlaceholder": "Ex.: LAN de casa",
  "subnets.add": "Adicionar",
  "subnets.baselineHint":
    "A primeira varredura de cada sub-rede é a linha de base: todos os MACs entram como novos, sem um alerta por dispositivo. Depois disso, cada MAC novo gera alerta.",
  "subnets.empty": "Nenhuma sub-rede cadastrada.",
  "subnets.paused": "pausada",
  "subnets.waiting": "aguardando primeira varredura",
  "subnets.via": "Via {via}",
  "subnets.via.arp": "varredura ARP",
  "subnets.via.mikrotik": "tabela ARP do roteador",
  "subnets.uncovered": "Nenhuma interface ou roteador cobre esta sub-rede",
  "subnets.lastScan": "última varredura {when}",
  "subnets.found": "{n} dispositivos",
  "subnets.error": "Erro: {error}",
  "subnets.scan": "Varrer",
  "subnets.pause": "Pausar",
  "subnets.resume": "Retomar",
  "subnets.rename": "Renomear",
  "subnets.remove": "Remover",
  "subnets.renamePrompt": "Nome da sub-rede",
  "subnets.removeConfirm": "Parar de monitorar {cidr}? Os dispositivos já encontrados continuam na lista.",

  "events.empty": "Nenhum evento ainda.",
  "events.type.new_device": "novo",
  "events.type.ip_changed": "IP mudou",
  "events.type.acknowledged": "conhecido",
  "events.type.baseline": "linha de base",
  "events.type.scan_failed": "falha",
  "events.msg.new_device": "Novo dispositivo {name} ({ip}) em {cidr}",
  "events.msg.new_device_vendor": "Novo dispositivo {name} ({ip}, {vendor}) em {cidr}",
  "events.msg.ip_changed": "{name} mudou de IP: {old_ip} → {ip}",
  "events.msg.acknowledged": "{name} marcado como conhecido",
  "events.msg.baseline": "Primeira varredura de {cidr}: {count} dispositivos",
  "events.msg.scan_failed": "Falha na varredura de {cidr}: {error}",
};

const dictionaries: Record<Lang, Record<Key, string>> = { en, pt };

export const locales: Record<Lang, string> = { en: "en-GB", pt: "pt-BR" };

const STORAGE_KEY = "vartija.lang";

function initialLang(): Lang {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === "en" || saved === "pt") return saved;
  } catch {
    /* storage unavailable */
  }
  return navigator.language?.toLowerCase().startsWith("pt") ? "pt" : "en";
}

export type T = (key: Key, params?: Record<string, string | number>) => string;

interface I18n {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: T;
  locale: string;
}

const Ctx = createContext<I18n | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(initialLang);

  useEffect(() => {
    document.documentElement.lang = lang === "pt" ? "pt-BR" : "en";
  }, [lang]);

  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    try {
      localStorage.setItem(STORAGE_KEY, l);
    } catch {
      /* storage unavailable */
    }
  }, []);

  const value = useMemo<I18n>(() => {
    const dict = dictionaries[lang];
    const t: T = (key, params) => {
      let s = dict[key] ?? en[key] ?? key;
      if (params) for (const [k, v] of Object.entries(params)) s = s.replaceAll(`{${k}}`, String(v));
      return s;
    };
    return { lang, setLang, t, locale: locales[lang] };
  }, [lang, setLang]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useI18n(): I18n {
  const v = useContext(Ctx);
  if (!v) throw new Error("useI18n outside I18nProvider");
  return v;
}
