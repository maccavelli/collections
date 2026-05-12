---
title: "TypeScript & Node.js Idioms and Conventions"
tags: ["node", "typescript", "conventions"]
description: "Covers TypeScript strict mode configuration, ESM module patterns, naming conventions, async/await best practices, immutability patterns, and project structure guidelines for Node.js applications."
---

# TypeScript & Node.js Idioms and Conventions

## 1. TypeScript-First Development
- **Strict Mode**: Enable `"strict": true` in `tsconfig.json` without exception. This activates `strictNullChecks`, `noImplicitAny`, `strictFunctionTypes`, and all other strict family options.
- **ESM Modules**: Use ES Modules (`import`/`export`) as the default module system. Set `"type": "module"` in `package.json` and `"module": "NodeNext"` / `"moduleResolution": "NodeNext"` in `tsconfig.json`.
- **Explicit Return Types**: Always declare explicit return types on exported functions and public API methods to prevent accidental type widening and improve IDE auto-completion.
- **Branded Types**: Use branded/opaque types for domain identifiers (e.g., `type UserId = string & { __brand: 'UserId' }`) to prevent accidental misuse of plain strings as IDs.

## 2. Naming Conventions
- **Variables & Functions**: Use `camelCase` for variables, functions, and method names.
- **Types & Interfaces**: Use `PascalCase` for types, interfaces, classes, and enums. Do NOT prefix interfaces with `I` (e.g., use `UserService`, not `IUserService`).
- **Constants**: Use `UPPER_SNAKE_CASE` for true compile-time constants and environment variable names.
- **Files**: Use `kebab-case` for filenames (e.g., `user-service.ts`, `auth-middleware.ts`).
- **Boolean Variables**: Prefix with `is`, `has`, `should`, `can` (e.g., `isActive`, `hasPermission`).

## 3. Async/Await Patterns
- Always use `async/await` over raw Promises or callbacks. Callbacks are a legacy pattern.
- Use `Promise.allSettled()` when executing independent async operations that should not fail fast. Use `Promise.all()` only when all operations must succeed.
- Always pass `AbortSignal` or timeout mechanisms to long-running async operations (HTTP requests, database queries) to prevent resource leaks.

## 4. Immutability & Safety
- Use `const` by default. Only use `let` when reassignment is genuinely required. Never use `var`.
- Prefer `readonly` on class properties and `Readonly<T>` utility type for function parameters that should not be mutated.
- Use `as const` assertions for literal type narrowing on configuration objects and constant arrays.
- Prefer discriminated unions over class hierarchies for modeling domain state (e.g., `type Result<T> = { ok: true; value: T } | { ok: false; error: Error }`).

## 5. Project Structure
- Group by feature/domain, not by technical role. Prefer `src/users/`, `src/orders/` over `src/controllers/`, `src/models/`.
- Use barrel exports (`index.ts`) sparingly—only at module boundaries to define the public API of a feature module.
- Keep `tsconfig.json` path aliases (`@/`) consistent with the bundler/runner configuration to avoid import resolution mismatches.

## 6. Linting & Formatting
- Use **ESLint** with `@typescript-eslint/parser` and `@typescript-eslint/eslint-plugin` for static analysis.
- Use **Prettier** for code formatting. Enforce it via CI—never rely on individual developer IDE settings.
- Enforce `no-explicit-any`, `no-unused-vars`, and `consistent-type-imports` rules.
