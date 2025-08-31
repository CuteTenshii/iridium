<div align="center">
  <h1>Iridium</h1>
  <p>A simple reverse proxy made in Go.</p>
</div>

## Features

- Configurable via YAML config files
- Supports HTTP
- Supports static responses, static files, and proxying to other servers
- Basic logging (to console and files)
- Uses raw TCP connections (no HTTP library)
- Lightweight and fast
- Has a built-in WAF (Web Application Firewall), configurable to block libraries (such as curl, wget, etc.), crawlers, specific IPs, and more.

## Installation

As of now, there are no pre-built binaries available. You need to build it from source.

### Docker

You can build and run Iridium using Docker with the following commands:

```bash
docker build -t iridium .
docker run -d -p 8080:8080 --name iridium_container iridium
```

## Configuration

On first run (without a config file), a default config file will be created in the `~/.iridium` directory (or `%APPDATA%\Iridium` on Windows). You can edit this file to customize the behavior of the reverse proxy. The configuration is done in YAML format.
