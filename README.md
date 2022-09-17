# Karoo Stratum Proxy

> Author: Carlos Rabelo - contato@carlosrabelo.com.br

Karoo started as a weekend experiment: a lightweight Stratum proxy so a rack of Nerdminers could share a single upstream connection. The idea quickly grew into a production-ready Stratum V1 front-end that keeps upstream pools happy while CPU, GPU, or embedded rigs hammer away behind it. What ships in this repository is exactly that proxy.

## Highlights

- Full Stratum V1 protocol: subscribe, authorize, submit, and extranonce management
- Concurrent downstream clients with automatic upstream reconnects and exponential backoff
- Efficient share routing with per-worker acceptance and rejection tracking
- Automatic failover to backup pools on upstream connection failure
- TLS support for both upstream pool and downstream miner connections
- Hot configuration reload without dropping active miner connections
- Per-client variable difficulty (VarDiff) with configurable target rates and bounds
- Per-IP rate limiting, connection throttling, and automatic temporary bans
- Prometheus `/metrics`, health `/healthz`, and runtime `/status` HTTP endpoints
- SOCKS5 proxy support for upstream connections

## Overview

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
- `proxy` – connection lifecycle, share routing, and upstream orchestration
- `routing` – message fan-out between miners and upstream
- `nonce` – extranonce allocation and subscription tracking
- `vardiff` – per-client difficulty controller
- `ratelimit` – connection throttling and ban list enforcement
- `connection` – buffered reader/writer helpers for Stratum frames
- `proxysocks` – SOCKS5 proxy support for upstream connections
- `metrics` – counters and gauges exposed over HTTP
- `stratum` – request/response encoding helpers

## Prerequisites

- Go 1.18+
- Linux or macOS (Windows may work but is not part of CI)

## Installation

### Build from Source

```bash
git clone https://github.com/carlosrabelo/karoo.git
cd karoo
make build
```

Install to `~/.local/bin` (default), or system-wide to `/usr/local/bin` (sudo only for the copy):

```bash
make install
make install-system
make uninstall    # removes from both common locations
```

### Using Go Install

```bash
go install github.com/carlosrabelo/karoo/karoo/cmd/karoo@latest
```

## Quick Start

1. Clone this repository and copy a config: `cp config.example.json config.json`
2. Build the proxy: `make build` (outputs `bin/karoo`)
3. Update `config.json` with your pool host (`upstream.host`), worker template (`user`), and optional VarDiff / rate-limit settings
4. Start the proxy: `./bin/karoo -config ./config.json` (or `make run` which does the same after building)
5. Point your miners to `stratum+tcp://<proxy-host>:3333` (or whatever `proxy.listen` you configured) and use the worker names that Karoo rewrites upstream
6. Hit `curl http://localhost:8080/status` and `curl http://localhost:8080/healthz` to confirm miners, shares, and upstream health

## Usage

### HTTP API
- `GET /healthz` – liveness probe that returns `ok` when the process is running
- `GET /status` – JSON payload with upstream connection flags, extranonce info, VarDiff stats, rate-limit counters, and every connected client with accepted/rejected shares

### Connecting Miners
1. Configure your miners to use the Karoo host/port as their Stratum pool
2. Set the worker name to anything meaningful (Karoo keeps the worker suffix and rewrites the upstream user)
3. Maintain the same password you configured under `upstream.pass` unless your pool requires per-worker passwords
4. Watch the Karoo logs: every accepted or rejected share is accounted and rolled up in the periodic report

### Deployment Shortcuts
- `make docker` builds the container image described in `deploy/docker`
- `make systemd` installs the unit file from `deploy/systemd` (requires sudo)
- `deploy/k8s` contains namespaced manifests for Kubernetes clusters

## Configuration

Copy the template and point Karoo at it:

```bash
cp config.example.json config.json
./bin/karoo -config ./config.json
```

Full field reference, SOCKS5 upstream proxy, and security notes: [docs/CONFIGURATION.md](docs/CONFIGURATION.md). Portuguese: [docs/CONFIGURATION-PT.md](docs/CONFIGURATION-PT.md).

## Security

### Rate Limiting
- Guard against connection flooding with `max_connections_per_ip`
- Keep reconnect storms in check via `max_connections_per_minute`
- Temporary bans (`ban_duration_seconds`) discourage repeated abuse

### Best Practices
1. Run behind a firewall and restrict downstream access to trusted networks
2. Enable TLS when pools support it; otherwise keep proxy-to-pool traffic isolated
3. Monitor `/status` regularly for rejection spikes and client churn
4. Keep binaries updated to pick up bug fixes and security hardening

## Project Layout

```
karoo/cmd/karoo/   # Go entry point
karoo/internal/    # Internal packages (proxy, routing, stratum, …)
bin/               # Compiled binaries (git-ignored)
.make/             # Build and install scripts
deploy/            # Docker, Kubernetes, and systemd assets
config.example.json # Example configuration (copy to config.json)
docs/              # Tutorials and configuration guide
```

## Development

```bash
make build           # Compile binary to bin/karoo
make test            # Run all tests
make quality         # Format, vet, and lint
make install         # Install binary to ~/.local/bin
make install-system  # Install binary to /usr/local/bin
make uninstall       # Remove from both common locations
```

## Troubleshooting

**Upstream Connection Fails** – ensure the pool host/port are reachable, firewall rules permit the egress port, and disable TLS if the upstream does not support it.

**Clients Can't Connect** – verify `proxy.listen` is exposed, confirm no other service is bound to the same port, and check perimeter firewalls.

**High Rejection Rate** – validate VarDiff parameters, confirm miners speak Stratum V1, and look for latency or packet loss between Karoo and the pool.

**Rate Limiting Too Aggressive** – raise `max_connections_per_ip/minute`, reduce ban duration, or disable the limiter for trusted networks.

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Write tests alongside code changes
4. Run `make test` and ensure `make build` still succeeds
5. Commit with a descriptive message and open a pull request

## Support

- GitHub Issues: https://github.com/carlosrabelo/karoo/issues
- Pull Requests: https://github.com/carlosrabelo/karoo/pulls

## Roadmap

- Expand the VarDiff loop into a moving-average controller with bucketed share statistics
- Add downstream protocol adapters (e.g., WebSockets)

## Changelog

### v0.0.1 (Current)
- Initial release with Stratum V1 support
- Variable difficulty controller
- Rate limiting and HTTP metrics API
- Comprehensive test coverage scaffold

## License

Karoo is released under the GNU General Public License, version 2. See [LICENSE](LICENSE) for the full text.

## Donations

If Karoo is useful to you, consider supporting development:

- **BTC**: `bc1qw2raw7urfuu2032uyyx9k5pryan5gu6gmz6exm`
- **DOGE**: `DTAkhF6oHiK9HmcsSk3RPZp5XqR2bvCaHK`
- **ETH**: `0xdb4d2517C81bE4FE110E223376dD9B23ca3C762E`
- **LTC**: `LSQFLPM89gABNEGutwWMFA4ma24qDVwy8m`
- **TRX**: `TTznF3FeDCqLmL5gx8GingeahUyLsJJ68A`
