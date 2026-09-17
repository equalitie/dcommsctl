package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
)

// LOG_LEVEL names the minimum severity that gets written. Every level below the named
// one is suppressed:
//
//	none  - nothing at all
//	error - ERROR
//	warn  - WARN and ERROR
//	info  - INFO, WARN, ERROR (the default)
//	debug - DEBUG, INFO, WARN, ERROR
//
// An unset or invalid value falls back to info.
//
// Output is JSON on stdout, one object per line.
const (
	levelNameNone  = "none"
	levelNameError = "error"
	levelNameWarn  = "warn"
	levelNameInfo  = "info"
	levelNameDebug = "debug"
)

// levelThresholds maps each accepted LOG_LEVEL name to the slog level it enables.
// The named level and everything more severe is written; the rest is dropped.
var levelThresholds = map[string]slog.Level{
	levelNameError: slog.LevelError,
	levelNameWarn:  slog.LevelWarn,
	levelNameInfo:  slog.LevelInfo,
	levelNameDebug: slog.LevelDebug,
}

// initLogging configures the default slog logger from LOG_LEVEL.
//
// "none" installs a handler that discards everything rather than merely raising the
// level out of reach, so even a stray slog.Error cannot leak a line someone asked not
// to see.
func initLogging() {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))

	if name == levelNameNone {
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
		return
	}

	level, ok := levelThresholds[name]
	if !ok {
		if name != "" {
			// Configure first, so the fallback notice is actually emitted.
			slog.SetDefault(newJSONLogger(os.Stdout, slog.LevelInfo))
			logInfo("unrecognised LOG_LEVEL, defaulting",
				"LOG_LEVEL", os.Getenv("LOG_LEVEL"), "using", levelNameInfo)
			return
		}

		level = slog.LevelInfo
	}

	slog.SetDefault(newJSONLogger(os.Stdout, level))
}

func newJSONLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

// The helpers below wrap slog for the shapes this daemon logs most often. They are
// deliberately thin: slog is the mechanism, these only save repeating the payload
// handling at every call site.

// logInfo logs at info level with alternating key/value pairs.
func logInfo(msg string, args ...any) {
	slog.Info(msg, args...)
}

// logWarn logs at warn level with alternating key/value pairs.
func logWarn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// logError logs at error level with alternating key/value pairs. For an error value,
// pass it as a trailing field so it renders as a field rather than inside the message:
//
//	logError("job failed", "job", id, "err", err)
func logError(msg string, args ...any) {
	slog.Error(msg, args...)
}

// logFatal logs at error level and exits with status 1. It is for startup failures
// where continuing would leave the process in a half-configured state (a missing
// socket, an unbound port).
//
// Unlike the old log.Fatalf, the deferred functions in main do NOT run, and shutdown is
// skipped -- which is intended, since there is nothing serving to shut down.
func logFatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

// logDebug logs at debug level with alternating key/value pairs. For anything
// carrying a JSON payload, prefer logJSON.
func logDebug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

// logJSON logs at debug level with a JSON payload under the "json" key, followed by
// any extra key/value pairs.
//
// This is the workhorse for RPC traffic: request params and results arrive as JSON and
// should be emitted as JSON, not as an escaped string inside a message.
//
// A missing or empty payload omits the key entirely, so an error line does not carry a
// meaningless "json":null.
//
// The context is context.Background(): slog's context carries handler-level state
// (sampling, tracing) that a future handler may rely on.
func logJSON(msg string, payload any, args ...any) {
	logJSONLevel(slog.LevelDebug, msg, payload, args...)
}

// logJSONInfo is logJSON at info level, for payloads worth logging in normal operation.
func logJSONInfo(msg string, payload any, args ...any) {
	logJSONLevel(slog.LevelInfo, msg, payload, args...)
}

// logJSONLevel is the shared implementation behind logJSON and logJSONInfo. Callers
// outside this file should use one of those, so they do not have to import log/slog
// for a level constant.
func logJSONLevel(level slog.Level, msg string, payload any, args ...any) {
	value := jsonValue(payload)

	if value == nil {
		slog.Log(context.Background(), level, msg, args...)
		return
	}

	slog.Log(context.Background(), level, msg, append(args, slog.Any("json", value))...)
}

// jsonValue turns a payload into something the JSON handler emits as real JSON.
//
// json.RawMessage passes through untouched, preserving the exact wire form including
// key order. Re-encoding through a map would reorder keys, which is precisely what
// makes the raw params worth keeping.
//
// Typed nils are handled explicitly. A *json.RawMessage that is nil is NOT equal to an
// untyped nil interface.
//
// A value that cannot be marshalled becomes a descriptive string rather than nil or a
// panic: logging must never be the reason an RPC fails.
func jsonValue(payload any) any {
	// Catches the untyped nil. json.Marshal(nil) would otherwise SUCCEED and return the
	// literal "null", so a nil payload would slip past the caller's emptiness check and
	// be emitted as a "json":null field.
	if payload == nil {
		return nil
	}

	switch v := payload.(type) {
	case json.RawMessage:
		// Len 0 covers both nil and an allocated-but-empty slice; neither carries
		// anything worth logging, and emitting one would produce an empty "json":"" field.
		if len(v) == 0 {
			return nil
		}
		return v
	case *json.RawMessage:
		if v == nil || len(*v) == 0 {
			return nil
		}
		return *v
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "<marshal failed: " + err.Error() + ">"
	}

	return json.RawMessage(encoded)
}
