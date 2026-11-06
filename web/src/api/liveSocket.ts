// liveSocket manages the browser WebSocket to /ws/ui with automatic reconnect.
// It is a tiny pub/sub: consumers subscribe to parsed UIMessages.

import type { UIMessage } from "./types";

type Listener = (msg: UIMessage) => void;

export class LiveSocket {
  private ws: WebSocket | null = null;
  private listeners = new Set<Listener>();
  private closed = false;
  private backoffMs = 1000;
  private readonly maxBackoffMs = 15000;

  constructor(private readonly url: string = defaultWsURL()) {}

  start(): void {
    this.closed = false;
    this.connect();
  }

  stop(): void {
    this.closed = true;
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  subscribe(fn: Listener): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  private connect(): void {
    if (this.closed) return;
    const ws = new WebSocket(this.url);
    this.ws = ws;

    ws.onopen = () => {
      this.backoffMs = 1000;
    };
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data as string) as UIMessage;
        this.listeners.forEach((l) => l(msg));
      } catch {
        // Ignore malformed frames.
      }
    };
    ws.onclose = () => {
      this.ws = null;
      if (!this.closed) this.scheduleReconnect();
    };
    ws.onerror = () => {
      ws.close();
    };
  }

  private scheduleReconnect(): void {
    const delay = this.backoffMs;
    this.backoffMs = Math.min(this.backoffMs * 2, this.maxBackoffMs);
    setTimeout(() => this.connect(), delay);
  }
}

function defaultWsURL(): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}/ws/ui`;
}
