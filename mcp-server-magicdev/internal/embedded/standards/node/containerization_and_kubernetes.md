---
title: "Node.js Docker, Kubernetes & Containerization Standards"
tags: ["node", "docker", "kubernetes", "containers"]
description: "Prescribes Dockerfile multi-stage builds, Alpine/Slim base image selection, graceful shutdown signal handling, Kubernetes health probes, and structured JSON logging for containerized Node.js services."
---

# Docker, Kubernetes & Containerization Standards

## 1. Dockerfile Best Practices
### Multi-Stage Builds
- **Always use multi-stage builds.** Use a `builder` stage with the full Node.js image to install all dependencies (including devDependencies) and compile TypeScript. Copy only production artifacts to a lean final stage.
- **Use Alpine or Slim base images**: Prefer `node:22-alpine` or `node:22-slim` for the runtime stage to minimize image size and attack surface.

### Dependency Installation
- **Use `npm ci --frozen-lockfile --omit=dev`** in the production stage for deterministic, reproducible installs from `package-lock.json`. Never use `npm install` in production builds.
- **Leverage Docker layer caching**: Copy `package.json` and `package-lock.json` first, run `npm ci`, then copy source code. This ensures the dependency layer is cached unless the lockfile changes.

### Security
- **Never run as root.** Create a dedicated non-root user and group in the Dockerfile and switch to it using `USER` before the `CMD` instruction.
- **Use `.dockerignore`**: Aggressively exclude `.git`, `node_modules` (local), `test/`, `.env`, `*.log`, and IDE configuration files from the build context.
- **Do not embed secrets in images.** Inject credentials via environment variables or secret management services (Kubernetes Secrets, HashiCorp Vault) at runtime.

### Example
```dockerfile
# Stage 1: Build
FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Stage 2: Production Runtime
FROM node:22-alpine AS runner
WORKDIR /app
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
COPY --from=builder --chown=appuser:appgroup /app/package*.json ./
RUN npm ci --omit=dev --frozen-lockfile
COPY --from=builder --chown=appuser:appgroup /app/dist ./dist
USER appuser
ENV NODE_ENV=production
EXPOSE 3000
CMD ["node", "dist/index.js"]
```

## 2. Runtime Configuration
- **Set `NODE_ENV=production`**: This is critical. Frameworks like Express enable performance optimizations, disable verbose stack traces, and reduce memory usage in production mode.
- **Listen on `process.env.PORT`**: Never hardcode the application port. Allow the orchestrator or environment to configure it dynamically.
- **Use `DOTNET_SYSTEM_GLOBALIZATION_INVARIANT`**: Not applicable to Node; instead, configure `TZ` environment variable explicitly if timezone-aware behavior is needed.

## 3. Graceful Shutdown
- Listen for `SIGTERM` and `SIGINT` signals explicitly.
- On signal receipt:
  1. Stop accepting new connections.
  2. Finish processing in-flight requests (with a configurable timeout).
  3. Close database connections, Redis clients, and message queue consumers.
  4. Flush pending logs and telemetry.
  5. Exit with code `0`.
- Use a shutdown timeout (e.g., 30 seconds) to prevent indefinite hangs. If the timeout expires, force exit.

## 4. Kubernetes Best Practices
### Health Probes
- **Liveness Probe** (`/health/live`): Checks if the Node.js process is running and responsive. Must NOT depend on external services (databases, caches). A simple HTTP 200 response is sufficient.
- **Readiness Probe** (`/health/ready`): Checks if the application is ready to serve traffic. This SHOULD verify database connectivity, cache availability, and other critical dependencies.
- **Startup Probe**: Use for applications with slow initialization (e.g., large schema migrations, warming caches) to prevent premature liveness kills.

### Resource Management
- Always define `resources.requests` and `resources.limits` in K8s deployment manifests to prevent noisy neighbor issues and enable effective horizontal pod autoscaling.
- Set `terminationGracePeriodSeconds` to match or exceed your application's graceful shutdown timeout.

### Logging
- Log exclusively to `stdout` and `stderr` using structured JSON format (e.g., via `pino` or `winston` with JSON transport).
- Do NOT write log files to the container filesystem. Let the cluster's log aggregation system (FluentBit, Fluentd, Loki) collect from stdout.
- Include `requestId`, `traceId`, and `spanId` in structured log entries for distributed tracing correlation.
