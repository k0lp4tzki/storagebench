// LiveProvider owns the single WebSocket connection and exposes live agent
// status and a rolling buffer of recent metric samples per agent. Components
// consume it via the useLive hook.

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api } from "../api/client";
import { LiveSocket } from "../api/liveSocket";
import type {
  AgentView,
  MetricSample,
  UIBatchPayload,
  UIMessage,
  UIMetricsPayload,
  UITestStatePayload,
} from "../api/types";

// How many recent samples to keep per agent for live sparklines/charts.
const MAX_LIVE_SAMPLES = 300;

interface LiveState {
  agents: AgentView[];
  // liveMetrics maps agentId -> recent samples (oldest first).
  liveMetrics: Record<string, MetricSample[]>;
  // testStates maps agentId -> latest phase for the active run.
  testStates: Record<string, { runId: string; phase: string }>;
  // batchProgress maps batchId -> latest progress.
  batchProgress: Record<string, UIBatchPayload>;
  connected: boolean;
}

const LiveContext = createContext<LiveState | null>(null);

export function LiveProvider({ children }: { children: ReactNode }) {
  const [agents, setAgents] = useState<AgentView[]>([]);
  const [liveMetrics, setLiveMetrics] = useState<Record<string, MetricSample[]>>(
    {},
  );
  const [testStates, setTestStates] = useState<
    Record<string, { runId: string; phase: string }>
  >({});
  const [batchProgress, setBatchProgress] = useState<
    Record<string, UIBatchPayload>
  >({});
  const [connected, setConnected] = useState(false);
  const socketRef = useRef<LiveSocket | null>(null);

  useEffect(() => {
    let cancelled = false;
    const loadAgents = () => {
      api
        .agents()
        .then((list) => {
          if (cancelled) return;
          setAgents(list ?? []);
        })
        .catch(() => {});
    };
    loadAgents();
    const poll = window.setInterval(loadAgents, 4000);

    const sock = new LiveSocket();
    socketRef.current = sock;

    const unsubConn = sock.onConnection((up) => {
      if (!cancelled) setConnected(up);
    });

    const unsub = sock.subscribe((msg: UIMessage) => {
      switch (msg.type) {
        case "agents": {
          setAgents(msg.payload as AgentView[]);
          setConnected(true);
          break;
        }
        case "metrics": {
          const p = msg.payload as UIMetricsPayload;
          setLiveMetrics((prev) => {
            const existing = prev[p.agentId] ?? [];
            const merged = [...existing, ...p.samples];
            const trimmed =
              merged.length > MAX_LIVE_SAMPLES
                ? merged.slice(merged.length - MAX_LIVE_SAMPLES)
                : merged;
            return { ...prev, [p.agentId]: trimmed };
          });
          break;
        }
        case "testState": {
          const p = msg.payload as UITestStatePayload;
          setTestStates((prev) => ({
            ...prev,
            [p.agentId]: { runId: p.state.runId, phase: p.state.phase },
          }));
          break;
        }
        case "batch": {
          const p = msg.payload as UIBatchPayload;
          setBatchProgress((prev) => ({ ...prev, [p.batchId]: p }));
          break;
        }
        default:
          break;
      }
    });

    sock.start();
    return () => {
      cancelled = true;
      window.clearInterval(poll);
      unsub();
      unsubConn();
      sock.stop();
    };
  }, []);

  const value = useMemo<LiveState>(
    () => ({ agents, liveMetrics, testStates, batchProgress, connected }),
    [agents, liveMetrics, testStates, batchProgress, connected],
  );

  return <LiveContext.Provider value={value}>{children}</LiveContext.Provider>;
}

export function useLive(): LiveState {
  const ctx = useContext(LiveContext);
  if (!ctx) throw new Error("useLive must be used within LiveProvider");
  return ctx;
}
