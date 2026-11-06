# Architecture & Phase 1 Concept

This document records the technical concept and the decisions behind
`storagebench`. It is the reference for *why* the system is built the way it is.

## 1. Goal

A small, reliable platform for a storage team to **benchmark** storage
attachments and **test their resilience** under fault conditions across three
Linux servers (SLES 15). It must answer not only "how fast is the storage?" but
"how does it behave when the network or a path fails, and how long does recovery
take?" — with faults correlated to performance on a shared timeline.

The guiding principle: **build the right system, not the most software.**

## 2. Architecture: Controller + Agent (not a cluster)

Three identical Linux hosts. Each runs one **agent**. Exactly one host also runs
the **controller** (coordinator + web UI).

```
          Browser ──HTTPS──> Controller (Host A)
                              │  REST API + WebSocket hub + SQLite
                              │
   Host A: Controller + Agent │  (agent is a separate systemd service,
                              │   driven over the same WS interface)
                              ├── wss ──> Agent (Host B) ── privilege-broker
                              └── wss ──> Agent (Host C) ── privilege-broker
```

### Why this instead of something "bigger"?

The requirement mentioned a Master/Client model and asked whether a simpler,
more robust alternative exists. The chosen shape *is* that alternative:

- **No distributed consensus, no message broker, no Kubernetes.** For three nodes
  these add operational cost and failure modes with no benefit.
- **The controller host is also a full test node**, satisfied cleanly by running
  the *same agent binary* there as a separate service. The controller talks to
  its local agent over the identical WebSocket interface used for remote agents.
  This gives **one code path for all nodes** and prevents the controller's own
  coordination/telemetry from being skewed by heavy benchmark load on that host.

### Responsibilities

| Component | Responsibility |
|---|---|
| Controller | Serve UI, REST API, WebSocket hub, orchestrate runs, persist results, compare/report |
| Agent (×3) | Run fio/diagnostics, sample host metrics, stream telemetry, buffer & resync on disconnect, keep process lifecycle clean |
| Privilege-broker (×3) | The only path to root actions; executes allowlisted, typed operations with validated arguments (lands in the resilience phase) |

## 3. Technology decisions

| Area | Choice | Rationale |
|---|---|---|
| Language | **Go** | Static, dependency-free Linux binaries; great concurrency for "run a process + sample + stream" simultaneously; tiny footprint. |
| Transport | **WebSocket over TLS** | Bidirectional, push-capable, one durable connection per agent. |
| Channels | control + telemetry over one socket | Logical separation of control vs. data as required, without a second connection. |
| Persistence | **SQLite (WAL)** | Sufficient for three nodes; one file; no server; trivial backups. |
| Time series | In SQLite (not Prometheus/Influx) | We need *run-scoped* series, not continuous observability. Keeps the stack small. |
| Benchmark engine | **fio** (+ optional dd, ioping) | Industry standard with structured output; no custom I/O engine. |
| Auth | **Pre-shared token + TLS** | Simple and robust for three trusted nodes; mTLS can be layered on later without message changes. |
| Web UI | React + Vite + TS, lightweight charts (e.g. uPlot) | Modern, clean UI for dense live time-series; shipped as a static bundle served by the controller. |

Net result: **two Go binaries + one SQLite file + one static JS bundle.**

## 4. Communication & resilience

Every frame is a JSON `Envelope { ch, type, ts, payload }` (see
`internal/protocol`). Two logical channels: `control` and `telemetry`.

- **Handshake:** agent sends `hello` (token + capabilities + system info);
  controller replies `welcome` (or `reject`). Protocol version is checked.
- **Heartbeat:** controller pings on an interval; missing pongs move an agent
  `online → degraded → offline`. RTT is tracked from the pong.
- **No lost data on disconnect:** telemetry batches carry **sequence numbers**;
  the controller **acks** each batch; the agent replays unacked batches after a
  **reconnect** (exponential backoff + jitter).
- **A disconnect never aborts a test:** the agent received a signed, time-bounded
  authorization at `assign_test` and continues *only that* test locally while
  buffering telemetry.
- **Time:** wall-clock (UTC ms) for cross-node correlation (requires chrony/NTP),
  plus a monotonic counter for drift-free durations; the agent's wall-clock offset
  to the controller is estimated at handshake.

## 5. Benchmark concept

- fio is driven via **generated jobfiles**, never hand-concatenated command
  lines. The UI exposes abstracted fields (workload, block size, queue depth,
  target IOPS/bandwidth, duration, …) that map to fio parameters.
- **Live metrics:** `fio --status-interval=1 --output-format=json` emits
  periodic *cumulative* JSON documents concatenated together; the agent splits
  them at JSON boundaries. Because those are running averages, per-interval
  curves (the ones that show "IOPS at minute 3") come from fio's `*_log` files
  with `log_avg_msec`. Final per-run precision uses `--output-format=json+`
  (full latency histograms).
- **Diagnostics sampler** (separate from fio, low overhead): `iostat`, `pidstat`,
  `vmstat`, `/proc` reads, `multipath -ll`, `smartctl`/`nvme`, and kernel-event
  following via `dmesg`/journal.
- **Overhead control:** configurable sample interval, local aggregation, batched
  sends, sampling off the I/O hot path.

## 6. Data model

SQLite tables (see `internal/store/schema.go`): `node`, `test_profile`,
`test_run`, `run_node`, `metric_sample`, `event`, `fault_action`.

Crucially, each run **freezes** its `resolved_config` and each participating
node's `system_snapshot` (kernel, distro, device/multipath info, …). *A result
without reproducible conditions is considered worthless*, so comparisons always
carry the exact conditions.

## 7. Fault-injection concept (Phase 4)

Every fault is a **typed, time-bounded, auto-reverting** action with logging.
Mechanisms (validated against current Linux tooling):

- **Network:** `tc netem` (latency/jitter/loss/bandwidth), `iptables` DROP with a
  unique marker scoped to a destination IP+port (targeted partition / path kill).
- **Storage:** device-mapper on a **dedicated test device** — `dm-flakey`
  (intermittent errors), `dm-delay` (latency), `dm-dust` (bad sectors) — and/or
  `scsi_debug` pseudo devices; multipath path failure by blocking one path while a
  redundant path remains.
- **Observation (not injection):** kernel reaction, I/O timeouts, D-state
  processes, failover time, recovery time via `dmesg`/journal, `/proc`, and
  `multipath -ll` diffs.

Recovery is judged by **measurable signals** (IOPS back to baseline, path
`active`, no new EIO), not by wall-clock guesses, and every fault is **verified**
as actually applied before being treated as injected. Faults are drawn as markers
over the performance charts so impact and recovery are visible.

### Who triggers faults

The storage admin will provoke failures manually on dedicated test servers; the
platform's first job is to **observe and correlate** those events. Software-side
fault injection is offered as an optional capability behind the broker and the
safety gates below, since the environment is test-only.

## 8. Security concept (the backbone)

- **No user input becomes a shell command.** Binaries are invoked with argument
  arrays; never `sh -c`.
- **Privilege-broker instead of broad sudo.** A small root service is the only
  path to privileged actions. It accepts **typed** operations (e.g.
  `{op:"netem_delay", iface:"eth1", ms:200, ttl:30}`), validates each field
  against an **allowlist** (ops, interfaces, devices/targets), and executes with
  argument arrays. This structurally eliminates command injection and keeps the
  large, network-facing agent out of root.
- **Resource allowlist per host** (admin-only): which block devices are testable,
  which interfaces may be used for `netem`, which iSCSI targets are "test".
- **System/root volume protection:** root/boot/swap devices and the default-route
  interface are hard-blocked — destructive tests against them are impossible, not
  merely greyed out.
- **Destructive tests off by default**, enabled per run with double confirmation
  and a server-side flag, naming the exact resource.
- **Time-bounded with a dead-man switch:** every network/storage change has a
  mandatory TTL and auto-reverts even if the controller or agent crashes; the
  management/SSH port is always whitelisted so an operator cannot be locked out.
- **Global stop/revert-all**, idempotent cleanup, process groups so no benchmark
  process is orphaned, and **one active test per node** (lock) to prevent double
  execution.
- **TLS + pre-shared token** (constant-time compare); UI behind login with Viewer
  vs. Operator roles (only Operator may authorize faults).
- **Full audit log** of every privileged action.

## 9. UI concept

Modern, calm, uncluttered (React/TS):

- **Dashboard:** three server tiles (online/offline, current attachment, live
  sparklines, running test, last results, error badges).
- **Test wizard:** guided flow (target → servers → workload → block size →
  IOPS/bandwidth → duration → optional faults → start), with faults in a clearly
  separated, collapsed "danger zone" requiring authorization; save as a profile.
- **Live view:** three servers side by side on a synchronized time axis with
  shared IOPS/latency charts and fault markers.
- **Result/report:** summary, frozen config + per-node system info, comparison
  table, time series, event log, assessment; CSV export always, PDF later.
- **Compare/history:** pick runs and compare IOPS/MB-s/avg/P95/P99/errors/recovery
  time/performance loss.

## 10. Risks & constraints

| Risk | Handling |
|---|---|
| Real "total storage loss" can genuinely hang a host (D-state, reboot) | Default to safely resettable `dm-*`/`scsi_debug`; real path kill only on allowlisted test targets, test servers only. |
| `iptables`/`tc` fault locks out management access | Mandatory SSH/controller-port whitelist; TTL auto-revert via watchdog. |
| Telemetry overhead skews the benchmark | Configurable interval, batching, low-priority sampler, off the I/O path. |
| Clock drift across hosts | chrony/NTP requirement + monotonic clock for durations + offset estimate. |
| `fio --status-interval` reports cumulative averages | Use `*_log` with `log_avg_msec` for true per-interval curves. |
| Kernel/distro capability differences | Agent capability probe at startup; UI hides unavailable features. |

## 11. Build & deployment reality

Runtime is Linux (SLES 15). The developer workstation here is Windows and is used
only to edit code — **nothing is installed or built on it**. Builds and tests run
on Linux (CI or a Linux build host), which is the authoritative signal that the
code compiles for the target. Deployment is `git pull` + build + `systemd` on the
servers.

## 12. Clarified decisions (from stakeholder Q&A)

- **Distro:** SLES 15. The agent still probes capabilities at startup.
- **Storage types:** several (block+multipath, NFS, SMB, local) — selectable.
- **Privilege model:** dedicated root privilege-broker with allowlist.
- **Fault scope:** admin provokes failures manually on dedicated test servers;
  software fault injection is a welcome optional addition behind safety gates.
- **Test targets:** dedicated test devices/mounts already exist; they must be
  explicitly allowlisted.
- **Auth:** pre-shared token + TLS.
- **Language:** Go.
