# Tutorial - Karoo

This document provides a complete tutorial on how to use Karoo.

## Installation

### Using Go

```bash
go install github.com/yourusername/karoo@latest
```

### Using Docker

```bash
docker build -t karoo -f docker/Dockerfile .
docker run -p 8080:8080 karoo
```

## Configuration

Copy the example configuration file:

```bash
cp config/config.example.json config.json
```

Edit the `config.json` file as needed.

## Usage

### Start the server

```bash
karoo serve
```

### Other commands

```bash
karoo --help
```

## Project Structure

- `core/cmd/karoo/` - Main entry point
- `core/internal/` - Internal application code
- `core/pkg/` - Reusable packages
- `docker/` - Docker configuration files
- `docs/` - Documentation
- `scripts/` - Utility scripts
