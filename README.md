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
- Edge caching support for static files: caches files in memory for faster delivery

## Installation

As of now, there are no pre-built binaries available. You need to build it from source.

### Docker

You can build and run Iridium using Docker with the following commands:

```bash
docker build -t iridium .
docker run -d -p 8080:8080 --name iridium -v ./iridium-data:/root/.iridium iridium
```

### macOS

You can install Iridium on macOS using Homebrew:

```bash
brew install IridiumProxy/iridium/iridium
```

## Configuration

On first run (without a config file), default config files will be created in the `~/.iridium` directory (or `%APPDATA%\Iridium` on Windows). You can edit those files to customize the behavior of Iridium. The configuration is done in YAML format, which is easy to read and write.

By default, the config file is located at `~/.iridium/config.yaml` (or `%APPDATA%\Iridium\config.yaml` on Windows).

## Edge Caching

Iridium supports edge caching for static files. You can enable edge caching in the configuration file by setting the `edge_cache.enabled` option to `true`. This option can be applied globally or per host.

Because edge caching stores files in memory, make sure your server has enough RAM to handle the cached files.

> [!TIP] 
> When Edge Caching is enabled, Iridium will automatically add a `X-Cache: HIT` or a `X-Cache: MISS` header to the response, indicating whether the response was served from the cache or not.

### Caching Time

If not specified, the caching time will be taken from the `Cache-Control` header of the response. If the header is not present, a default caching time of 1 hour (3600 seconds) will be used.

This can be configured in the `edge_cache.default_cache_time` option in the configuration file (in seconds).