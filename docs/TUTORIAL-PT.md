# Tutorial - Karoo (PT)

Passo a passo completo para compilar, configurar e operar o proxy Stratum V1 do Karoo.

## 1. Requisitos
- Go 1.25.4+
- Git
- Acesso a um pool Stratum V1 (URL + template de worker)
- Shell Linux ou macOS (Windows via WSL funciona)

## 2. Clonar e Compilar

```bash
git clone https://github.com/carlosrabelo/karoo.git
cd karoo
make build            # gera ./bin/karoo
```

Se preferir instalar direto com Go:

```bash
go install github.com/carlosrabelo/karoo/core/cmd/karoo@latest
```

## 3. Preparar a Configuração

```bash
cp config/config.example.json config.json
```

Edite `config.json` e ajuste:
- `proxy.listen`: host/porta exposta aos mineradores (padrão `0.0.0.0:3333`).
- `upstream.host` / `upstream.port`: endpoint do pool (ex.: `pool.example.com:3333`).
- `upstream.user`: carteira ou conta + sufixo opcional de worker (`carteira.worker`).
- `upstream.pass`: senha esperada pelo pool (normalmente `x`).
- Opcional: habilite `vardiff`, configure `ratelimit` e escolha a porta HTTP em `http.listen` (padrão `0.0.0.0:8080`).

Mantenha o arquivo próximo ao binário ou passe outro caminho usando `-config`.

## 4. Executar o Proxy

```bash
./bin/karoo -config ./config.json
# ou
make run                       # compila (se preciso) e executa com ./config.json
```

O Karoo imediatamente:
1. Escuta mineradores em `proxy.listen`.
2. Abre o upstream sob demanda quando o primeiro cliente aparece.
3. Expõe `/healthz` e `/status` no HTTP configurado.

Finalize com `Ctrl+C` para um desligamento limpo.

## 5. Apontar os Mineradores
1. Troque a URL de pool para `stratum+tcp://<host-do-karoo>:<porta-proxy.listen>`.
2. Use nomes de worker que façam sentido para você; o Karoo preserva esse sufixo e apenas reescreve o usuário base definido em `upstream.user`.
3. Utilize a mesma senha definida em `upstream.pass`, salvo se o pool exigir diferente por worker.

Os logs exibem shares aceitas/rejeitadas, além do relatório periódico com taxas e acurácia.

## 6. Monitorar Métricas

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/status | jq
```

`/status` retorna dados de extranonce, contadores de shares, estatísticas de VarDiff, rate limiting e todos os clientes conectados com seus números de aceites/rejeições. Integre em dashboards ou alertas.

## 7. Opções de Deploy (Opcional)

### Docker / docker-compose
```bash
cd deploy/docker
docker compose up --build
```
Monte seu `config.json` ou inclua-o na imagem (veja `deploy/docker/Dockerfile`).

### Serviço systemd
```bash
sudo cp deploy/systemd/karoo.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now karoo
```
Edite a unit para apontar `ExecStart` ao caminho correto do binário/config.

### Kubernetes
```bash
kubectl apply -f deploy/k8s/
```
Atualize o ConfigMap e os manifestos de Service conforme necessário.

## 8. Dicas de Troubleshooting
- Conexão upstream instável: confira `upstream.host`, regras de firewall e TLS.
- Shares rejeitadas: valide se os mineradores falam Stratum V1 e ajuste `compat.strict_broadcast`.
- IPs banidos: aumente `max_connections_per_ip` ou desabilite `ratelimit.enabled` em redes confiáveis.

Para detalhes adicionais, consulte o `README-PT.md`.
