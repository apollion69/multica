import { afterEach, describe, expect, it, vi } from "vitest";
import { WSClient } from "./ws-client";

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  static readonly OPEN = 1;

  readonly url: string;
  readyState = FakeWebSocket.OPEN;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  sent: string[] = [];

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  send(message: string) {
    this.sent.push(message);
  }

  close() {
    this.readyState = 3;
  }
}

describe("WSClient", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    FakeWebSocket.instances = [];
  });

  it("reconnects with bounded exponential backoff after transient disconnects", () => {
    vi.useFakeTimers();
    vi.stubGlobal("WebSocket", FakeWebSocket);

    const client = new WSClient("ws://api.example.test/ws", { cookieAuth: true });
    client.setAuth(null, "team-a");
    client.connect();

    expect(FakeWebSocket.instances).toHaveLength(1);
    FakeWebSocket.instances[0]!.onclose?.();
    expect(FakeWebSocket.instances).toHaveLength(1);

    vi.advanceTimersByTime(999);
    expect(FakeWebSocket.instances).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(FakeWebSocket.instances).toHaveLength(2);

    FakeWebSocket.instances[1]!.onclose?.();
    vi.advanceTimersByTime(1_999);
    expect(FakeWebSocket.instances).toHaveLength(2);
    vi.advanceTimersByTime(1);
    expect(FakeWebSocket.instances).toHaveLength(3);

    client.disconnect();
  });

  it("stops reconnecting and reports confirmed invalid token errors", () => {
    vi.useFakeTimers();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const onInvalidSession = vi.fn();

    const client = new WSClient("ws://api.example.test/ws", {
      onInvalidSession,
    });
    client.setAuth("token", "team-a");
    client.connect();

    const ws = FakeWebSocket.instances[0]!;
    ws.onopen?.();
    expect(ws.sent[0]).toContain('"auth"');

    ws.onmessage?.({ data: JSON.stringify({ error: "invalid token" }) });
    expect(onInvalidSession).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(30_000);
    expect(FakeWebSocket.instances).toHaveLength(1);
  });
});
