# Configuration

Karoo reads a JSON file at startup (`-config`, default `config.json`). Start from the template and edit the fields you need:

```bash
cp config.example.json config.json
```

Reload a running process without dropping miners by sending `SIGHUP` (or restart after edits).

## Template

See [`config.example.json`](../config.example.json) for a complete starter file. Defaults after copy:

| Area | Default | Purpose |
|------|---------|---------|
| Downstream Stratum | `:3333` | Miners connect here |
| Upstream pool | `pool.example.org:3333` | Primary pool |
| HTTP status | `:8080` | `/healthz`, `/status`, `/metrics` |
| VarDiff | enabled | Per-client difficulty control |
| Rate limit | enabled | Per-IP connection throttling |

## Sections

### `proxy` — downstream listener

| Field | Description |
|-------|-------------|
| `listen` | Bind address for miner Stratum connections |
| `client_idle_ms` | Disconnect idle miners after this many milliseconds |
| `max_clients` | Hard cap on concurrent downstream clients |
| `read_buf` / `write_buf` | Socket buffer sizes in bytes |
| `tls.enabled` | Enable TLS for miner connections |
| `tls.cert_file` / `tls.key_file` | PEM certificate and key paths when TLS is on |

### `upstream` — primary pool

| Field | Description |
|-------|-------------|
| `host` / `port` | Pool hostname and Stratum port |
| `user` / `pass` | Upstream credentials or worker template (Karoo rewrites the worker suffix) |
| `tls` | Use TLS toward the pool |
| `insecure_skip_verify` | Skip TLS certificate verification (dev/testing only) |
| `backoff_min_ms` / `backoff_max_ms` | Reconnect backoff window after upstream failure |

### `backups` — failover pools

Array of upstream objects with the same shape as `upstream`. Karoo fails over when the primary connection cannot be established or kept alive.

### `http` — status API

| Field | Description |
|-------|-------------|
| `listen` | HTTP bind address; set to `""` to disable |
| `pprof` | Expose Go pprof endpoints when `true` |

Endpoints when HTTP is enabled:

- `GET /healthz` — liveness (`ok` when the process is up)
- `GET /status` — JSON with upstream flags, VarDiff, rate-limit, and per-client share stats
- `GET /metrics` — Prometheus metrics

### `vardiff` — variable difficulty

| Field | Description |
|-------|-------------|
| `enabled` | Turn the per-worker difficulty controller on or off |
| `target_seconds` | Target seconds between shares per client |
| `min_diff` / `max_diff` | Difficulty bounds |
| `adjust_every_ms` | How often difficulty is recalculated |

### `ratelimit` — connection abuse controls

| Field | Description |
|-------|-------------|
| `enabled` | Enable per-IP throttling and temporary bans |
| `max_connections_per_ip` | Concurrent connections allowed from one IP |
| `max_connections_per_minute` | New connections allowed per IP per minute |
| `ban_duration_seconds` | Temporary ban length after limits are exceeded |
| `cleanup_interval_seconds` | How often expired ban/state entries are purged |

### `compat` — pool quirks

| Field | Description |
|-------|-------------|
| `strict_broadcast` | When `false`, unknown `mining.*` methods are forwarded unchanged |
| `local_authorize` | When `true`, answer `mining.authorize` locally and never forward miner passwords upstream (pool auth uses `upstream.user` / `upstream.pass`) |

Recommended: keep `local_authorize` enabled so miner passwords stay on the proxy.

## SOCKS5 for upstream

Route pool connections through a SOCKS5 proxy (VPN, Tor, or similar). Only upstream traffic is proxied; miner connections are not.

Add `socks_proxy` under `upstream` (and optionally under each backup):

```json
{
  "upstream": {
    "host": "pool.example.org",
    "port": 3333,
    "user": "wallet.worker",
    "pass": "x",
    "socks_proxy": {
      "enabled": true,
      "type": "socks5",
      "host": "127.0.0.1",
      "port": 1080,
      "username": "",
      "password": ""
    }
  }
}
```

| Field | Description |
|-------|-------------|
| `enabled` | Route upstream TCP through the proxy |
| `type` | Must be `"socks5"` (SOCKS4 is rejected) |
| `host` / `port` | Proxy address |
| `username` / `password` | Optional SOCKS5 auth (leave empty if unused) |

TLS to the pool still works: SOCKS5 opens the TCP path, then Karoo performs the TLS handshake.

## Minimal example

```json
{
  "proxy": {
    "listen": ":3333"
  },
  "upstream": {
    "host": "pool.example.org",
    "port": 3333,
    "user": "wallet.worker",
    "pass": "x"
  },
  "http": {
    "listen": ":8080"
  }
}
```

## Hot reload

Send `SIGHUP` to a running process (or restart) after editing `config.json`. Reload applies:

- Upstream host/user/pass/TLS/SOCKS (forces reconnect)
- Backup pool list used on the next failover cycle
- VarDiff, rate-limit, and `compat` flags

Listen address / HTTP bind still require a process restart.

## Security notes

- Restrict `proxy.listen` to trusted networks or put a firewall in front
- Prefer TLS to the pool when the upstream supports it
- Enable `compat.local_authorize` so miner passwords are not sent upstream
- Watch `/status` for rejection spikes and client churn
- Tune or disable rate limiting on fully trusted LANs if it is too aggressive
