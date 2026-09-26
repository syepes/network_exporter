package target

import (
	"context"
	"encoding/json"
	"log/slog"
)

// logDebugResult serializes result to JSON and logs it at Debug level, but only
// when Debug logging is actually enabled.
//
// The probe path runs on every cycle for every target, so at the tens-of-
// thousands scale an unconditional json.Marshal (as the code previously did,
// marshaling before the level check inside logger.Debug) is pure wasted CPU and
// allocation at the default Info level. Gating on Enabled first skips the
// marshal entirely unless the operator has opted into Debug.
func logDebugResult(logger *slog.Logger, msg string, typ string, fn string, result any) {
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	bytes, err := json.Marshal(result)
	if err != nil {
		logger.Error("Failed to marshal result", "type", typ, "func", fn, "err", err)
		return
	}
	logger.Debug(msg, "type", typ, "func", fn, "result", string(bytes))
}
