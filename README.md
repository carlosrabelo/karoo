# Karoo Stratum Proxy

> Author: Carlos Rabelo - contato@carlosrabelo.com.br

Karoo started as a weekend experiment: a lightweight Stratum proxy so a rack of Nerdminers could share a single upstream connection. The idea quickly grew into a general-purpose, multi-protocol Stratum front-end that keeps upstream pools happy while CPU, GPU, or embedded rigs hammer away behind it.

## Features

- **Upstream on demand** – automatically dials the configured pool only when miners are connected and backs off with jittered retries on failures.
- **Protocol aware fan-out** – normalises `mining.subscribe`, `mining.authorize`, and `mining.submit` flows while preserving client IDs and minimizing extranonce collisions.
- **Share accounting & reporting** – detailed logs per worker (latency, spacing between accepted shares) plus periodic aggregate reports and HTTP `/status` / `/healthz` endpoints.
- **VarDiff scaffold** – optional, per-worker difficulty adjustments with room to grow into a full moving-average controller.
- **Flexible transport** – TCP today, with plumbing designed to extend to TLS or additional wire protocols.

## Getting Started

```bash
make build        # compile to build/karoo
make run          # run with ./config.json
```

The default configuration listens on `:3334` for Stratum clients and connects to the upstream pool defined in `config.json`. HTTP status endpoints are exposed at `:8080` by default.

### Configuration Highlights

- `proxy.listen` – downstream address the miners connect to.
- `upstream.{host,port,user,pass}` – upstream Stratum pool credentials.
- `proxy.client_idle_ms` – disconnect idle miners after the specified timeout.
- `compat.strict_broadcast` – when `false`, forwards unfamiliar `mining.*` messages unchanged.
- `vardiff.enabled` – toggle simple per-worker VarDiff adjustments.

See the commented sample at the bottom of `main.go` for a full reference.

## Development Workflow

- Build with `make build` (outputs `build/karoo`).
- Run unit-style checks by executing `go test ./...` (once tests are added).
- Format code with `gofmt` (the Makefile target already does this before building).
- Publish binaries by pushing a tag like `v1.0.0` or running the GitHub Actions workflow manually.

## Roadmap

- Expand the VarDiff loop into a moving average controller with bucketed share statistics.
- Add downstream protocol adapters (e.g., WebSockets) and upstream failover lists.
- Ship structured metrics (Prometheus/OpenTelemetry) to complement the existing logs.

## License

Karoo is released under the GNU General Public License, version 2. See [LICENSE](LICENSE) for the full text.

## Donations

If Karoo is useful to you, consider supporting development:

- **BTC**: `bc1qw2raw7urfuu2032uyyx9k5pryan5gu6gmz6exm`
- **DOGE**: `DTAkhF6oHiK9HmcsSk3RPZp5XqR2bvCaHK`
- **ETH**: `0xdb4d2517C81bE4FE110E223376dD9B23ca3C762E`
- **LTC**: `LSQFLPM89gABNEGutwWMFA4ma24qDVwy8m`
- **TRX**: `TTznF3FeDCqLmL5gx8GingeahUyLsJJ68A`
