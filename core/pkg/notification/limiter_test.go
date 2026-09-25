package notification

import (
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

func TestLimiterDeduplication(t *testing.T) {
	lim := NewLimiter(20.0, 10.0, 200*time.Millisecond)

	frame1 := &phonebridgev1.NotificationFrame{
		Event: &phonebridgev1.NotificationFrame_Posted{
			Posted: &phonebridgev1.NotificationPosted{
				Key:         "0|com.app|1|null|1000",
				PackageName: "com.app",
				Title:       "Progress",
				Text:        "50%",
			},
		},
	}

	// First send -> allowed
	if !lim.Allow(frame1) {
		t.Fatal("first frame should be allowed")
	}

	// Immediate identical resend -> suppressed by dedup
	if lim.Allow(frame1) {
		t.Fatal("immediate duplicate should be suppressed")
	}

	// Slightly changed content -> allowed
	frame2 := &phonebridgev1.NotificationFrame{
		Event: &phonebridgev1.NotificationFrame_Posted{
			Posted: &phonebridgev1.NotificationPosted{
				Key:         "0|com.app|1|null|1000",
				PackageName: "com.app",
				Title:       "Progress",
				Text:        "55%",
			},
		},
	}
	if !lim.Allow(frame2) {
		t.Fatal("content change should be allowed")
	}

	// Wait past dedup window -> allowed again
	time.Sleep(250 * time.Millisecond)
	if !lim.Allow(frame2) {
		t.Fatal("duplicate after dedup window should be allowed")
	}
}

func TestLimiterBurstThrottling(t *testing.T) {
	// 5 Hz sustained, burst capacity 2
	lim := NewLimiter(5.0, 2.0, 10*time.Millisecond)

	for i := 0; i < 2; i++ {
		frame := &phonebridgev1.NotificationFrame{
			Event: &phonebridgev1.NotificationFrame_Posted{
				Posted: &phonebridgev1.NotificationPosted{
					Key:         "0|com.app|1|null|1000",
					PackageName: "com.app",
					Title:       "Title",
					Text:        string(rune('A' + i)),
				},
			},
		}
		if !lim.Allow(frame) {
			t.Fatalf("frame %d within burst should be allowed", i)
		}
	}

	// 3rd frame immediately should exceed burst allowance
	frameOver := &phonebridgev1.NotificationFrame{
		Event: &phonebridgev1.NotificationFrame_Posted{
			Posted: &phonebridgev1.NotificationPosted{
				Key:         "0|com.app|1|null|1000",
				PackageName: "com.app",
				Title:       "Title",
				Text:        "Z",
			},
		},
	}
	if lim.Allow(frameOver) {
		t.Fatal("excess frame should be throttled")
	}
}
