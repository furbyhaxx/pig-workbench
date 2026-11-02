package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

var mu sync.Mutex

func logf(kind string, fields map[string]any) {
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(os.Getenv("PROBE_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fields["ext"] = "producer"
	fields["kind"] = kind
	fields["pid"] = os.Getpid()
	fields["t"] = float64(time.Now().UnixMicro()) / 1e6
	b, _ := json.Marshal(fields)
	f.Write(append(b, '\n'))
}

func Extension() *sdk.Extension {
	e := sdk.New("probe-producer-iso")
	obj := sdk.Schema{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}, "block": map[string]any{"type": "boolean"}, "sleepMs": map[string]any{"type": "number"}, "cancelMessage": map[string]any{"type": "boolean"}}}
	e.Tool("producer_echo", "echo text", obj, func(ctx sdk.Context, p map[string]any) (any, error) {
		return fmt.Sprintf("echo: %v", p["text"]), nil
	})
	if _, err := e.Events().On("wb.reply", func(ctx sdk.Context, data any) error {
		logf("bus", map[string]any{"step": "reply-received", "data": data})
		return nil
	}); err != nil {
		logf("bus", map[string]any{"step": "subscribe-failed", "err": err.Error()})
	}
	e.Tool("producer_request", "emit a workbench request", obj, func(ctx sdk.Context, p map[string]any) (any, error) {
		id := fmt.Sprintf("req-%d", time.Now().UnixNano())
		logf("bus", map[string]any{"step": "emit-start", "id": id})
		err := ctx.Events().Emit("wb.request", map[string]any{"id": id, "sleepMs": p["sleepMs"]})
		logf("bus", map[string]any{"step": "emit-returned", "id": id, "err": fmt.Sprint(err), "ctxErr": fmt.Sprint(ctx.Err())})
		if err != nil && p["cancelMessage"] == true {
			cerr := e.Events().Emit("wb.cancel", map[string]any{"id": id})
			logf("bus", map[string]any{"step": "cancel-emitted", "id": id, "err": fmt.Sprint(cerr)})
		}
		if err != nil {
			return nil, err
		}
		return "requested " + id, nil
	})
	return e
}

func main() {
	if err := Extension().Run(); err != nil {
		panic(err)
	}
}
