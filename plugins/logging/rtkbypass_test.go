package logging

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// TestMergeRealtimeMetadata_RtkBypassedTruncated pins the metadata mapping for
// the RTK "echoed truncated content" signal: message/block indices whose tool
// output was recognised as already-truncated RTK output and passed through
// unchanged. It must be persisted as rtk_bypassed_truncated so the log detail
// view can explain visible truncation on requests where nothing was compressed,
// and must stay absent when the ctx key is unset.
func TestMergeRealtimeMetadata_RtkBypassedTruncated(t *testing.T) {
	t.Run("writes_bypassed_truncated", func(t *testing.T) {
		ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
		ctx.SetValue(schemas.BifrostContextKeyRTKBypassedTruncated, []int{7, 36})

		md := mergeRealtimeMetadata(nil, ctx)
		got, ok := md["rtk_bypassed_truncated"].([]int)
		if !ok {
			t.Fatalf("rtk_bypassed_truncated missing or wrong type %T", md["rtk_bypassed_truncated"])
		}
		if !reflect.DeepEqual(got, []int{7, 36}) {
			t.Errorf("rtk_bypassed_truncated = %v, want [7 36]", got)
		}
	})

	t.Run("omits_when_unset", func(t *testing.T) {
		ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
		md := mergeRealtimeMetadata(nil, ctx)
		if _, ok := md["rtk_bypassed_truncated"]; ok {
			t.Error("rtk_bypassed_truncated must be absent when ctx key is unset")
		}
	})
}
