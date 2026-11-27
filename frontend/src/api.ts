// Thin client for the Vartija API. Every state-changing request carries the
// X-Vartija header the backend requires (CSRF protection).

export type DeviceStatus = "new" | "known";

export interface Device {
  id: number;
  mac: string;
  nickname: string;
  notes: string;
  hostname: string;
  hostname_source: string;
  display_name: string;
  ip: string;
  subnet_id: number;
  status: DeviceStatus;
  random_mac: boolean;
  first_seen: string;
  last_seen: string;
  acknowledged_at: string | null;
}

export interface Subnet {
  id: number;
  cidr: string;
  name: string;
  enabled: boolean;
  baselined: boolean;
  last_scan_at: string | null;
  last_scan_error: string;
  last_scan_found: number;
  via: string;
}

export interface VartijaEvent {
  id: number;
  type: "new_device" | "ip_changed" | "acknowledged" | "baseline" | "scan_failed";
  device_id: number | null;
  subnet_id: number | null;
  message: string;
  at: string;
}

export interface Summary {
  new: number;
  known: number;
  scanning: boolean;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

let onUnauthorized: () => void = () => {};

export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (method !== "GET") headers["X-Vartija"] = "1";
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, {
    method,
    headers,
    credentials: "same-origin",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 401 && path !== "/api/login") {
    onUnauthorized();
  }
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const data = await res.json();
      if (data?.error) msg = data.error;
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  me: () => request<{ user: string }>("GET", "/api/me"),
  login: (user: string, password: string) =>
    request<{ user: string }>("POST", "/api/login", { user, password }),
  logout: () => request<void>("POST", "/api/logout"),
  summary: () => request<Summary>("GET", "/api/summary"),

  devices: (params: { status?: DeviceStatus | ""; q?: string; subnet_id?: number }) => {
    const qs = new URLSearchParams();
    if (params.status) qs.set("status", params.status);
    if (params.q) qs.set("q", params.q);
    if (params.subnet_id) qs.set("subnet_id", String(params.subnet_id));
    return request<Device[]>("GET", `/api/devices?${qs}`);
  },
  updateDevice: (id: number, patch: Partial<Pick<Device, "nickname" | "notes" | "status">>) =>
    request<Device>("PATCH", `/api/devices/${id}`, patch),
  deleteDevice: (id: number) => request<void>("DELETE", `/api/devices/${id}`),
  acknowledge: (ids: number[]) =>
    request<{ acknowledged: number }>("POST", "/api/devices/acknowledge", { ids }),
  acknowledgeAll: () =>
    request<{ acknowledged: number }>("POST", "/api/devices/acknowledge", { all: true }),

  subnets: () => request<Subnet[]>("GET", "/api/subnets"),
  createSubnet: (cidr: string, name: string) =>
    request<Subnet>("POST", "/api/subnets", { cidr, name }),
  updateSubnet: (id: number, patch: Partial<Pick<Subnet, "name" | "enabled">>) =>
    request<Subnet>("PATCH", `/api/subnets/${id}`, patch),
  deleteSubnet: (id: number) => request<void>("DELETE", `/api/subnets/${id}`),
  scanSubnet: (id: number) => request<void>("POST", `/api/subnets/${id}/scan`),
  scanAll: () => request<void>("POST", "/api/scan"),

  events: (limit = 200) => request<VartijaEvent[]>("GET", `/api/events?limit=${limit}`),
};
