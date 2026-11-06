# storagebench

A small, high-quality platform for **storage benchmarking and resilience testing**
across three Linux servers. It measures how a storage attachment performs under
different I/O loads and how it behaves under fault conditions (network/storage
interruptions, path failover, recovery), correlating faults with performance on a
shared timeline.

> Status: **Phase 2 (MVP) — vertical slice.** Controller + agent, authenticated
> real-time WebSocket link, heartbeat/liveness, node registration with capability
> detection, host telemetry streaming, and SQLite persistence are in place. The
> fio benchmark engine, web UI, and fault injection land in the following phases
> (see [Roadmap](#roadmap)).

## Why this design

This project deliberately favors a **small, comprehensible stack** over a
microservice/Kubernetes setup. For three nodes that would be pure overhead.

- **Controller + Agent** (not a cluster). One coordinator with a web UI; a thin
  agent on every host. See [`docs/architecture.md`](docs/architecture.md) for the
  full Phase 1 concept, the rationale, and the alternatives that were considered.
- **The controller host is also a full test node.** It runs the *same* agent
  binary as a separate service and is driven over the same WebSocket interface as
  the remote agents — one code path for all three nodes, and the controller's own
  load never pollutes another node's telemetry.
- **Go**, producing dependency-free Linux binaries that deploy by copying a file
  and enabling a `systemd` unit.
- **SQLite** (WAL) for results and history: one file, no DB server, trivial
  backups.
- **fio** as the benchmark engine — no reinventing an I/O engine.

## Repository layout

```
cmd/
  controller/        # controller entrypoint (coordinator + web server)
  agent/             # agent entrypoint (benchmark + telemetry node)
internal/
  protocol/          # the WebSocket wire contract shared by both sides
  controller/        # hub (agent registry/liveness), HTTP/WS API, ingestion
  agent/             # connection lifecycle, reconnect, host-metric sampler
  store/             # SQLite persistence (schema, ingestion, queries)
  wsconn/            # WebSocket envelope read/write helper
  config/            # controller & agent config loading + validation
  sysinfo/           # host info + capability (tool) detection
  auth/              # pre-shared-token auth (constant-time compare)
  clock/, logx/, version/
configs/             # example configuration files
deploy/systemd/      # systemd units for controller and agent
docs/                # architecture / Phase 1 concept
.github/workflows/   # Linux CI (build + vet + test)
```

## Build

The platform runs on **Linux (SLES 15)**. Builds are produced on a Linux host or
in CI — **nothing is installed or built on a developer's Windows machine**; that
machine is used only to edit code. CI (`.github/workflows/ci.yml`) compiles and
tests on Linux and is the authoritative green/red signal.

On a Linux build host with Go 1.22+:

```bash
make tidy      # first time: resolve dependencies, write go.sum
make build     # builds bin/storagebench-controller and bin/storagebench-agent
make test      # run unit tests
make vet       # static checks
```

The agent builds fully static (`CGO_ENABLED=0`). The controller links the SQLite
driver and therefore builds with `CGO_ENABLED=1` (a C toolchain, e.g. `gcc`, must
be present on the build host).

## Deploy (overview)

1. Build on a Linux host (or download CI artifacts).
2. Copy `bin/storagebench-controller` to the controller host and
   `bin/storagebench-agent` to **all three** hosts (including the controller host).
3. Create configs from the examples in `configs/` and the environment files
   referenced by the `systemd` units in `deploy/systemd/`.
4. Put the shared token in the `systemd` environment files (`chmod 600`), **not**
   in the JSON configs.
5. Enable the services. Full step-by-step install commands are in the header
   comments of each unit file under `deploy/systemd/`.

### Time synchronization is required

Cross-node correlation relies on synchronized clocks. **Ensure `chrony`/NTP is
active on all three hosts.** The platform additionally uses a monotonic clock for
durations and estimates each agent's wall-clock offset for context.

## Security model (summary)

Security is the backbone of this project, because it drives privileged Linux
tools. Highlights (full detail in [`docs/architecture.md`](docs/architecture.md)):

- **No user input ever becomes a shell command.** Commands are executed with
  argument arrays, never `sh -c`.
- **The agent runs unprivileged.** Privileged actions (fault injection via
  `tc`/`iptables`, `dmsetup`, `multipath`, `iscsiadm`, raw-device fio) are
  delegated to a small, separate **privilege-broker** service with a strict
  allowlist (lands with the resilience phase).
- **Destructive tests are off by default** and require explicit, per-run
  authorization naming the exact resource.
- **TLS + pre-shared token** between controller and agents; constant-time token
  comparison.
- **Every privileged action is audit-logged.**

## Resilience behavior

A controller disconnect must never abort a running test. The agent:

- continues an **already-authorized** test locally (and only that test),
- keeps sampling and **buffers telemetry** with sequence numbers,
- **reconnects** with exponential backoff + jitter, and
- **replays** unacknowledged telemetry so no measurement data is lost.

## Roadmap

- **Phase 2 (current):** controller + agents, real-time link, registration,
  telemetry, persistence — plus the fio engine and first result views.
- **Phase 3:** full workload catalog, reusable test profiles, comparisons.
- **Phase 4:** controlled fault injection (network + storage) with recovery
  measurement and event correlation, behind the privilege-broker and safety gates.
- **Phase 5:** test history, result comparison, CSV/PDF export, optimization.

## License

TBD.
