# Configuração

O Karoo lê um arquivo JSON na inicialização (`-config`, padrão `config.json`). Comece pelo template e ajuste o que precisar:

```bash
cp config.example.json config.json
```

Recarregue um processo em execução sem derrubar mineradores enviando `SIGHUP` (ou reinicie após editar).

## Template

Veja [`config.example.json`](../config.example.json) para o arquivo inicial completo. Após copiar, os padrões são:

| Área | Padrão | Função |
|------|--------|--------|
| Stratum downstream | `:3333` | Mineradores conectam aqui |
| Pool upstream | `pool.example.org:3333` | Pool primário |
| Status HTTP | `:8080` | `/healthz`, `/status`, `/metrics` |
| VarDiff | ativo | Controle de dificuldade por cliente |
| Rate limit | ativo | Throttling de conexões por IP |

## Seções

### `proxy` — listener downstream

| Campo | Descrição |
|-------|-----------|
| `listen` | Endereço de bind para conexões Stratum dos mineradores |
| `client_idle_ms` | Desconecta mineradores ociosos após esse tempo (ms) |
| `max_clients` | Limite máximo de clientes downstream simultâneos |
| `read_buf` / `write_buf` | Tamanho dos buffers de socket em bytes |
| `tls.enabled` | Habilita TLS nas conexões dos mineradores |
| `tls.cert_file` / `tls.key_file` | Caminhos do certificado e chave PEM quando TLS está ativo |

### `upstream` — pool primário

| Campo | Descrição |
|-------|-----------|
| `host` / `port` | Hostname e porta Stratum do pool |
| `user` / `pass` | Credenciais upstream ou template de worker (o Karoo reescreve o sufixo do worker) |
| `tls` | Usa TLS até o pool |
| `insecure_skip_verify` | Ignora verificação do certificado TLS (apenas dev/teste) |
| `backoff_min_ms` / `backoff_max_ms` | Janela de backoff ao reconectar após falha |

### `backups` — pools de failover

Array de objetos no mesmo formato de `upstream`. O Karoo faz failover quando a conexão primária não consegue ser estabelecida ou mantida.

### `http` — API de status

| Campo | Descrição |
|-------|-----------|
| `listen` | Endereço HTTP; use `""` para desabilitar |
| `pprof` | Expõe endpoints pprof do Go quando `true` |

Endpoints com HTTP ativo:

- `GET /healthz` — liveness (`ok` quando o processo está no ar)
- `GET /status` — JSON com flags upstream, VarDiff, rate-limit e shares por cliente
- `GET /metrics` — métricas Prometheus

### `vardiff` — dificuldade variável

| Campo | Descrição |
|-------|-----------|
| `enabled` | Liga ou desliga o controlador de dificuldade por worker |
| `target_seconds` | Segundos alvo entre shares por cliente |
| `min_diff` / `max_diff` | Limites de dificuldade |
| `adjust_every_ms` | Intervalo de recálculo da dificuldade |

### `ratelimit` — controle de abuso de conexões

| Campo | Descrição |
|-------|-----------|
| `enabled` | Ativa throttling e banimentos temporários por IP |
| `max_connections_per_ip` | Conexões simultâneas permitidas por IP |
| `max_connections_per_minute` | Novas conexões por IP por minuto |
| `ban_duration_seconds` | Duração do ban temporário após estourar limites |
| `cleanup_interval_seconds` | Intervalo de limpeza de bans/estado expirados |

### `compat` — peculiaridades de pools

| Campo | Descrição |
|-------|-----------|
| `strict_broadcast` | Quando `false`, métodos `mining.*` desconhecidos são reencaminhados sem alteração |
| `local_authorize` | Quando `true`, responde `mining.authorize` localmente e nunca encaminha senhas de mineradores ao pool (a auth do pool usa `upstream.user` / `upstream.pass`) |

Recomendado: manter `local_authorize` ativo para as senhas dos mineradores ficarem só no proxy.

## SOCKS5 para upstream

Roteie conexões com o pool por um proxy SOCKS5 (VPN, Tor ou similar). Só o tráfego upstream é proxied; conexões de mineradores não são.

Adicione `socks_proxy` em `upstream` (e opcionalmente em cada backup):

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

| Campo | Descrição |
|-------|-----------|
| `enabled` | Roteia o TCP upstream pelo proxy |
| `type` | Deve ser `"socks5"` (SOCKS4 é rejeitado) |
| `host` / `port` | Endereço do proxy |
| `username` / `password` | Auth SOCKS5 opcional (deixe vazio se não usar) |

TLS até o pool continua funcionando: o SOCKS5 abre o TCP e o Karoo faz o handshake TLS.

## Exemplo mínimo

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

## Reload a quente

Envie `SIGHUP` a um processo em execução (ou reinicie) após editar `config.json`. O reload aplica:

- Host/user/pass/TLS/SOCKS do upstream (força reconnect)
- Lista de backups no próximo ciclo de failover
- Flags de VarDiff, rate-limit e `compat`

Endereço de listen / bind HTTP ainda exigem reinício do processo.

## Notas de segurança

- Restrinja `proxy.listen` a redes confiáveis ou coloque um firewall na frente
- Prefira TLS até o pool quando o upstream suportar
- Ative `compat.local_authorize` para não enviar senhas de mineradores ao upstream
- Monitore `/status` para picos de rejeição e churn de clientes
- Ajuste ou desative o rate limiting em LANs totalmente confiáveis se estiver agressivo demais
