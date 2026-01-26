import {applyEvent, newMessageBuffer} from "./reconcile";

export type TokenProvider = () => Promise<string | null> | string | null;

export type ClientOptions = {
  baseURL: string;
  tokenProvider?: TokenProvider;
  useCookies?: boolean;
  headers?: Record<string, string>;
  retries?: number;
  retryDelayMs?: number;
  retryStatuses?: number[];
  timeoutMs?: number;
  fetchImpl?: typeof fetch;
};

export type CreateConversationRequest = {
  model?: string;
  agent?: string;
  tools?: string;
  title?: string;
  visibility?: string;
};

export type CreateConversationResponse = {
  id: string;
  title: string;
  createdAt: string;
  model?: string;
  agent?: string;
  tools?: string;
};

export type PostMessageRequest = {
  content: string;
  agent?: string;
  model?: string;
  tools?: string[];
  context?: Record<string, any>;
  toolCallExposure?: string;
  autoSummarize?: boolean;
  autoSelectTools?: boolean;
  disableChains?: boolean;
  allowedChains?: string[];
  attachments?: UploadedAttachment[];
};

export type PostMessageResponse = {
  id: string;
};

export type UploadedAttachment = {
  name: string;
  size?: number;
  stagingFolder?: string;
  uri: string;
  mime?: string;
};

export type UploadResponse = {
  name: string;
  size?: number;
  uri: string;
  stagingFolder?: string;
};

export type ToolRunRequest = {
  service: string;
  method: string;
  args?: Record<string, any>;
};

export type PayloadResponse = {
  contentType: string;
  body: ArrayBuffer;
};

export type AuthProvider = {
  name?: string;
  label?: string;
  mode?: string;
};

export class AgentlyClient {
  private baseURL: string;
  private tokenProvider?: TokenProvider;
  private useCookies: boolean;
  private headers: Record<string, string>;
  private retries: number;
  private retryDelayMs: number;
  private retryStatuses: Set<number>;
  private timeoutMs: number;
  private fetchImpl: typeof fetch;

  constructor(opts: ClientOptions) {
    this.baseURL = opts.baseURL.replace(/\/+$/, "");
    this.tokenProvider = opts.tokenProvider;
    this.useCookies = !!opts.useCookies;
    this.headers = opts.headers || {};
    this.retries = typeof opts.retries === "number" ? opts.retries : 1;
    this.retryDelayMs = typeof opts.retryDelayMs === "number" ? opts.retryDelayMs : 200;
    this.retryStatuses = new Set(opts.retryStatuses || [429, 502, 503, 504]);
    this.timeoutMs = typeof opts.timeoutMs === "number" ? opts.timeoutMs : 0;
    this.fetchImpl = opts.fetchImpl || fetch;
  }

  async createConversation(req: CreateConversationRequest): Promise<CreateConversationResponse> {
    return this.request("POST", "/v1/api/conversations", req);
  }

  async listConversations(): Promise<any[]> {
    return this.request("GET", "/v1/api/conversations");
  }

  async postMessage(conversationId: string, req: PostMessageRequest): Promise<PostMessageResponse> {
    if (!conversationId) {
      throw new Error("conversationId is required");
    }
    return this.request("POST", `/v1/api/conversations/${encodeURIComponent(conversationId)}/messages`, req);
  }

  async getMessages(conversationId: string, sinceOrOpts?: string | {since?: string; includeModelCallPayload?: boolean; includeLinked?: boolean}): Promise<any> {
    if (!conversationId) {
      throw new Error("conversationId is required");
    }
    let since = "";
    let includeModelCallPayload = false;
    let includeLinked = false;
    if (typeof sinceOrOpts === "string") {
      since = sinceOrOpts;
    } else if (sinceOrOpts) {
      since = sinceOrOpts.since || "";
      includeModelCallPayload = !!sinceOrOpts.includeModelCallPayload;
      includeLinked = !!sinceOrOpts.includeLinked;
    }
    const params = new URLSearchParams();
    if (since) params.set("since", since);
    if (includeModelCallPayload) params.set("includeModelCallPayload", "1");
    if (includeLinked) params.set("includeLinked", "1");
    const qs = params.toString();
    return this.request("GET", `/v1/api/conversations/${encodeURIComponent(conversationId)}/messages${qs ? "?" + qs : ""}`);
  }

  streamEvents(conversationId: string, opts?: {since?: string; include?: string[]; onEvent?: (ev: any) => void; onError?: (err: any) => void;}): EventSource {
    if (!conversationId) {
      throw new Error("conversationId is required");
    }
    const params = new URLSearchParams();
    if (opts?.since) params.set("since", opts.since);
    if (opts?.include?.length) params.set("include", opts.include.join(","));
    const url = `${this.baseURL}/v1/api/conversations/${encodeURIComponent(conversationId)}/events?${params.toString()}`;
    const es = new EventSource(url, {withCredentials: this.useCookies});
    es.addEventListener("message", (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data);
        opts?.onEvent?.(data);
      } catch (err) {
        opts?.onError?.(err);
      }
    });
    es.addEventListener("delta", (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data);
        opts?.onEvent?.(data);
      } catch (err) {
        opts?.onError?.(err);
      }
    });
    es.onerror = (err) => {
      opts?.onError?.(err);
    };
    return es;
  }

  streamConversation(conversationId: string, opts?: {since?: string; onUpdate?: (u: {id: string; text: string; final: boolean}) => void; onError?: (err: any) => void;}): EventSource {
    const buffer = newMessageBuffer();
    return this.streamEvents(conversationId, {
      since: opts?.since,
      include: ["text", "tool_op", "control"],
      onEvent: (ev) => {
        const update = applyEvent(buffer, ev);
        if (update) opts?.onUpdate?.(update);
      },
      onError: opts?.onError,
    });
  }

  async pollEvents(conversationId: string, opts?: {since?: string; include?: string[]; waitMs?: number}): Promise<any> {
    if (!conversationId) {
      throw new Error("conversationId is required");
    }
    const params = new URLSearchParams();
    if (opts?.since) params.set("since", opts.since);
    if (opts?.include?.length) params.set("include", opts.include.join(","));
    if (opts?.waitMs && opts.waitMs > 0) params.set("wait", String(opts.waitMs));
    const url = `/v1/api/conversations/${encodeURIComponent(conversationId)}/events?${params.toString()}`;
    return this.request("GET", url);
  }

  async postAndStream(conversationId: string, req: PostMessageRequest, opts?: {include?: string[]; onEvent?: (ev: any) => void; onError?: (err: any) => void;}): Promise<{id: string; stream: EventSource}> {
    const resp = await this.postMessage(conversationId, req);
    const stream = this.streamEvents(conversationId, {include: opts?.include, onEvent: opts?.onEvent, onError: opts?.onError});
    return {id: resp.id, stream};
  }

  async postAndStreamConversation(conversationId: string, req: PostMessageRequest, opts?: {onUpdate?: (u: {id: string; text: string; final: boolean}) => void; onError?: (err: any) => void;}): Promise<{id: string; stream: EventSource}> {
    const resp = await this.postMessage(conversationId, req);
    const stream = this.streamConversation(conversationId, {onUpdate: opts?.onUpdate, onError: opts?.onError});
    return {id: resp.id, stream};
  }

  async runTool(conversationId: string, req: ToolRunRequest): Promise<any> {
    if (!conversationId) {
      throw new Error("conversationId is required");
    }
    if (!req?.service) {
      throw new Error("tool service is required");
    }
    return this.request("POST", `/v1/api/conversations/${encodeURIComponent(conversationId)}/tools/run`, req);
  }

  async uploadAttachment(file: File | Blob, name?: string): Promise<UploadResponse> {
    if (!file) {
      throw new Error("file is required");
    }
    const url = `${this.baseURL}/upload`;
    const form = new FormData();
    const filename = name || (file instanceof File ? file.name : "upload");
    form.append("file", file, filename);

    const headers: Record<string, string> = {...this.headers};
    const token = this.tokenProvider ? await this.tokenProvider() : null;
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
    const resp = await this.fetchImpl(url, {
      method: "POST",
      headers,
      body: form,
      credentials: this.useCookies ? "include" : "same-origin",
    });
    if (!resp.ok) {
      const text = await resp.text().catch(() => "");
      throw new Error(`${resp.status} ${resp.statusText}: ${text}`);
    }
    return resp.json();
  }

  async authProviders(): Promise<AuthProvider[]> {
    return this.request("GET", "/v1/api/auth/providers");
  }

  async authMe(): Promise<any> {
    return this.request("GET", "/v1/api/auth/me");
  }

  async authLogout(): Promise<void> {
    await this.request("POST", "/v1/api/auth/logout", {});
  }

  async authLocalLogin(name: string): Promise<void> {
    if (!name) {
      throw new Error("name is required");
    }
    await this.request("POST", "/v1/api/auth/local/login", {name});
  }

  async authOAuthInitiate(): Promise<{authURL: string}> {
    return this.request("POST", "/v1/api/auth/oauth/initiate", {});
  }

  async getPayload(payloadId: string): Promise<PayloadResponse> {
    if (!payloadId) {
      throw new Error("payloadId is required");
    }
    const url = `${this.baseURL}/v1/api/payloads/${encodeURIComponent(payloadId)}`;
    const headers: Record<string, string> = {...this.headers};
    const token = this.tokenProvider ? await this.tokenProvider() : null;
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
    const resp = await this.fetchImpl(url, {
      method: "GET",
      headers,
      credentials: this.useCookies ? "include" : "same-origin",
    });
    if (!resp.ok) {
      const text = await resp.text().catch(() => "");
      throw new Error(`${resp.status} ${resp.statusText}: ${text}`);
    }
    const body = await resp.arrayBuffer();
    const contentType = resp.headers.get("Content-Type") || "";
    return {contentType, body};
  }

  private async request(method: string, path: string, body?: any): Promise<any> {
    const url = `${this.baseURL}${path}`;
    const headers: Record<string, string> = {...this.headers};
    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
    }
    const maxAttempts = Math.max(1, this.retries);
    let lastErr: any = null;
    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      const token = this.tokenProvider ? await this.tokenProvider() : null;
      if (token) {
        headers["Authorization"] = `Bearer ${token}`;
      } else {
        delete headers["Authorization"];
      }
      const controller = this.timeoutMs > 0 ? new AbortController() : null;
      const timer = controller ? setTimeout(() => controller.abort(), this.timeoutMs) : null;
      try {
        const resp = await this.fetchImpl(url, {
          method,
          headers,
          body: body !== undefined ? JSON.stringify(body) : undefined,
          credentials: this.useCookies ? "include" : "same-origin",
          signal: controller ? controller.signal : undefined,
        });
        if (!resp.ok) {
          const text = await resp.text().catch(() => "");
          lastErr = new Error(`${resp.status} ${resp.statusText}: ${text}`);
          if (!this.shouldRetry(method, resp.status) || attempt === maxAttempts) {
            throw lastErr;
          }
          await this.sleep(this.retryDelayMs);
          continue;
        }
        return resp.json();
      } catch (err) {
        lastErr = err;
        if (!this.shouldRetry(method, 0) || attempt === maxAttempts) {
          throw err;
        }
        await this.sleep(this.retryDelayMs);
      } finally {
        if (timer) clearTimeout(timer);
      }
    }
    throw lastErr || new Error("request failed");
  }

  private shouldRetry(method: string, status: number): boolean {
    const m = method.toUpperCase();
    if (m !== "GET" && m !== "HEAD") return false;
    if (status <= 0) return true;
    return this.retryStatuses.has(status);
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
