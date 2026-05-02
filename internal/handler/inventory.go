package handler

// InternalToolsInventoryJSON provides a static, SDK-adherent definition
// for all internal magictools orchestrator capabilities.
// This list is used to guarantee tool integrity during tools/list responses.
var InternalToolsInventoryJSON = []byte(`
[
  {
    "name": "sync_ecosystem",
    "description": "[DIRECTIVE: Lifecycle] Synchronize all managed sub-servers to globally refresh tool indices. Call with an empty schema to initialize before cross-server interactions. Keywords: refresh global initialize synchronize orchestrator tool ecosystem",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {}
    }
  },
  {
    "name": "sync_server",
    "description": "[DIRECTIVE: Lifecycle] Refresh the dynamic tool index for a specific sub-server locally. Keywords: refresh local subserver sync dynamic index single",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "name": {
          "type": "string",
          "description": "The name of the server to sync"
        }
      },
      "required": [
        "name"
      ]
    }
  },
  {
    "name": "sleep_servers",
    "description": "[DIRECTIVE: Lifecycle] Gracefully hibernate all active sub-server processes to conserve memory. Keywords: hibernate limit memory suspend standby sleep",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {}
    }
  },
  {
    "name": "wake_servers",
    "description": "[DIRECTIVE: Lifecycle] Bring all sleeping sub-servers back to hot standby. Keywords: wake start resume standby initialize",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {}
    }
  },
  {
    "name": "reload_servers",
    "description": "[DIRECTIVE: Lifecycle] Forcefully terminate and restart sub-server processes. Pass space-separated 'names' to selectively reload, or omit to reload all servers. Use to recover from timeouts. Keywords: force terminate restart reload timeouts recover",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "names": {
          "type": "string",
          "description": "Optional space-separated list of server names to reload (e.g. 'go-refactor recall'). If omitted, all servers are reloaded."
        }
      }
    }
  },
  {
    "name": "get_internal_logs",
    "description": "[DIRECTIVE: System Diagnostic] Retrieve orchestrator diagnostic system logs. Pass 'max_lines' to limit the output. Keywords: diagnostic retrieve system limit view orchestrator logs",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "max_lines": {
          "type": "integer",
          "description": "Maximum number of recent log lines to return (default 50)."
        }
      }
    }
  },
  {
    "name": "get_session_stats",
    "description": "[DIRECTIVE: Network Telemetry] Analyzes active sub-server handler network overhead latencies dynamically. Produces structural efficiency boundaries and orchestrator latency values. Keywords: network telemetry latency overhead structural bound efficiency",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {}
    }
  },
  {
    "name": "get_health_report",
    "description": "[DIRECTIVE: System Config] Returns the health status of all managed MCP servers, showing which are online or offline. Keywords: health status online offline diagnostics node up down verify",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {}
    }
  },
  {
    "name": "analyze_system_logs",
    "description": "[DIRECTIVE: Predictive Telemetry] Provides a high-resolution, predictive-telemetry filtered view of orchestrator structural logs. Filter via 'server_id', 'lines', or 'severity' (ERROR, WARNING, CRITICAL). Correlates organic error vectors for self-healing. Produces telemetry markdown blocks. Keywords: predictive error critical trace logs filter diagnose parse syslog vector",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "server_id": {
          "type": "string",
          "description": "Optional: Filter logs for a specific sub-server (e.g. 'github', 'ddg-search')."
        },
        "lines": {
          "type": "integer",
          "description": "Number of recent log lines to scan (default 50)."
        },
        "severity": {
          "type": "string",
          "description": "Optional: Filter by ERROR, WARNING, or CRITICAL.",
          "enum": [
            "ERROR",
            "WARNING",
            "CRITICAL"
          ]
        }
      }
    }
  },
  {
    "name": "align_tools",
    "description": "[DIRECTIVE: Routing Oracle] The primary Bleve-backed intent mapping engine for orchestrator capability resolution. DO NOT use this as a naive filter string by setting 'server_name' alone. Provide a rich semantic string to the 'query' field describing your exact goal (e.g., 'evaluate complex logic in go'). Automatically scores highest on algorithmic proxy resolution algorithms. Keywords: oracle bleve schema search intent discover route evaluate strategy proxy mapping",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "query": {
          "type": "string",
          "description": "Search query for tool name, description or category. Leave empty to list all tools for a server."
        },
        "server_name": {
          "type": "string",
          "description": "Optional name of the sub-server to pull tools for (e.g. 'recall')."
        },
        "category": {
          "type": "string",
          "description": "Optional category filter"
        },
        "full_schema": {
          "type": "boolean",
          "description": "Optional payload parameter to force returning massive raw JSON descriptors natively bypassingly"
        }
      }
    }
  },
  {
    "name": "call_proxy",
    "description": "[DIRECTIVE: Execution Gateway] The absolute execution endpoint for orchestrating downstream MCP nodes natively. You MUST invoke this securely, strictly bypassing legacy list enumerations. Pass the URN mapped from align_tools and perfectly map the discovered schema. Carries the highest telemetry weight. Keywords: absolute bypass execute downstream run shell proxy native payload map",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "urn": {
          "type": "string"
        },
        "arguments": {
          "type": "object"
        },
        "bypass_minification": {
          "type": "boolean",
          "description": "Skip the Markdown transformation and return raw JSON (Still protected by 10MB Slicer)"
        }
      },
      "required": [
        "urn",
        "arguments"
      ]
    }
  },
  {
    "name": "update_config",
    "description": "[DIRECTIVE: System Config] Updates a magictools configuration variable instantly to persistent storage. Pass required 'key' and 'value'. Produces modified config.yaml boundaries. Keywords: modify set persistent variable configure yaml parameters save state",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "key": {
          "type": "string",
          "description": "Configuration key to update. One of: logLevel, mcpLogLevel, squeezeLevel, logFormat, scoreThreshold, validateProxyCalls, pinnedServers, squeezeBypass",
          "enum": [
            "logLevel",
            "mcpLogLevel",
            "squeezeLevel",
            "logFormat",
            "scoreThreshold",
            "validateProxyCalls",
            "pinnedServers",
            "squeezeBypass"
          ]
        },
        "value": {
          "type": "string",
          "description": "New value for the key. Examples: 'DEBUG', '3', '0.5', 'true', 'recall brainstorm' (space-separated for list keys)"
        }
      },
      "required": [
        "key",
        "value"
      ]
    }
  },
  {
    "name": "self_check",
    "description": "[DIRECTIVE: System Config] Get orchestrator system health: CPU, memory, cache stats, and database integrity. Keywords: cpu cache integrity health db memory test validator orchestrator metrics",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {}
    }
  },
  {
    "name": "list_tools",
    "description": "[DIRECTIVE: Exhaustive Enumeration] Directly lists all available tools from a specific sub-server natively without pagination or AI semantic truncation. Use ONLY when explicitly prompted to list all tools for a given target, bypassing intent alignment restrictions. Keywords: exhaustive bypass raw exact enumerate native list index dump paginated",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "server_name": {
          "type": "string",
          "description": "Optional name of the sub-server to pull tools for (e.g. 'recall')."
        },
        "options": {
          "type": "object",
          "properties": {
            "max_tools": {
              "type": "integer",
              "description": "Maximum number of tools to return."
            }
          }
        }
      }
    }
  },
  {
    "name": "semantic_similarity",
    "description": "[DIRECTIVE: Vector Diagnostic] Identifies redundancy patterns across the ecosystem employing Cosine Distance thresholds organically. Produces deduplication Markdown maps natively routing towards view_file. Keywords: identify distance redundancy dedupe patterns map trace organic similar",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "servers": {
          "type": "string",
          "description": "Optional space-separated list of server names to restrict the structural audit to."
        }
      }
    }
  },
  {
    "name": "query_standards",
    "description": "[DIRECTIVE: Architectural Oracle] Queries the unified vector memory matrices returning exact mathematical constraints across local structural frameworks dynamically using spatial embeddings. Pass 'query' containing architectural keywords. Produces top 5 conceptual boundary URIs. Keywords: mathematical framework pattern strict structure standard boundary spatial search oracle embedding",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "query": {
          "type": "string",
          "description": "Natural language query to search standards and architectural patterns."
        }
      },
      "required": [
        "query"
      ]
    }
  },
  {
    "name": "execute_pipeline",
    "description": "[DIRECTIVE: Autonomous DAG Executor] Composes and executes a full pipeline DAG end-to-end with automatic output chaining, Socratic Trifecta gate enforcement, and Option-A AST-based MUTATOR transformation. Pass 'target' (project path) and 'intent'. The executor runs ALL steps sequentially, chains outputs as context inputs, blocks MUTATOR steps until Socratic approval passes, and applies automated AST fixes (missing godoc, struct tag compliance) through apply_vetted_edit. Keywords: execute run autonomous pipeline dag sequential chain mutator ast transform",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "session_id": {
          "type": "string",
          "description": "Optional CSSA session correlation ID. Auto-generated if omitted. For CONTINUATION MODE: pass session_id WITHOUT target/intent to resume a paused pipeline after human approval."
        },
        "target": {
          "type": "string",
          "description": "Absolute path to the project root or package to analyze and refactor."
        },
        "intent": {
          "type": "string",
          "description": "What the pipeline should accomplish (e.g., 'evaluate and refactor for enterprise compliance')."
        },
        "plan_hash": {
          "type": "string",
          "description": "Optional SHA-256 hash of an approved implementation plan for MUTATOR integrity verification."
        },
        "dry_run": {
          "type": "boolean",
          "description": "When true, return the DAG plan as markdown without executing. Equivalent to the old compose_pipeline preview."
        },
        "target_roles": {
          "type": "array",
          "items": { "type": "string" },
          "description": "Optional role filter (e.g. ['ANALYZER', 'CRITIC']). Only tools matching these roles enter the DAG."
        },
        "reject": {
          "type": "boolean",
          "description": "Set to true when continuing a paused pipeline to reject the plan and abort without mutations."
        }
      },
      "required": [
        "target",
        "intent"
      ]
    }
  },
  {
    "name": "validate_pipeline_step",
    "description": "[ROLE: VALIDATOR] [PHASE: 4] [DIRECTIVE: Validation Node] Mathematical pipeline constraint verifier evaluating diagnostic completion guarantees natively off CSSA backplanes. Extracts execution matrices cross-referencing completion logic natively. Pass 'step_name' bounds, 'step_output' dumps, and 'project_path'. [REQUIRES: go-refactor:suggest_fixes] Keywords: constraint execution verifier extract structural hashes math verify matrix ensure output code evaluate pipeline node refactor go idioms pause compliance",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "step_name": {
          "type": "string",
          "description": "Name of the pipeline step (e.g., 'go_complexity_analyzer')."
        },
        "step_output": {
          "type": "string",
          "description": "The output from the pipeline step to validate."
        },
        "project_path": {
          "type": "string",
          "description": "Absolute path to the project being analyzed."
        }
      },
      "required": [
        "step_name",
        "step_output"
      ]
    }
  },
  {
    "name": "cross_server_quality_gate",
    "description": "[ROLE: FIREWALL] [PHASE: 6] [DIRECTIVE: Safety Firmware] Mandatory evaluation firewall evaluating synced session states prior to filesystem executions natively. Define 'project_path' mappings and 'plan_hash'. [PIPELINE CONSTRAINT: Blocks terminal execution endpoints until all safety limits are verified structurally.] [REQUIRES: go-refactor:generate_implementation_plan] Keywords: evaluate boundary block firmware firewall safe sync execution state mapping matrix refactor go idioms pause compliance",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "project_path": {
          "type": "string",
          "description": "Absolute path to the project root."
        },
        "plan_hash": {
          "type": "string",
          "description": "SHA-256 hash of the implementation plan to verify."
        }
      },
      "required": [
        "project_path",
        "plan_hash"
      ]
    }
  },
  {
    "name": "generate_audit_report",
    "description": "[ROLE: SYNTHESIZER] [PHASE: 10] [DIRECTIVE: Compliance Gate] Mandatory finalization node to compute absolute git diff telemetry onto disk natively. Pass 'target' and global 'session_id'. Closes the orchestration loop. Produces absolute pipeline execution matrices complying with CSSA protocols. Keywords: commit file push telemetry finalize write save diff logic array report publish",
    "category": "orchestrator",
    "inputSchema": {
      "type": "object",
      "properties": {
        "target": {
          "type": "string",
          "description": "Absolute path to the project root."
        },
        "session_id": {
          "type": "string",
          "description": "The active CSSA tracking session correlation ID."
        }
      },
      "required": [
        "target",
        "session_id"
      ]
    }
  }
]
`)
