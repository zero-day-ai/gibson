# Requirements: TUI Complete Implementation

## Overview
Complete the Gibson TUI implementation by connecting all stubbed/placeholder functionality to existing backend services. The TUI currently has significant gaps where commands return "not implemented" messages instead of performing actual operations.

## Problem Statement
The Gibson TUI has ~75% implementation coverage with 8 critical stub handlers and missing integrations:
- **Agent logs**: Shows file path instead of actual log content
- **Attack command**: Returns placeholder, no orchestration integration
- **Tool/Plugin install/uninstall**: All return "not implemented"
- **Tool logs**: Shows path instead of content
- **Mission logs**: Uses placeholder data instead of real execution logs
- **Findings export**: Dialog exists but doesn't actually export files
- **Mission creation**: Requires workflow file parsing not implemented

## User Stories

### US-1: View Component Logs in TUI
**As a** security operator
**I want to** view agent, tool, and plugin logs directly in the TUI
**So that** I can debug component behavior without switching to CLI

**Acceptance Criteria:**
- `/agent logs <name>` displays actual log content with tail functionality
- `/tool logs <name>` displays actual log content with tail functionality
- `/plugin logs <name>` displays actual log content with tail functionality
- Support `--follow` flag for real-time log streaming
- Support `--lines N` flag to control output lines
- Logs display in scrollable viewport

### US-2: Execute Attacks from TUI
**As a** security tester
**I want to** run attacks against targets directly from the TUI console
**So that** I can execute security tests without leaving the TUI

**Acceptance Criteria:**
- `/attack <target> --agent <name> --goal "<objective>"` executes attack
- Attack progress streams to console in real-time
- Findings are displayed as discovered
- Attack can be cancelled with Ctrl+C
- Attack status shows in dashboard during execution
- Support all attack flags (--mode, --payloads, --max-findings, etc.)

### US-3: Install Components from TUI
**As a** Gibson administrator
**I want to** install agents, tools, and plugins from the TUI
**So that** I can manage components without switching to CLI

**Acceptance Criteria:**
- `/agent install <github-url>` installs agent
- `/tool install <github-url>` installs tool
- `/plugin install <github-url>` installs plugin
- Installation progress displays in console
- Build output streams in real-time
- Success/failure clearly indicated
- Component appears in status after install

### US-4: Uninstall Components from TUI
**As a** Gibson administrator
**I want to** uninstall agents, tools, and plugins from the TUI
**So that** I can remove components without switching to CLI

**Acceptance Criteria:**
- `/agent uninstall <name>` uninstalls agent
- `/tool uninstall <name>` uninstalls tool
- `/plugin uninstall <name>` uninstalls plugin
- Confirmation prompt before uninstall
- Component stops if running before uninstall
- Component removed from registry after uninstall

### US-5: View Mission Execution Logs
**As a** security operator
**I want to** view real mission execution logs in the Mission view
**So that** I can monitor and debug mission execution

**Acceptance Criteria:**
- Mission view shows actual execution logs, not placeholders
- Logs update in real-time during mission execution
- Logs persist after mission completion for review
- Log viewport is scrollable with search capability
- Agent output, tool invocations, and findings visible in logs

### US-6: Export Findings to Files
**As a** security analyst
**I want to** export findings to files from the TUI
**So that** I can generate reports without using CLI

**Acceptance Criteria:**
- Export dialog writes actual files, not just showing success message
- JSON export produces valid JSON file
- SARIF export produces SARIF 2.1.0 compliant file
- CSV export produces proper CSV with headers
- Markdown export produces readable markdown report
- HTML export produces styled HTML report
- Export respects current filters (severity, mission, etc.)
- File path shown after successful export

### US-7: Create Missions from Workflow Files
**As a** security operator
**I want to** create new missions from workflow files in the TUI
**So that** I can start security assessments without using CLI

**Acceptance Criteria:**
- `/mission create -f <workflow.yaml>` parses and creates mission
- Workflow validation errors clearly displayed
- Created mission appears in mission list
- Mission can be started immediately after creation
- Support inline workflow definition or file path

## Technical Requirements

### TR-1: Leverage Existing Backend Services
- Use existing `cmd/gibson/component/logs.go` log reading functions
- Use existing `internal/attack/attack.go` AttackRunner
- Use existing `cmd/gibson/component/install.go` installation logic
- Use existing `internal/finding/export/` exporters
- Use existing `internal/mission/controller.go` for mission creation

### TR-2: Console Command Handler Integration
- Fix flag parsing in CommandHandler (currently broken)
- Pass ParsedCommand to handlers for proper flag access
- Maintain backward compatibility with existing working handlers

### TR-3: Real-time Streaming
- Agent logs use existing file streaming with `followLogs()`
- Attack progress uses existing EventProcessor/EventRenderer
- Mission logs integrate with workflow executor events

### TR-4: Error Handling
- All operations must have proper error handling
- Errors display clearly in console with context
- Failed operations don't crash the TUI

## Non-Functional Requirements

### NFR-1: Performance
- Log streaming must not block UI
- Large log files handle efficiently (streaming, not loading all)
- Export operations complete within reasonable time

### NFR-2: Consistency
- TUI commands behave identically to CLI equivalents
- Output formatting matches CLI where appropriate
- Help text accurate and complete

### NFR-3: User Experience
- All operations provide progress feedback
- Long-running operations can be cancelled
- Success/failure clearly communicated

## Out of Scope
- New backend services or APIs (use existing)
- Real-time mission progress streaming API (future enhancement)
- WebSocket-based log streaming (future enhancement)
- Agent capability discovery enhancements
- System metrics dashboard panel

## Dependencies
- Existing component lifecycle manager
- Existing finding store and exporters
- Existing attack runner and orchestrator
- Existing mission controller and service
- Existing workflow executor

## Success Metrics
- 0 commands return "not implemented" in TUI
- All 8 stub handlers fully implemented
- All console commands work identically to CLI equivalents
- Findings export creates actual files
- Mission view shows real execution logs
