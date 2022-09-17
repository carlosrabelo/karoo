# Karoo Stratum Proxy

> Autor: Carlos Rabelo - contato@carlosrabelo.com.br

Karoo começou como um experimento de fim de semana: um proxy Stratum leve para permitir que um rack de Nerdminers compartilhasse uma única conexão upstream. Hoje ele evoluiu para um front-end Stratum V1 pronto para produção, mantendo os pools satisfeitos enquanto rigs CPU, GPU ou embarcadas martelam shares por trás. Este repositório contém exatamente esse proxy.

## Destaques

- Protocolo Stratum V1 completo: subscribe, authorize, submit e gestão de extranonce
- Múltiplos clientes downstream com reconexão automática ao pool e backoff exponencial
- Roteamento eficiente de shares com contadores de aceitação e rejeição por worker
- Failover automático para pools de backup em caso de falha de conexão upstream
- Suporte TLS para conexões upstream com pools e downstream com mineradores
- Recarga de configuração sem derrubar conexões ativas de mineradores
- VarDiff por cliente com metas configuráveis e limites mínimo/máximo de dificuldade
- Rate limiting por IP, throttling de conexões e banimentos temporários automáticos
- Endpoints HTTP: Prometheus `/metrics`, saúde `/healthz` e status `/status`
- Suporte a proxy SOCKS5 para conexões upstream

## Visão Geral

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
- `proxy` – ciclo de vida das conexões, roteamento de shares e orquestração do upstream
- `routing` – fan-out de mensagens entre mineradores e pool
- `nonce` – alocação de extranonce e controle de inscrições
- `vardiff` – controlador de dificuldade por cliente
- `ratelimit` – limites e banimentos por IP
- `connection` – utilidades de leitura/escrita para frames Stratum
- `proxysocks` – suporte a proxy SOCKS5 para conexões upstream
- `metrics` – contadores e gauges expostos via HTTP
- `stratum` – helpers de codificação de requisições e respostas

## Pré-requisitos

- Go 1.18+
- Linux ou macOS (Windows pode funcionar, mas não faz parte do CI)

## Instalação

### Compilar a partir do código

```bash
git clone https://github.com/carlosrabelo/karoo.git
cd karoo
make build
```

Instale em `~/.local/bin` (padrão) ou em `/usr/local/bin` (sudo apenas na cópia):

```bash
make install
make install-system
make uninstall    # remove dos dois locais comuns
```

### Via Go Install

```bash
go install github.com/carlosrabelo/karoo/karoo/cmd/karoo@latest
```

## Início Rápido

1. Clone o repositório e copie a configuração: `cp config.example.json config.json`
2. Compile o proxy: `make build` (gera `bin/karoo`)
3. Ajuste `config.json` com o host do pool (`upstream.host`), o modelo de usuário (`user`) e eventuais parâmetros de VarDiff e rate limiting
4. Inicie o proxy: `./bin/karoo -config ./config.json` (ou `make run`, que compila e roda usando esse arquivo)
5. Aponte seus mineradores para `stratum+tcp://<host-do-proxy>:3333` (ou a porta definida em `proxy.listen`) e use os nomes de worker que o Karoo reescreverá para o upstream
6. Consulte `curl http://localhost:8080/status` e `curl http://localhost:8080/healthz` para validar clientes, shares e saúde do upstream

## Uso

### API HTTP
- `GET /healthz` – verificação simples que responde `ok` enquanto o processo estiver vivo
- `GET /status` – payload JSON com flags do upstream, dados de extranonce, estatísticas de VarDiff e rate limiting, além dos clientes conectados com shares aceitas/rejeitadas

### Conectando Mineradores
1. Configure seus dispositivos para usar o host/porta do Karoo como pool Stratum
2. Escolha nomes de worker significativos; o Karoo preserva o sufixo do worker e reescreve apenas o usuário base configurado para o pool
3. Use a mesma senha definida em `upstream.pass`, a menos que o pool exija algo diferente por worker
4. Acompanhe os logs do Karoo: cada share aceita ou rejeitada entra no relatório periódico

### Opções de Deploy
- `make docker` gera a imagem usando os artefatos em `deploy/docker`
- `make systemd` instala a unit de `deploy/systemd` (requer sudo)
- O diretório `deploy/k8s` contém manifestos namespaced para Kubernetes

## Configuração

Copie o template e aponte o Karoo para ele:

```bash
cp config.example.json config.json
./bin/karoo -config ./config.json
```

Referência completa dos campos, proxy SOCKS5 e notas de segurança: [docs/CONFIGURATION-PT.md](docs/CONFIGURATION-PT.md). English: [docs/CONFIGURATION.md](docs/CONFIGURATION.md).

## Segurança

### Rate Limiting
- Use `max_connections_per_ip` para conter floods de conexão
- Limite reconexões com `max_connections_per_minute`
- `ban_duration_seconds` desestimula abusos repetidos

### Boas Práticas
1. Restrinja o acesso downstream via firewall ou redes confiáveis
2. Habilite TLS no upstream quando disponível e mantenha o tráfego isolado
3. Monitore `/status` para detectar picos de rejeição ou churn de clientes
4. Atualize os binários para receber correções e hardenings

## Estrutura do Projeto

```
karoo/cmd/karoo/   # Ponto de entrada Go
karoo/internal/    # Pacotes internos (proxy, routing, stratum, …)
bin/               # Binários compilados (ignorados pelo git)
.make/             # Scripts de build e instalação
deploy/            # Artefatos para Docker, Kubernetes e systemd
config.example.json # Configuração de exemplo (copiar para config.json)
docs/              # Tutoriais e guia de configuração
```

## Desenvolvimento

```bash
make build           # Compila o binário para bin/karoo
make test            # Executa todos os testes
make quality         # Formata, vet e lint
make install         # Instala o binário em ~/.local/bin
make install-system  # Instala o binário em /usr/local/bin
make uninstall       # Remove dos dois locais comuns
```

## Solução de Problemas

**Falha ao conectar no upstream** – confira host/porta, regras de firewall e desative TLS se o pool não suportar.

**Clientes não conectam** – valide `proxy.listen`, garanta que nenhuma outra aplicação usa a porta e revise firewalls de borda.

**Muitas shares rejeitadas** – ajuste parâmetros do VarDiff, confirme que os mineradores falam Stratum V1 e investigue latência entre Karoo e o pool.

**Rate limiting agressivo** – aumente `max_connections_per_ip/minute`, reduza o tempo de ban ou desative o limite em redes confiáveis.

## Contribuição

1. Faça um fork do repositório
2. Crie uma branch (`git checkout -b feature/minha-feature`)
3. Escreva testes junto com as mudanças
4. Rode `make test` e assegure que `make build` continua funcionando
5. Abra um Pull Request com uma mensagem de commit descritiva

## Suporte

- Issues no GitHub: https://github.com/carlosrabelo/karoo/issues
- Pull Requests: https://github.com/carlosrabelo/karoo/pulls

## Roteiro

- Evoluir o VarDiff para um controlador de média móvel com estatísticas em buckets
- Adicionar adaptadores downstream (ex.: WebSockets)

## Changelog

### v0.0.1 (Atual)
- Release inicial com suporte ao Stratum V1
- Controlador de dificuldade VarDiff
- Rate limiting e API HTTP de métricas
- Base de testes para ampliar cobertura

## Licença

Karoo é distribuído sob GNU General Public License v2. Veja [LICENSE](LICENSE) para o texto completo.

## Doações

Se Karoo for útil, considere apoiar o desenvolvimento:

- **BTC**: `bc1qw2raw7urfuu2032uyyx9k5pryan5gu6gmz6exm`
- **DOGE**: `DTAkhF6oHiK9HmcsSk3RPZp5XqR2bvCaHK`
- **ETH**: `0xdb4d2517C81bE4FE110E223376dD9B23ca3C762E`
- **LTC**: `LSQFLPM89gABNEGutwWMFA4ma24qDVwy8m`
- **TRX**: `TTznF3FeDCqLmL5gx8GingeahUyLsJJ68A`
