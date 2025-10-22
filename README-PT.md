# Karoo Stratum Proxy

> Autor: Carlos Rabelo - contato@carlosrabelo.com.br

Karoo começou como um experimento de fim de semana: um proxy Stratum leve para permitir que um rack de Nerdminers compartilhasse uma única conexão upstream. A ideia rapidamente cresceu para um front-end Stratum multi-protocolo que mantém os pools satisfeitos enquanto rigs CPU, GPU ou embarcadas martelam trabalho por trás.

## Funcionalidades

- **Upstream sob demanda** – disca o pool configurado apenas quando existem mineradores conectados e aplica backoff com jitter nos erros.
- **Fan-out ciente do protocolo** – normaliza os fluxos `mining.subscribe`, `mining.authorize` e `mining.submit`, preservando IDs e minimizando colisões de extranonce.
- **Contabilização de shares** – logs detalhados por worker (latência, espaçamento entre shares aceitas) mais relatórios periódicos e endpoints HTTP `/status` / `/healthz`.
- **VarDiff básico** – ajustes opcionais de dificuldade por worker com espaço para evoluir para um controlador mais elaborado.
- **Transporte flexível** – suporte atual a TCP, com encanamento pronto para TLS ou protocolos adicionais.

## Primeiros Passos

```bash
make build                 # compila para bin/karoo (CGO desabilitado)
cp config/config.example.json config.json
make run                   # executa usando ./config.json
```

A configuração padrão escuta em `:3334` para clientes Stratum e conecta ao pool upstream definido em `config.json`. Os endpoints HTTP de status ficam expostos em `:8080`.

### Destaques da Configuração

- `proxy.listen` – endereço downstream onde os mineradores se conectam.
- `upstream.{host,port,user,pass}` – credenciais do pool Stratum.
- `proxy.client_idle_ms` – desconecta mineradores após o tempo configurado.
- `compat.strict_broadcast` – quando `false`, encaminha mensagens `mining.*` desconhecidas.
- `vardiff.enabled` – ativa ajustes simples de dificuldade por worker.

Para referência completa, consulte `config/config.example.json`.

### Estrutura do Projeto

- `core/cmd/karoo/` – ponto de entrada principal e flags da CLI.
- `core/internal/` – espaço reservado para pacotes compartilhados enquanto o proxy amadurece.
- `deploy/Makefile` – alvos de Docker, Kubernetes e systemd.
- `deploy/docker/` – assets de build multi-stage da imagem.
- `deploy/systemd/` – unidades para `make systemd`.
- `deploy/k8s/` – manifests Kubernetes (`configmap.yaml`, `deployment.yaml`, `service.yaml`, `kustomization.yaml`).
- `bin/` – artefatos produzidos por `make build` (fora do Git por padrão).

## Fluxo de Desenvolvimento

- Compile com `make build` (gera `bin/karoo`).
- Execute verificações com `go test ./...` (assim que testes forem adicionados).
- Formate o código via `gofmt` (o Makefile já lida com isso antes de buildar).
- Publica binários criando uma tag como `v1.0.0` ou acionando o workflow GitHub Actions manualmente.

### Docker

Para gerar uma imagem:

```bash
docker build -f deploy/docker/Dockerfile -t karoo:latest .
```

A imagem inclui `config/config.example.json` como `/app/config.json`. Monte uma configuração própria em runtime se precisar de parâmetros diferentes:

```bash
docker run --rm -p 3334:3334 -p 8080:8080 \
  -v $PWD/config.json:/app/config.json:ro \
  karoo:latest
```

Os manifests para orquestração vivem em `deploy/k8s/`. Aplique com `kubectl apply -k deploy/k8s/` ou `make -C deploy k8s-apply` após ajustar imagem e dados sensíveis.

## Roteiro

- Evoluir o loop VarDiff para um controlador baseado em média móvel com estatísticas por buckets.
- Adicionar adaptadores de protocolo downstream (ex.: WebSockets) e listas de failover upstream.
- Expor métricas estruturadas (Prometheus/OpenTelemetry) complementando os logs atuais.

## Licença

Karoo é distribuído sob GNU GPL v2. Veja [LICENSE](LICENSE) para o texto completo.

## Doações

Se Karoo for útil, considere apoiar o desenvolvimento:

- **BTC**: `bc1qw2raw7urfuu2032uyyx9k5pryan5gu6gmz6exm`
- **DOGE**: `DTAkhF6oHiK9HmcsSk3RPZp5XqR2bvCaHK`
- **ETH**: `0xdb4d2517C81bE4FE110E223376dD9B23ca3C762E`
- **LTC**: `LSQFLPM89gABNEGutwWMFA4ma24qDVwy8m`
- **TRX**: `TTznF3FeDCqLmL5gx8GingeahUyLsJJ68A`
