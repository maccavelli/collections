---
title: ".NET 8 Architecture & Design Standards"
tags: ["dotnet", "architecture", "csharp"]
description: "Defines layered Clean Architecture standards, CQRS/MediatR patterns, and dependency injection guidelines for .NET 8 enterprise applications."
---

# .NET 8 Architecture & Design Standards

## 1. Clean Architecture & Layering
- **Separation of Concerns**: Strictly enforce Clean Architecture. Divide solutions into `Domain`, `Application`, `Infrastructure`, and `Presentation` (API) layers.
- **Domain Layer**: Contains no dependencies on external frameworks. Houses entities, value objects, and domain events.
- **Application Layer**: Contains business logic, CQRS commands/queries, and interfaces for infrastructure.
- **Infrastructure Layer**: Implements interfaces from the Application layer (e.g., EF Core DbContexts, external API clients).
- **Presentation Layer**: Exposes REST or GraphQL endpoints. Should be as thin as possible, delegating logic to the Application layer via MediatR.

## 2. CQRS (Command Query Responsibility Segregation)
- Use **MediatR** to implement CQRS.
- Strictly separate Commands (state-changing operations) from Queries (read-only operations).
- Use distinct models for commands (DTOs) and queries (Read Models).

## 3. Dependency Injection
- Leverage the native `Microsoft.Extensions.DependencyInjection`.
- Avoid Service Locator anti-patterns. Inject only the specific interfaces needed.
- Scope correctly:
  - `Transient`: Lightweight, stateless services.
  - `Scoped`: DbContexts, Unit of Work, MediatR handlers.
  - `Singleton`: Caching services, HttpClientFactory, configuration.

## 4. API Design & Security
- **JWT Authentication**: Use `Microsoft.AspNetCore.Authentication.JwtBearer` for securing endpoints. Keep tokens stateless.
- **Minimal APIs vs Controllers**: Use Minimal APIs for microservices with simple route structures. Use Controllers for complex REST architectures requiring heavy versioning or OData.
- **Global Error Handling**: Implement `IExceptionHandler` in .NET 8 to centrally manage and format problem details (RFC 7807).

## 5. Middleware Best Practices
- Keep middleware pipeline lean. Use scoped services efficiently.
- Enforce early termination for security (e.g., rate limiting, CORS) before hitting heavy routing logic.
