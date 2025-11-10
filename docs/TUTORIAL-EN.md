# Karoo Tutorial (EN)

End-to-end walkthrough to compile, configure, and operate the Karoo Stratum V1 proxy.

## 1. Requirements
- Go 1.25+ (tested with 1.25.4)
- Git
- Access to a Stratum V1 pool (URL + worker template)
- Linux or macOS shell (Windows works via WSL)

## 2. Clone and Build

```bash
git clone https://github.com/carlosrabelo/karoo.git
cd karoo
make build            # produces ./bin/karoo from core module
```

Alternatively, install straight from Go:

```bash
go install github.com/carlosrabelo/karoo/core/cmd/karoo@latest
```

The project uses a hierarchical Makefile structure. All build operations are forwarded to the `core/` module automatically.

## 3. Prepare the Configuration

```bash
cp config/config.example.json config.json
```

Edit `config.json` and set at least:
- `proxy.listen`: host/port Karoo exposes to your miners (default `:3334` in example).
- `upstream.host` / `upstream.port`: pool endpoint (e.g., `pool.example.org:3333`).
- `upstream.user`: wallet or account plus optional worker suffix (`wallet.worker`).
- `upstream.pass`: password expected by the pool (`x` for most BTC pools).
- Optional: enable `vardiff`, configure `http.listen` for metrics (default `:8080`), and adjust `compat.strict_broadcast` for pool quirks.

Keep the file alongside the binary or point Karoo to a different path with `-config`.

## 4. Run the Proxy

```bash
./bin/karoo -config ./config.json
# or
make run                       # builds (if needed) and runs with ./config.json
```

The binary is built in the `core/` module and placed in `./bin/karoo`. The `make run` command handles the build automatically and forwards execution to the core module.

Karoo immediately:
1. Listens for miners on `proxy.listen`.
2. Establishes an upstream connection on demand when the first miner appears.
3. Exposes HTTP `/healthz` and `/status` on the configured port.

Stop the proxy with `Ctrl+C` – it performs a graceful shutdown.

## 5. Point Your Miners
1. Change the pool URL on each miner to `stratum+tcp://<karoo-host>:<proxy.listen-port>`.
2. Use worker names you want to see at the upstream pool. Karoo prepends the upstream user automatically (`upstream.user.worker`).
3. Keep the same password set in `upstream.pass` unless your pool enforces per-worker credentials.

Each accepted or rejected share is logged and counted inside Karoo; use the periodic log report to watch rates.

## 6. Observe Metrics

```bash
curl http://localhost:8080/healthz    # returns "ok" if the process is alive
curl http://localhost:8080/status | jq
```

`/status` returns extranonce data, share counters, VarDiff statistics, rate-limit state, and every connected client with accepted/rejected shares. Feed it to dashboards or alerts as needed.

## 7. Optional Deployment Targets

### Docker / docker-compose
```bash
make docker                    # builds via deploy module
# or manually:
cd deploy/docker
docker compose up --build
```
Mount your `config.json` or bake it into the image (see `deploy/docker/Dockerfile`).

### Systemd Service
```bash
make systemd                   # installs via deploy module
# or manually:
sudo cp deploy/systemd/karoo.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now karoo
```
Edit the unit to point `ExecStart` at the correct binary/config paths.

### Kubernetes
```bash
kubectl apply -f deploy/k8s/
```
Patch the ConfigMap and Service manifests with your own `config.json` and exposure rules.

All deployment commands are orchestrated through the hierarchical Makefile structure.

## 8. Troubleshooting
- Upstream connection flaps: verify `upstream.host` is reachable and your firewall allows the outbound port.
- Miners rejected: ensure they use Stratum V1 and that `compat.strict_broadcast` fits your pool quirks.
- Build issues: ensure Go 1.25+ is installed and run `make mod-tidy` to clean dependencies.
- Connection refused: check that `proxy.listen` port is available and not blocked by firewall.

Refer back to `README.md` for deeper explanations of VarDiff, rate limiting, and architecture. Run `make help` to see all available commands.
