# Tutorial - Karoo

Este documento fornece um tutorial completo sobre como usar o Karoo.

## Instalação

### Usando Go

```bash
go install github.com/yourusername/karoo@latest
```

### Usando Docker

```bash
docker build -t karoo -f docker/Dockerfile .
docker run -p 8080:8080 karoo
```

## Configuração

Copie o arquivo de configuração de exemplo:

```bash
cp config/config.example.json config.json
```

Edite o arquivo `config.json` conforme necessário.

## Uso

### Iniciar o servidor

```bash
karoo serve
```

### Outros comandos

```bash
karoo --help
```

## Estrutura do Projeto

- `core/cmd/karoo/` - Ponto de entrada principal
- `core/internal/` - Código interno do aplicativo
- `core/pkg/` - Pacotes reutilizáveis
- `docker/` - Arquivos de configuração Docker
- `docs/` - Documentação
- `scripts/` - Scripts utilitários
