---
title: ".NET 8 Database, Persistence & Caching Standards"
tags: ["dotnet", "database", "redis", "mongodb", "sqlserver", "elasticsearch"]
description: "Standards for Entity Framework Core migrations, repository patterns, Redis distributed caching, MongoDB/Elasticsearch integration, and connection resiliency in .NET applications."
---

# Database, Persistence & Caching Standards

## 1. Relational Data (SQL Server & PostgreSQL)
- **Entity Framework Core (EF Core) 8**: Use EF Core as the primary ORM for relational data.
- **Migrations**: Always generate migrations locally and apply them via automated deployment pipelines using `dotnet ef database update` or standalone migration bundles. Never use `EnsureCreated()` in production.
- **Query Optimization**: 
  - Use `AsNoTracking()` for read-only queries to improve performance.
  - Avoid N+1 query issues by explicitly loading related data using `.Include()` or utilizing split queries (`.AsSplitQuery()`) for complex joins.
- **Connection Resiliency**: Configure `EnableRetryOnFailure()` in the DbContext setup to handle transient SQL connection drops.

## 2. Distributed Caching (Redis)
- **IDistributedCache**: Implement Redis using `Microsoft.Extensions.Caching.StackExchangeRedis`.
- **Patterns**: Use the Cache-Aside pattern. Fallback to the primary database if a cache miss occurs.
- **Serialization**: Serialize cached objects using `System.Text.Json` for maximum performance.
- **Key Naming Strategy**: Use colon-delimited namespacing (e.g., `User:Profiles:{Id}`) to logically group cache keys.

## 3. NoSQL (MongoDB)
- **Driver**: Use the official `MongoDB.Driver`.
- **Dependency Injection**: Register `IMongoClient` as a Singleton. The client manages its own connection pool.
- **Idempotency & Transactions**: While MongoDB supports multi-document transactions, design document schemas to be naturally atomic to avoid cross-document transactional overhead.
- **Indexing**: explicitly define indexes in the application startup or deployment scripts. Do not rely on ad-hoc runtime indexing.

## 4. Search & Observability (Elasticsearch)
- **Driver**: Use `Elastic.Clients.Elasticsearch` (v8+).
- **Index Management**: Map DTOs explicitly to index templates. Avoid relying heavily on dynamic mapping in production.
- **Queries**: Use structured queries (e.g., `term`, `range`) for exact matches and `match` for full-text search.
- **Bulk Operations**: Always use the Bulk API when indexing or updating multiple documents simultaneously.
