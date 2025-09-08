<div align="center">
  <h1>Iridium</h1>
  <p>A simple reverse proxy made in Go.</p>

  <a href="https://iridiumproxy.github.io">
    <img alt="Read the Docs" src="https://img.shields.io/badge/read-the%20docs-blue">
  </a>
</div>

## Features

- Configurable via YAML config files
- Supports HTTP
- Supports static responses, static files, and proxying to other servers
- Basic logging (to console and files)
- Uses raw TCP connections (no HTTP library)
- Lightweight and fast
- Has a built-in WAF (Web Application Firewall), configurable to block libraries (such as curl, wget, etc.), crawlers, specific IPs, and more.
  Even supports serving a captcha page for blocked requests.
- Edge caching support for static files: caches files in memory for faster delivery
- Compression support (gzip, deflate, zstd)
- Rate limiting support: limits the number of requests per IP per second

## Installation

As of now, there are no pre-built binaries available. You need to build it from source.

### Docker

You can build and run Iridium using Docker with the following commands:

```bash
docker run -d \
  -p 8080:8080 \
  -v ./iridium-data:/root/.iridium \
  --name iridium \
  ghcr.io/iridiumproxy/iridium:latest
```

### Docker Compose

You can also use Docker Compose to set up Iridium. Create a `docker-compose.yml` file with the following content:

```yaml
services:
  iridium:
    image: ghcr.io/iridiumproxy/iridium:latest
    container_name: iridium
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./iridium-data:/root/.iridium
```

### macOS

You can install Iridium on macOS using Homebrew:

```bash
brew install IridiumProxy/iridium/iridium
```

## Configuration

On first run (without a config file), default config files will be created in the `~/.iridium` directory (or `%APPDATA%\Iridium` on Windows). You can edit those files to customize the behavior of Iridium. The configuration is done in YAML format, which is easy to read and write.

By default, the config file is located at `~/.iridium/config.yaml` (or `%APPDATA%\Iridium\config.yaml` on Windows).
