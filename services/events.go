// Package services event catalog (N-44 / Proposal N).
//
// This file documents the canonical event-name → payload-type mapping
// for all backend-emitted Wails events. The frontend mirrors these
// types in frontend/src/types/index.ts (WailsEvent<T> and per-channel
// aliases). When adding a new event channel, update BOTH this file
// and the TypeScript types together.
//
// Emitting an event:
//
//	s.app.Event.Emit("ai:chunk", "hello")
//	s.app.Event.Emit("terminal:output", map[string]string{
//	    "sessionId": id,
//	    "data":      output,
//	})
//
// All payloads MUST be JSON-serializable (Wails marshals them for the
// webview). The frontend handler receives a WailsEvent<T> whose `data`
// field carries the payload.
//
// Event channels:
//
//	ai:chunk            string              — a streamed token from the AI
//	ai:done             string              — finish reason (may be "")
//	ai:error            string              — error message
//	file:saved          string              — absolute path of saved file
//	terminal:output     {sessionId, data}   — a single PTY stdout/stderr chunk
//	terminal:exited     {sessionId}         — PTY process exited
//	workflow:completed  {name}              — workflow finished (Proposal R)
//
// The frontend types are:
//
//	type WailsEvent<T> = { data: T; name?: string }
//	type AIChunkEvent         = WailsEvent<string>
//	type AIDoneEvent          = WailsEvent<string>
//	type AIErrorEvent         = WailsEvent<string>
//	type FileSavedEvent       = WailsEvent<string>
//	type TerminalOutputEvent  = WailsEvent<{ sessionId: string; data: string }>
//	type TerminalExitedEvent  = WailsEvent<{ sessionId: string }>
//	type WorkflowCompletedEvent = WailsEvent<{ name: string }>
//
// This file is documentation-only — it declares no Go symbols. The
// actual Emit calls live in ai_service.go, file_service.go,
// terminal_service.go, etc.
package services
