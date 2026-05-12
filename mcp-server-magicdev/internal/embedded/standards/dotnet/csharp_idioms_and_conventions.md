---
title: "C# 12 & .NET 8 Idioms and Conventions"
tags: ["dotnet", "csharp", "conventions"]
description: "Covers C# language idioms, naming conventions, async/await patterns, LINQ usage, and null-safety practices for idiomatic .NET development."
---

# C# 12 & .NET 8 Idioms and Conventions

## 1. Modern Syntax Features
- **File-scoped namespaces**: Always use file-scoped namespaces (`namespace Project.Domain;`) to reduce nesting and improve readability.
- **Primary Constructors**: Use primary constructors (C# 12) for dependency injection in classes to reduce boilerplate field assignments.
- **Records**: Use `record` or `readonly record struct` for immutable DTOs, Value Objects, and Command/Query definitions. 
- **Collection Expressions**: Leverage C# 12 collection expressions (`[]`) for concise array and list initialization.

## 2. Asynchronous Programming
- Always use `async` and `await` for I/O bound operations.
- Suffix asynchronous methods with `Async` (e.g., `GetUserAsync`).
- Prefer `CancellationToken` in all async method signatures and pass it down the call chain to ensure graceful cancellation.
- Use `ConfigureAwait(false)` in library code (though less necessary in ASP.NET Core apps, it remains a strong practice for shared SDKs).

## 3. Nullability & Safety
- Enable Nullable Reference Types (`<Nullable>enable</Nullable>`) across all projects.
- Use the null-coalescing operator (`??`) and null-conditional operator (`?.`) to handle nullable types cleanly.
- Use pattern matching (`is not null`) instead of standard inequality checks for robust null checking.

## 4. Error Handling
- Do not use exceptions for expected control flow.
- Use the `Result<T>` pattern for business logic failures, reserving exceptions for catastrophic, unexpected infrastructure errors.

## 5. Performance
- Use `Span<T>` and `Memory<T>` for high-performance parsing and slice operations without allocations.
- Prefer `IAsyncEnumerable<T>` for streaming large datasets over loading entire collections into memory.
