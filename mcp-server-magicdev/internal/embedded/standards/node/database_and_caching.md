---
title: "Node.js Database, Persistence & Caching Standards"
tags: ["node", "database", "redis", "mongodb", "sqlserver", "elasticsearch"]
description: "Standards for Redis caching patterns, MongoDB/Mongoose schema design, SQL Server parameterized queries, Elasticsearch bulk indexing, and general connection lifecycle management in Node.js applications."
---

# Database, Persistence & Caching Standards

## 1. Redis
### Library Selection
- **`node-redis`** (Official): Recommended default for new projects. Clean Promise-based API, built-in TypeScript support, and full compatibility with modern Redis modules (JSON, Search, TimeSeries).
- **`ioredis`** (Community): Preferred for complex infrastructure requiring advanced Redis Cluster and Sentinel support. Battle-tested reconnection logic. Required by task queue libraries like **BullMQ**.
- **Recommendation**: Use `node-redis` for general-purpose caching. Use `ioredis` for cluster deployments or when using BullMQ.

### Patterns
- Implement **Cache-Aside** pattern: Check cache first, fall back to primary database on miss, populate cache on read.
- Use colon-delimited key namespacing (e.g., `user:profile:{userId}`).
- Always set TTLs on cached entries to prevent unbounded memory growth.
- Use Redis Pub/Sub or Streams for real-time event propagation between microservices instead of polling.

## 2. MongoDB
### Library Selection
- **Mongoose (ODM)**: Standard for CRUD-heavy web applications. Provides schema enforcement, validation, middleware (pre/post hooks), population for relationships, and virtuals. Start here for rapid development and data integrity.
- **Native MongoDB Driver**: Maximum performance, zero abstraction overhead. Ideal for complex aggregation pipelines or high-throughput paths where Mongoose hydration overhead is measurable.
- **Recommendation**: Start with Mongoose. Drop to the native driver for specific performance-critical paths.

### Patterns
- Define indexes explicitly in schema definitions or deployment scripts. Never rely on runtime auto-indexing in production.
- Use `lean()` on Mongoose queries that return read-only data to skip document hydration and gain significant performance.
- Design document schemas to be naturally atomic. Avoid multi-document transactions unless absolutely necessary.
- Use change streams for reactive, event-driven data processing.

## 3. SQL Server
### Library Selection
- **`mssql`**: The standard Node.js interface for SQL Server, built on top of the `tedious` TDS driver. Supports connection pooling, transactions, prepared statements, and streaming.
- **Prisma**: Type-safe ORM with first-class SQL Server support. Recommended for teams prioritizing developer experience, schema migrations, and type safety over raw SQL flexibility.

### Patterns
- Always use parameterized queries or prepared statements. Never interpolate user input into SQL strings.
- Use connection pooling (`mssql.ConnectionPool`) with explicit `max`/`min` pool sizes configured for your expected concurrency.
- Wrap multi-step operations in explicit transactions using `pool.transaction()`.

## 4. Elasticsearch
### Library Selection
- **`@elastic/elasticsearch`**: The official Node.js client. Supports all Elasticsearch APIs, connection pooling, sniffing, and automatic retries.

### Patterns
- Map DTOs to explicit index templates. Avoid relying on dynamic mapping in production—it leads to mapping explosions and unpredictable field types.
- Use structured queries (`term`, `range`) for exact matches and `match`/`multi_match` for full-text search.
- Always use the **Bulk API** for batch indexing or updating operations. Individual document writes at scale are catastrophically slow.
- Implement scroll or `search_after` for paginating large result sets instead of deep `from`/`size` pagination.

## 5. General Database Patterns
- **Connection Management**: Initialize database connections at application startup. Close them gracefully on `SIGTERM`/`SIGINT`. Never open connections per-request.
- **Health Checks**: Each database client should expose a `ping()` or equivalent health check method wired into the application's readiness probe.
- **Retry Logic**: Implement exponential backoff with jitter for transient connection failures.
