---
title: "Node.js Architecture & Design Standards"
tags: ["node", "architecture", "typescript"]
description: "Defines layered architecture standards, dependency injection patterns, centralized error handling, and framework selection criteria for Node.js/TypeScript applications."
---

# Node.js Architecture & Design Standards

## 1. Layered Architecture
- **Separation of Concerns**: Structure projects using at minimum three layers: `API/Routes`, `Service/Business Logic`, and `Data Access/Repository`.
- **API Layer**: Express/Fastify route handlers should be thin. Extract request parameters, call the service layer, and format the response. No business logic belongs here.
- **Service Layer**: Contains all business rules, validation, and orchestration. Services should be framework-agnostic and unit-testable without HTTP dependencies.
- **Data Access Layer**: Encapsulate all database interactions behind repository interfaces. This enables swapping storage backends without touching business logic.

## 2. Dependency Injection
- Use a DI container (`tsyringe`, `awilix`, or NestJS's built-in DI) to manage service lifetimes and decouple modules.
- Avoid importing concrete implementations directly in service files. Depend on abstractions (interfaces/types) and inject implementations at composition root.
- Prefer constructor injection over property injection for explicit, testable dependencies.

## 3. Error Handling
- **Centralized Error Handler**: Implement a single Express/Fastify error-handling middleware that catches all unhandled errors, logs them, and returns standardized error responses (RFC 7807 Problem Details).
- **Operational vs Programmer Errors**: Distinguish between operational errors (e.g., database timeout, invalid input) and programmer errors (e.g., `TypeError`, `ReferenceError`). Operational errors should be handled gracefully; programmer errors should crash the process and let the process manager restart.
- **Never Swallow Errors**: Always attach `.catch()` to Promises or use `try/catch` in `async/await` blocks. Unhandled rejections should terminate the process (`process.on('unhandledRejection', ...)` with exit).

## 4. Design Patterns
- **Middleware Pattern**: Use composable middleware chains for cross-cutting concerns (authentication, logging, rate limiting, request validation).
- **Event-Driven Architecture**: Use Node.js `EventEmitter` or message queues (BullMQ, RabbitMQ) for decoupling long-running or side-effect-heavy operations from the request/response cycle.
- **Circuit Breaker**: Implement circuit breakers (`opossum`) for external service calls to prevent cascade failures.
- **Repository Pattern**: Abstract data persistence behind repository interfaces to enable unit testing with in-memory fakes.

## 5. Framework Selection
- **Express**: Mature, massive ecosystem, widely understood. Best for teams that need maximum flexibility and community support.
- **Fastify**: Higher performance, built-in schema validation (via JSON Schema), superior plugin architecture. Recommended for new high-throughput APIs.
- **NestJS**: Opinionated, Angular-inspired framework with built-in DI, decorators, and modular architecture. Ideal for large enterprise applications requiring strict architectural guardrails.
