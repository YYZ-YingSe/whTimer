package whTimer

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// 全流程测试：添加N条延迟D的任务，等待全部执行完成，计算平均耗时

var testCases = []struct {
	count int
	delay time.Duration
}{
	{100, 1 * time.Second},
	{10000, 5 * time.Second},
	{1000000, 30 * time.Second},
	{100000000, 1 * time.Minute},
}

func Benchmark_whTimer_FullCycle(b *testing.B) {
	for _, tc := range testCases {
		name := fmt.Sprintf("%s_delay-%s", formatCount(tc.count), formatDuration(tc.delay))
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				var executed atomic.Int64
				handler := func(e *Entry) {
					e.Execute()
					executed.Add(1)
				}

				timer := NewTimer(handler)
				timer.Start()

				start := time.Now()

				// 添加所有任务
				for j := 0; j < tc.count; j++ {
					timer.AddEntry(tc.delay, func() {})
				}

				// 等待所有任务执行完成
				target := int64(tc.count)
				deadline := time.Now().Add(tc.delay + 10*time.Second)
				for executed.Load() < target && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}

				elapsed := time.Since(start)
				timer.Stop()

				if executed.Load() != target {
					b.Fatalf("expected %d, got %d", target, executed.Load())
				}

				// 报告平均每个任务的耗时 = (总时间 - 延迟时间) / 任务数
				overhead := elapsed - tc.delay
				avgNs := float64(overhead.Nanoseconds()) / float64(tc.count)
				b.ReportMetric(avgNs, "ns/task")
				b.ReportMetric(float64(tc.count), "tasks")
			}
		})
	}
}

func Benchmark_Stdlib_FullCycle(b *testing.B) {
	for _, tc := range testCases { // 标准库跳过1亿，会超时
		name := fmt.Sprintf("%s_delay-%s", formatCount(tc.count), formatDuration(tc.delay))
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				var executed atomic.Int64
				done := make(chan struct{})

				start := time.Now()

				// 添加所有任务
				target := int64(tc.count)
				for j := 0; j < tc.count; j++ {
					time.AfterFunc(tc.delay, func() {
						if executed.Add(1) == target {
							close(done)
						}
					})
				}

				// 等待所有任务执行完成
				select {
				case <-done:
				case <-time.After(tc.delay + 10*time.Second):
					b.Fatalf("timeout: expected %d, got %d", target, executed.Load())
				}

				elapsed := time.Since(start)

				// 报告平均每个任务的耗时 = (总时间 - 延迟时间) / 任务数
				overhead := elapsed - tc.delay
				avgNs := float64(overhead.Nanoseconds()) / float64(tc.count)
				b.ReportMetric(avgNs, "ns/task")
				b.ReportMetric(float64(tc.count), "tasks")
			}
		})
	}
}

func formatCount(n int) string {
	switch n {
	case 100:
		return "100"
	case 10000:
		return "10K"
	case 1000000:
		return "1M"
	case 100000000:
		return "100M"
	default:
		return fmt.Sprintf("%d", n)
	}
}

func formatDuration(d time.Duration) string {
	switch d {
	case time.Second:
		return "1s"
	case 5 * time.Second:
		return "5s"
	case 30 * time.Second:
		return "30s"
	case time.Minute:
		return "1min"
	default:
		return d.String()
	}
}
