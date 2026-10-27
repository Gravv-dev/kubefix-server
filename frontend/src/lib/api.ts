export const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "ApiError";
  }
}

async function request(
  input: string,
  init: RequestInit = {}
): Promise<any> {
  const url = input.startsWith("http") ? input : `${API_BASE}${input}`;
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init.headers as Record<string, string>),
  };

  // Credentials are required so the browser sends the session cookie cross-origin.
  const res = await fetch(url, {
    ...init,
    headers,
    credentials: "include",
    cache: "no-store",
  });

  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    try {
      const body = await res.json();
      message = body.error || body.message || message;
    } catch {
      // ignore non-json error bodies
    }
    throw new ApiError(res.status, message);
  }

  const contentType = res.headers.get("content-type") || "";
  if (contentType.includes("application/json")) {
    return res.json();
  }
  return res.text();
}

export const api = {
  get: (path: string, init?: RequestInit) => request(path, { ...init, method: "GET" }),
  post: (path: string, body?: any, init?: RequestInit) =>
    request(path, {
      ...init,
      method: "POST",
      body: body ? JSON.stringify(body) : undefined,
    }),
  put: (path: string, body?: any, init?: RequestInit) =>
    request(path, {
      ...init,
      method: "PUT",
      body: body ? JSON.stringify(body) : undefined,
    }),
  patch: (path: string, body?: any, init?: RequestInit) =>
    request(path, {
      ...init,
      method: "PATCH",
      body: body ? JSON.stringify(body) : undefined,
    }),
  del: (path: string, init?: RequestInit) =>
    request(path, { ...init, method: "DELETE" }),
};

export { ApiError };
