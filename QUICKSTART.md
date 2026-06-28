# Quickstart

## Run

```bash
go run .
```

Admin:

```text
http://127.0.0.1:18788/admin
```

Health check:

```text
http://127.0.0.1:18788/healthz
```

Default admin password:

```text
Fyb2530+
```

## NewAPI

Create an OpenAI-compatible upstream/channel:

```text
Base URL: http://127.0.0.1:18788/v1
Model: video-parse
Key: Wrapper Secret from /admin
```

Users still call NewAPI with their NewAPI keys. This wrapper should normally only be reachable by NewAPI.

## Concurrency Defaults

```text
workers: 128
queueSize: 10000
timeout: 45000ms
retryTimes: 1
```

Worker count and queue size changes require service restart.

## Docker

```bash
docker compose up -d --build
```
