*I still didn't find a name for this project*

A simple reverse proxy made in Go.

## Features

- Configurable via YAML config files
- Supports HTTP
- Supports static responses, static files, and proxying to other servers
- Basic logging (to console and files)
- Uses raw TCP connections (no HTTP library)
- Lightweight and fast

## Installation

As of now, there are no pre-built binaries available. You need to build it from source.

## Configuration

On first run (without a config file), a default config file will be created in the `~/.reverseproxy` directory (or `%APPDATA%\reverseproxy` on Windows). You can edit this file to customize the behavior of the reverse proxy. The configuration is done in YAML format.
