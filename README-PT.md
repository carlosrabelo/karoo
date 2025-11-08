# Karoo Stratum Proxy

> Autor: Carlos Rabelo - contato@carlosrabelo.com.br

Karoo começou como um experimento de fim de semana: um proxy Stratum leve para permitir que um rack de Nerdminers compartilhasse uma única conexão upstream. Hoje ele evoluiu para um front-end Stratum V1 pronto para produção, mantendo os pools satisfeitos enquanto rigs CPU, GPU ou embarcadas martelam shares por trás. Este repositório contém exatamente esse proxy.

## Funcionalidades

### Funcionalidades Principais
- **Suporte ao Stratum V1** – tratamento completo de `mining.subscribe`, `mining.authorize` e `mining.submit`, incluindo gestão de extranonce.
- **Gestão de Clientes e Upstream** – múltiplos clientes downstream com reconexão automática ao pool e backoff exponencial.
- **Roteamento de Shares** – encaminhamento eficiente com contadores de aceitação/rejeição.

### Controles Avançados
- **VarDiff** – ajuste dinâmico por cliente com metas configuráveis e limites mínimo/máximo.
- **Rate Limiting e Banimento** – limites por IP, conexões por minuto e banimentos temporários automáticos.
- **Métricas Completas** – HTTP `/status` e `/healthz` com estatísticas de shares, clientes e upstream.
- **API HTTP** – endpoints REST leves para saúde e status em tempo real.

### Confortos Operacionais
- **Upstream sob demanda** – disca o pool apenas quando há mineradores e aplica backoff com jitter nos erros.
- **Fan-out ciente do protocolo** – normaliza fluxos Stratum preservando IDs e evitando colisões de extranonce.
- **Contabilização e logs** – relatórios por worker (latência e cadência) e sumários agregados.
- **Transporte flexível** – TCP hoje, com encanamento pronto para TLS ou protocolos adicionais.

## Arquitetura

Karoo atua como intermediário entre mineradores e pools, expondo Stratum downstream e agregando conexões upstream e métricas.

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│ Mineradores │────▶│    Karoo     │────▶│    Pool     │
│  (Clientes) │◀────│    Proxy     │◀────│ (Upstream)  │
└─────────────┘     └──────────────┘     └─────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │   HTTP API   │
                    │  (Métricas)  │
                    └──────────────┘
```

### Pacotes Internos
- `proxy` – ciclo de vida das conexões, roteamento de shares e orquestração do upstream.
- `routing` – fan-out de mensagens entre mineradores e pool.
- `nonce` – alocação de extranonce e controle de inscrições.
- `vardiff` – controlador de dificuldade por cliente.
- `ratelimit` – limites e banimentos por IP.
- `connection` – utilidades de leitura/escrita para frames Stratum.
- `metrics` – contadores e gauges expostos via HTTP.
- `stratum` – helpers de codificação de requisições e respostas.

## Primeiros Passos

### Pré-requisitos
- Go 1.21+
- Linux ou macOS (Windows pode funcionar, mas não faz parte do CI)

### Guia Rápido
1. Clone o repositório e copie a configuração: `cp config/config.example.json config.json`.
2. Compile o proxy: `make build` (gera `bin/karoo`).
3. Ajuste `config.json` com o host do pool (`upstream.host`), o modelo de usuário (`user`) e eventuais parâmetros de VarDiff e rate limiting.
4. Inicie o proxy: `./bin/karoo -config ./config.json` (ou `make run`, que compila e roda usando esse arquivo).
5. Aponte seus mineradores para `stratum+tcp://<host-do-proxy>:3333` (ou a porta definida em `proxy.listen`) e use os nomes de worker que o Karoo reescreverá para o upstream.
6. Consulte `curl http://localhost:8080/status` e `curl http://localhost:8080/healthz` para validar clientes, shares e saúde do upstream.

### Fluxo via Make

```bash
make build        # compila para bin/karoo
make run          # compila + executa usando ./config.json
make install      # instala em ~/.local/bin ou /usr/local/bin
```

### Instalação com Go

```bash
go install github.com/carlosrabelo/karoo/core/cmd/karoo@latest
```

O binário instalado é idêntico ao produzido pelo `make build`.

### Build direto no módulo core

```bash
cd core
go build -o karoo ./cmd/karoo
go test ./...
```

### Arquivo de configuração

A configuração padrão escuta em `:3334`, conecta ao pool definido em `config.json` e expõe HTTP em `:8080`. Copie `config/config.example.json` (ou `core/config.example.json` ao trabalhar dentro do módulo Go) e ajuste conforme necessário:

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

Campos em destaque:
- `proxy.listen` – endpoint Stratum exposto aos mineradores.
- `upstream.host/port/user/pass` – credenciais ou template de worker no pool.
- `proxy.client_idle_ms` – desconexão automática após o tempo configurado.
- `compat.strict_broadcast` – quando `false`, repassa métodos `mining.*` desconhecidos.
- `vardiff.enabled` – ativa o controlador de dificuldade por worker.
- `http.listen` – porta usada pelos endpoints HTTP (deixe vazio para desabilitar).

### API HTTP
- `GET /healthz` – verificação simples que responde `ok` enquanto o processo estiver vivo.
- `GET /status` – payload JSON com flags do upstream, dados de extranonce, estatísticas de VarDiff e rate limiting, além dos clientes conectados com shares aceitas/rejeitadas. Ideal para dashboards ou watchdogs.

### Conectando Mineradores
1. Configure seus dispositivos para usar o host/porta do Karoo como pool Stratum.
2. Escolha nomes de worker significativos; o Karoo preserva o sufixo do worker e reescreve apenas o usuário base configurado para o pool.
3. Use a mesma senha definida em `upstream.pass`, a menos que o pool exija algo diferente por worker.
4. Acompanhe os logs do Karoo: cada share aceita ou rejeitada entra no relatório periódico.

### Opções de Deploy
- `make docker` gera a imagem usando os artefatos em `deploy/docker`.
- `make systemd` instala a unit de `deploy/systemd` (requer sudo).
- O diretório `deploy/k8s` contém manifestos namespaced para Kubernetes.

## Segurança

### Rate Limiting
- Use `max_connections_per_ip` para conter floods de conexão.
- Limite reconexões com `max_connections_per_minute`.
- `ban_duration_seconds` desestimula abusos repetidos.

### Boas Práticas
1. Restrinja o acesso downstream via firewall ou redes confiáveis.
2. Habilite TLS no upstream quando disponível e mantenha o tráfego isolado.
3. Monitore `/status` para detectar picos de rejeição ou churn de clientes.
4. Atualize os binários para receber correções e hardenings.

## Solução de Problemas

**Falha ao conectar no upstream** – confira host/porta, regras de firewall e desative TLS se o pool não suportar.

**Clientes não conectam** – valide `proxy.listen`, garanta que nenhuma outra aplicação usa a porta e revise firewalls de borda.

**Muitas shares rejeitadas** – ajuste parâmetros do VarDiff, confirme que os mineradores falam Stratum V1 e investigue latência entre Karoo e o pool.

**Rate limiting agressivo** – aumente `max_connections_per_ip/minute`, reduza o tempo de ban ou desative o limite em redes confiáveis.

## Desenvolvimento

### Execução de testes

```bash
go test ./...
go test -race ./...
go test -cover ./...
```

### Estrutura do código

```
.
├── core/               # Módulo Go (cmd/karoo + pacotes internos)
├── deploy/             # Artefatos para Docker, Kubernetes e systemd
├── docs/               # Tutoriais (EN/PT)
├── scripts/            # Scripts auxiliares
├── config/             # Exemplos copiados para config.json
└── bin/                # Binários gerados pelo make build
```

## Contribuição
1. Faça um fork do repositório.
2. Crie uma branch (`git checkout -b feature/minha-feature`).
3. Escreva testes junto com as mudanças.
4. Rode `go test ./...` e assegure que `make build` continua funcionando.
5. Abra um Pull Request com uma mensagem de commit descritiva.

## Suporte
- Issues no GitHub: https://github.com/carlosrabelo/karoo/issues
- Pull Requests: https://github.com/carlosrabelo/karoo/pulls

## Roteiro
- Evoluir o VarDiff para um controlador de média móvel com estatísticas em buckets.
- Adicionar adaptadores downstream (ex.: WebSockets) e failover para upstream.
- Expor métricas estruturadas (Prometheus/OpenTelemetry) além dos logs.

## Changelog

### v0.0.1 (Atual)
- Release inicial com suporte ao Stratum V1.
- Controlador de dificuldade VarDiff.
- Rate limiting e API HTTP de métricas.
- Base de testes para ampliar cobertura.

## Licença

Karoo é distribuído sob GNU General Public License v2. Veja [LICENSE](LICENSE) para o texto completo.

## Doações

Se Karoo for útil, considere apoiar o desenvolvimento:

- **BTC**: `bc1qw2raw7urfuu2032uyyx9k5pryan5gu6gmz6exm`
- **DOGE**: `DTAkhF6oHiK9HmcsSk3RPZp5XqR2bvCaHK`
- **ETH**: `0xdb4d2517C81bE4FE110E223376dD9B23ca3C762E`
- **LTC**: `LSQFLPM89gABNEGutwWMFA4ma24qDVwy8m`
- **TRX**: `TTznF3FeDCqLmL5gx8GingeahUyLsJJ68A`
