# FlareTunnel Manager

> A secure orchestration layer for creating, removing, and serving Cloudflare Workers through [FlareTunnel](https://github.com/johndoe237/FlareTunnel).

[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Containerized](https://img.shields.io/badge/runtime-Docker-2496ED?logo=docker&logoColor=white)](https://www.docker.com/)
[![License](https://img.shields.io/badge/license-review%20upstream%20terms-informational)](https://github.com/johndoe237/FlareTunnel)

FlareTunnel Manager is a small Go service that coordinates Cloudflare account state with the FlareTunnel command-line application. It deliberately keeps Cloudflare orchestration, validation, temporary credential handling, and deployment concerns separate from the Worker and tunnel implementation maintained by FlareTunnel.

The project is distributed as **one reproducible container image**. The same image can be built once and deployed to a local Docker host, a VPS, or any container-compatible PaaS.

## Highlights

- **Bounded deletion:** automated cleanup always uses `cleanup --count N --yes` and never invokes the legacy unbounded cleanup operation.
- **Real-state reconciliation:** only Workers whose names begin with `flaretunnel-` are counted.
- **Safe retries:** create and delete operations re-check Cloudflare state before each attempt, retry at most three times per account, and continue processing subsequent accounts after a failure.
- **Strict input validation:** active-mode JSON must be a non-empty array with required fields and non-negative worker targets.
- **Multi-account tunnel bootstrap:** `use` writes one temporary account configuration, runs one `list` operation, removes temporary credentials, and then replaces the manager process with the tunnel process.
- **Embedded blacklists:** the three upstream blacklist files are packaged into the runtime image and selected by level, not by an arbitrary user-supplied path.
- **Secret hygiene:** credentials are written with restrictive permissions and are redacted from operational errors and logs.

## Mandatory proxy authentication

`AUTH_PROXY` is a manager-only JSON secret required in `use` mode:

```bash
AUTH_PROXY='{"username":"user1","password":"pass1"}'
```

The manager parses this object in memory, encodes `username:password` as Base64,
and passes only the resulting `AUTH_PROXY_BASIC` value to the FlareTunnel child
process. For example, `user1:pass1` becomes `dXNlcjE6cGFzczE=`. The JSON,
username, password, and encoded value are never written to runtime files or
operational logs.

FlareTunnel requires `AUTH_PROXY_BASIC` at startup and checks
`Proxy-Authorization: Basic <AUTH_PROXY_BASIC>` before processing every
supported method, including `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`,
`OPTIONS`, and `CONNECT`. Missing or incorrect credentials receive `407 Proxy
Authentication Required` and `Proxy-Authenticate: Basic`.

## Upstream version

The image builds the public FlareTunnel fork at the exact commit below:

```text
b308a52ce3eb775c13744b23f2024bead88a4c99
```

The Docker build checks the resolved Git commit before compiling the complete upstream package. The manager does not reimplement FlareTunnel functionality.

## Operating modes

| Mode | Active variable | Behavior of `target_workers` |
|---|---|---|
| `create` | `CF_CREATE_ACCOUNTS` | Desired final number of `flaretunnel-*` Workers. |
| `delete` | `CF_DELETE_ACCOUNTS` | Number of `flaretunnel-*` Workers to remove. It is not a desired final count. |
| `use` | `CF_USE_ACCOUNTS` | No target is required. Accounts are used to build the tunnel endpoint set. |

Accounts are processed sequentially. In `create`, the manager calculates `missing = target_workers - existing`. In `delete`, it calculates the remaining requested deletion count and caps each cleanup request by the number of Workers that actually exist.

A bounded delete request is equivalent to:

```text
cleanup --account <ACCOUNT> --count <N> --yes
```

If the requested deletion count is zero, or if Cloudflare reports no matching Workers, no cleanup command is executed.

## Configuration

| Variable | Required | Default | Description |
|---|---:|---:|---|
| `MODE` | Yes | — | `create`, `delete`, or `use`. |
| `AUTH_PROXY` | In `use` | — | Strict JSON object with non-empty `username` and `password`. |
| `PORT` | No | `8080` | Port passed to the tunnel process. |
| `FLARETUNNEL_MODE` | No | `random` | `random` or `round-robin`. Unknown values fall back to `random`. |
| `FLARETUNNEL_BLACKLIST` | No | `minimal` | `minimal`, `full`, or `aggressive`. Unknown values fall back to `minimal`. |
| `CF_CREATE_ACCOUNTS` | In `create` | — | JSON array of accounts to reconcile. |
| `CF_DELETE_ACCOUNTS` | In `delete` | — | JSON array of accounts to clean up. |
| `CF_USE_ACCOUNTS` | In `use` | — | JSON array of accounts used by the tunnel. |
| `CF_API_BASE_URL` | No | Cloudflare API | Optional API endpoint override for controlled testing. |

The active account variable must contain a non-empty JSON array. Each account requires `name`, `api_token`, and `account_id`. `target_workers` is required in `create` and `delete`, must be an integer greater than or equal to zero, and is ignored in `use`.

Example account object:

```json
[
  {
    "name": "main",
    "api_token": "INJECTED_SECRET",
    "account_id": "CLOUDFLARE_ACCOUNT_ID",
    "target_workers": 20
  }
]
```

Do not place real credentials in Git, Dockerfiles, image layers, documentation, tests, or shell history. Use `.env.example` as a template and inject real values through a protected local file or the secret manager of your platform. `AUTH_PROXY` is not required in `create` or `delete` mode.

## Embedded blacklist files

The runtime image contains these upstream files:

| Level | File in the container |
|---|---|
| `minimal` | `/opt/flaretunnel/blacklist-minimal.txt` |
| `full` | `/opt/flaretunnel/blacklist.txt` |
| `aggressive` | `/opt/flaretunnel/blacklist-aggressive.txt` |

The manager intentionally does not support `FLARETUNNEL_BLACKLIST_DIR`. Users can select a level, but cannot provide an arbitrary blacklist path.

## Quick start with Docker

```bash
cp .env.example .env
# Edit .env locally. Never commit it.
docker build -t flaretunnel-manager:latest .
docker run --rm --env-file .env -p 8080:8080 flaretunnel-manager:latest
```

The image entrypoint is `/usr/local/bin/flaretunnel-manager`. In `use` mode, the manager eventually executes FlareTunnel so that the tunnel becomes the container's main process. The container is therefore long-running in `use` mode and one-shot in normal `create` and `delete` runs.

## VPS deployment

Install Docker or an OCI-compatible runtime on the VPS. Pull a previously built image or build this repository on the VPS. Store the environment file outside the repository with restrictive permissions, or use the VPS secret manager:

```bash
chmod 600 /secure/path/flaretunnel-manager.env
docker pull IMAGE_REFERENCE

docker run -d \
  --name flaretunnel-manager \
  --restart unless-stopped \
  --env-file /secure/path/flaretunnel-manager.env \
  -p 8080:8080 \
  IMAGE_REFERENCE
```

The VPS does not need Go installed. It runs the container image directly.

## PaaS deployment

Select this Docker image or its Dockerfile in the PaaS deployment configuration. Define `AUTH_PROXY`, the active account JSON, and API tokens in the platform secret manager. Configure the image entrypoint as the main process, expose `PORT` when required by the platform, and use a TCP or HTTP health check appropriate for `use` mode. Do not assume that a PaaS filesystem is persistent; the manager treats credentials as temporary and keeps the runtime endpoint file only for the lifetime of the tunnel container.

## Security model

The manager validates the complete active JSON before processing any account. It does not partially process an invalid list. Per-account configuration files are created with mode `0600` and are removed after the account operation. In `use`, the combined credential file is removed before the tunnel process starts. `AUTH_PROXY_BASIC` is passed only through the FlareTunnel process environment and is never placed in `flaretunnel.json` or another runtime file. Error output redacts sensitive environment values and structured credential fields.

Cloudflare API tokens should be scoped to the minimum permissions required by the selected operation. Rotate tokens through the Cloudflare dashboard or your secret manager rather than editing source files.

## Project layout

```text
.
├── cmd/manager/              # Process entrypoint and mode selection
├── internal/business/        # Create, delete, and use orchestration
├── internal/cloudflare/      # Cloudflare Worker discovery and counting
├── internal/config/          # Environment configuration and defaults
├── internal/flaretunnel/     # FlareTunnel commands and file contracts
├── internal/logging/         # Secret-aware operational logging
├── internal/runtime/         # Temporary directory lifecycle
├── internal/validation/      # Strict account JSON validation
├── Dockerfile                # Reproducible multi-stage image build
└── .env.example              # Non-secret configuration template
```

## Development and verification

The following commands validate the complete manager package:

```bash
gofmt -w .
go test ./...
go vet ./...
go build ./cmd/manager
```

To validate the container build on a machine with Docker:

```bash
docker build -t flaretunnel-manager:test .
```

## License and upstream notices

Review the upstream FlareTunnel license and terms before redistribution. This repository is an orchestration layer and does not replace the legal or operational requirements of Cloudflare or the upstream project.

## References

[1]: https://github.com/johndoe237/FlareTunnel "FlareTunnel upstream fork"
[2]: https://github.com/johndoe237/FlareTunnel/commit/b308a52ce3eb775c13744b23f2024bead88a4c99 "Pinned FlareTunnel commit"
[3]: https://docs.docker.com/ "Docker documentation"
