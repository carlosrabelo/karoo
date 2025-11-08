# Karoo Stratum Proxy

> Author: Carlos Rabelo - contato@carlosrabelo.com.br

Karoo started as a weekend experiment: a lightweight Stratum proxy so a rack of Nerdminers could share a single upstream connection. The idea quickly grew into a production-ready Stratum V1 front-end that keeps upstream pools happy while CPU, GPU, or embedded rigs hammer away behind it. What ships in this repository is exactly that proxy.

## Features

### Core Functionality
- **Stratum V1 Protocol Support** – full `mining.subscribe`, `mining.authorize`, and `mining.submit` handling with extranonce management.
- **Client & Upstream Management** – concurrent downstream clients with automatic upstream reconnects and exponential backoff.
- **Share Routing** – efficient share forwarding plus acceptance/rejection tracking.

### Advanced Controls
- **Variable Difficulty (VarDiff)** – dynamic, per-client adjustment with configurable target rates and min/max bounds.
- **Rate Limiting & Bans** – per-IP caps, connection-per-minute throttles, and automatic temporary bans.
- **Comprehensive Metrics** – HTTP `/status` and `/healthz` plus counters for shares, clients, and upstream health.
- **HTTP API** – light REST interface for health checks and runtime status.

### Runtime Comforts
- **Upstream on demand** – dials pools only when miners are online and backs off with jittered retries upon failures.
- **Protocol aware fan-out** – normalises request/response flows while preserving worker identity and minimizing extranonce collisions.
- **Share accounting & logs** – per-worker latency and cadence reports with aggregate summaries.
- **Flexible transport** – TCP today with a clear path toward TLS or alternative downstream protocols.

## Architecture

Karoo runs as an intermediary between miners and pools, exposing Stratum downstream while aggregating upstream connections and metrics.

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Miners    │────▶│    Karoo     │────▶│    Pool     │
│  (Clients)  │◀────│    Proxy     │◀────│ (Upstream)  │
└─────────────┘     └──────────────┘     └─────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │   HTTP API   │
                    │  (Metrics)   │
                    └──────────────┘
```

### Internal Packages
- `proxy` – connection lifecycle, share routing, and upstream orchestration.
- `routing` – message fan-out between miners and upstream.
- `nonce` – extranonce allocation and subscription tracking.
- `vardiff` – per-client difficulty controller.
- `ratelimit` – connection throttling and ban list enforcement.
- `connection` – buffered reader/writer helpers for Stratum frames.
- `metrics` – counters and gauges exposed over HTTP.
- `stratum` – request/response encoding helpers.

## Getting Started

### Prerequisites
- Go 1.25.4+
- Linux or macOS (Windows may work but is not part of CI)

### Quickstart
1. Clone this repository and copy a config: `cp config/config.example.json config.json`.
2. Build the proxy: `make build` (outputs `bin/karoo`).
3. Update `config.json` with your pool host (`upstream.host`), worker template (`user`), and optional VarDiff / rate-limit settings.
4. Start the proxy: `./bin/karoo -config ./config.json` (or `make run` which does the same after building).
5. Point your miners to `stratum+tcp://<proxy-host>:3333` (or whatever `proxy.listen` you configured) and use the worker names that Karoo rewrites upstream.
6. Hit `curl http://localhost:8080/status` and `curl http://localhost:8080/healthz` to confirm miners, shares, and upstream health.

### Make-based Workflow

```bash
make build        # compile to bin/karoo
make run          # build + run using ./config.json
make install      # install to ~/.local/bin or /usr/local/bin
```

### Install via Go

```bash
go install github.com/carlosrabelo/karoo/core/cmd/karoo@latest
```

The installed binary behaves exactly like the one produced by `make build`.

### Direct Go Build (core module)

```bash
cd core
go build -o karoo ./cmd/karoo
go test ./...
```

### Configuration File

The default configuration listens on `:3334` for Stratum clients, connects to the upstream defined in `config.json`, and exposes HTTP status on `:8080`. Copy `config/config.example.json` (or `core/config.example.json` if you are working inside the Go module) and adjust the fields below to suit your deployment.

```json
{
  "proxy": {
    "listen": "0.0.0.0:3333",
    "client_idle_ms": 300000,
    "max_clients": 1000,
    "read_buf": 4096,
    "write_buf": 4096
  },
  "upstream": {
    "host": "pool.example.com",
    "port": 3333,
    "user": "your_wallet_address.proxy",
    "pass": "x",
    "tls": false,
    "insecure_skip_verify": false,
    "backoff_min_ms": 1000,
    "backoff_max_ms": 60000
  },
  "http": {
    "listen": "0.0.0.0:8080",
    "pprof": false
  },
  "vardiff": {
    "enabled": true,
    "target_seconds": 15,
    "min_diff": 1000,
    "max_diff": 65536,
    "adjust_every_ms": 60000
  },
  "ratelimit": {
    "enabled": true,
    "max_connections_per_ip": 100,
    "max_connections_per_minute": 60,
    "ban_duration_seconds": 300,
    "cleanup_interval_seconds": 60
  },
  "compat": {
    "strict_broadcast": false
  }
}
```

Key fields:
- `proxy.listen` – downstream Stratum endpoint.
- `upstream.host/port/user/pass` – upstream pool credentials or worker template.
- `proxy.client_idle_ms` – disconnect idle miners after the configured period.
- `compat.strict_broadcast` – when `false`, forwards unknown `mining.*` methods unchanged.
- `vardiff.enabled` – enables the per-worker difficulty controller.
- `http.listen` – HTTP status listener (set empty string to disable).

### HTTP API
- `GET /healthz` – liveness probe that returns `ok` when the process is running.
- `GET /status` – JSON payload with upstream connection flags, extranonce info, VarDiff stats, rate-limit counters, and every connected client with accepted/rejected shares. Useful for dashboards and watchdogs.

### Connecting Miners
1. Configure your miners to use the Karoo host/port as their Stratum pool.
2. Set the worker name to anything meaningful (Karoo keeps the worker suffix and rewrites the upstream user).
3. Maintain the same password you configured under `upstream.pass` unless your pool requires per-worker passwords.
4. Watch the Karoo logs: every accepted or rejected share is accounted and rolled up in the periodic report.

### Deployment Shortcuts
- `make docker` builds the container image described in `deploy/docker`.
- `make systemd` installs the unit file from `deploy/systemd` (requires sudo).
- `deploy/k8s` contains namespaced manifests for Kubernetes clusters.

## Security

### Rate Limiting
- Guard against connection flooding with `max_connections_per_ip`.
- Keep reconnect storms in check via `max_connections_per_minute`.
- Temporary bans (`ban_duration_seconds`) discourage repeated abuse.

### Best Practices
1. Run behind a firewall and restrict downstream access to trusted networks.
2. Enable TLS when pools support it; otherwise keep proxy-to-pool traffic isolated.
3. Monitor `/status` regularly for rejection spikes and client churn.
4. Keep binaries updated to pick up bug fixes and security hardening.

## Troubleshooting

**Upstream Connection Fails** – ensure the pool host/port are reachable, firewall rules permit the egress port, and disable TLS if the upstream does not support it.

**Clients Can't Connect** – verify `proxy.listen` is exposed, confirm no other service is bound to the same port, and check perimeter firewalls.

**High Rejection Rate** – validate VarDiff parameters, confirm miners speak Stratum V1, and look for latency or packet loss between Karoo and the pool.

**Rate Limiting Too Aggressive** – raise `max_connections_per_ip/minute`, reduce ban duration, or disable the limiter for trusted networks.

## Development

### Running Tests

```bash
go test ./...
go test -race ./...
go test -cover ./...
```

### Code Structure

```
.
├── core/               # Go module (cmd/karoo + internal packages)
├── deploy/             # Docker, Kubernetes, and systemd assets
├── docs/               # Tutorials (EN/PT)
├── scripts/            # Helper scripts
├── config/             # Example configs copied into config.json
└── bin/                # Binaries produced by make build
```

## Contributing
1. Fork the repository.
2. Create a feature branch (`git checkout -b feature/amazing-feature`).
3. Write tests alongside code changes.
4. Run `go test ./...` and ensure `make build` still succeeds.
5. Commit with a descriptive message and open a pull request.

## Support
- GitHub Issues: https://github.com/carlosrabelo/karoo/issues
- Pull Requests: https://github.com/carlosrabelo/karoo/pulls

## Roadmap
- Expand the VarDiff loop into a moving-average controller with bucketed share statistics.
- Add downstream protocol adapters (e.g., WebSockets) and upstream failover lists.
- Ship structured metrics (Prometheus/OpenTelemetry) to complement the existing logs.

## Changelog

### v0.0.1 (Current)
- Initial release with Stratum V1 support.
- Variable difficulty controller.
- Rate limiting and HTTP metrics API.
- Comprehensive test coverage scaffold.

## License

Karoo is released under the GNU General Public License, version 2. See [LICENSE](LICENSE) for the full text.

## Donations

If Karoo is useful to you, consider supporting development:

- **BTC**: `bc1qw2raw7urfuu2032uyyx9k5pryan5gu6gmz6exm`
- **DOGE**: `DTAkhF6oHiK9HmcsSk3RPZp5XqR2bvCaHK`
- **ETH**: `0xdb4d2517C81bE4FE110E223376dD9B23ca3C762E`
- **LTC**: `LSQFLPM89gABNEGutwWMFA4ma24qDVwy8m`
- **TRX**: `TTznF3FeDCqLmL5gx8GingeahUyLsJJ68A`
