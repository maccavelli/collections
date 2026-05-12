---
title: ".NET 8 Docker, Kubernetes & Containerization Standards"
tags: ["dotnet", "docker", "kubernetes", "containers"]
description: "Prescribes Dockerfile multi-stage build patterns, Kubernetes health probe configuration, graceful shutdown handling, and container runtime best practices for .NET applications."
---

# Docker, Kubernetes & Containerization Standards

## 1. Container Images & Builds
- **Multi-stage Builds**: Always use multi-stage Dockerfiles. Use the heavy SDK image (`mcr.microsoft.com/dotnet/sdk:8.0`) for building and publishing, and the lightweight ASP.NET runtime image (`mcr.microsoft.com/dotnet/aspnet:8.0`) for the final execution stage.
- **Chiseled Images**: Use Ubuntu Chiseled images (`mcr.microsoft.com/dotnet/aspnet:8.0-jammy-chiseled`) to reduce the attack surface. They contain no shell, package manager, or unnecessary OS components.
- **Non-Root Execution**: .NET 8 defaults to non-root execution (UID `1654`, user `app`). Do not revert to root unless absolutely necessary for legacy integrations. Ensure mapped volumes and mounted secrets have the correct permissions.

## 2. Environment Configuration
- Use environment variables to inject configuration logic at runtime. Avoid baking configuration files (like `appsettings.Production.json`) directly into the image if they contain secrets.
- Use `DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1` only if internationalization is strictly not required, to save memory.

## 3. Kubernetes (K8s) Best Practices
- **Health Checks**: Implement the `Microsoft.Extensions.Diagnostics.HealthChecks` package.
  - Expose `/health/liveness` to indicate if the container is running.
  - Expose `/health/readiness` to indicate if the container is ready to accept traffic (e.g., database connections are established).
- **Resource Limits**: Always define `resources.requests` and `resources.limits` in your K8s deployment manifests.
- **Graceful Shutdown**: .NET 8 handles `SIGTERM` signals natively. Ensure long-running processes respond to `IHostApplicationLifetime.ApplicationStopping` tokens so they can flush logs and disconnect cleanly before the pod terminates.
- **Logging**: Log exclusively to `stdout` and `stderr` using JSON format for efficient ingestion by FluentBit or ELK stacks. Do not write log files to the local container filesystem.
