package agent

import (
	"context"
	"testing"
)

type benchmarkEventSource struct {
	events <-chan ConnectionEvent
}

func (s benchmarkEventSource) Subscribe(ctx context.Context) (<-chan ConnectionEvent, error) {
	return s.events, nil
}

func BenchmarkControllerStatusParallel(b *testing.B) {
	b.ReportAllocs()
	controller, err := NewController(benchmarkNoopClient{}, NewServiceNode(benchmarkServiceOptions(), benchmarkServiceDescs(4, 8)))
	if err != nil {
		b.Fatalf("new controller failed: %v", err)
	}
	if err := controller.OnConnected(context.Background()); err != nil {
		b.Fatalf("on connected failed: %v", err)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = controller.Status()
		}
	})
}

func BenchmarkControllerObserveEventParallel(b *testing.B) {
	b.ReportAllocs()
	controller, err := NewController(benchmarkNoopClient{}, NewServiceNode(benchmarkServiceOptions(), benchmarkServiceDescs(4, 8)))
	if err != nil {
		b.Fatalf("new controller failed: %v", err)
	}
	event := ConnectionEvent{
		Type:      ConnectionEventTypeHeartbeat,
		Connected: true,
		EventId:   "heartbeat-1",
		Message:   "alive",
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			controller.ObserveEvent(event)
		}
	})
}

func BenchmarkRunnerProcessEventBurst(b *testing.B) {
	b.ReportAllocs()

	cases := []struct {
		name       string
		eventCount int
	}{
		{name: "16_events", eventCount: 16},
		{name: "128_events", eventCount: 128},
		{name: "512_events", eventCount: 512},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			ctx := context.Background()
			for i := 0; i < b.N; i++ {
				controller, err := NewController(benchmarkNoopClient{}, NewServiceNode(benchmarkServiceOptions(), benchmarkServiceDescs(2, 8)))
				if err != nil {
					b.Fatalf("new controller failed: %v", err)
				}
				events := make(chan ConnectionEvent, tc.eventCount)
				for eventIndex := 0; eventIndex < tc.eventCount; eventIndex++ {
					switch eventIndex % 3 {
					case 0:
						events <- ConnectionEvent{Type: ConnectionEventTypeConnected, Connected: true}
					case 1:
						events <- ConnectionEvent{Type: ConnectionEventTypeHeartbeat, Connected: true}
					default:
						events <- ConnectionEvent{Type: ConnectionEventTypeDisconnected, Connected: false}
					}
				}
				close(events)

				runner, err := NewRunner(benchmarkEventSource{events: events}, controller, nil)
				if err != nil {
					b.Fatalf("new runner failed: %v", err)
				}
				if err := runner.Run(ctx); err != nil {
					b.Fatalf("runner run failed: %v", err)
				}
			}
		})
	}
}
