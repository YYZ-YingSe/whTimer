package whTimer

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

// 测试 whTimer CronInterval 性能
func Benchmark_whTimer_CronInterval(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(formatCronCount(n), func(b *testing.B) {
			var executed atomic.Int64

			timer := NewTimer(func(e *Entry) {
				e.Execute()
			})
			timer.Start()
			defer timer.Stop()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				executed.Store(0)
				entries := make([]*CronEntry, n)

				// 创建 n 个周期任务
				for j := 0; j < n; j++ {
					entries[j] = timer.CronInterval(50*time.Millisecond, func() {
						executed.Add(1)
					})
				}

				// 等待执行一轮
				time.Sleep(60 * time.Millisecond)

				// 停止所有任务
				for j := 0; j < n; j++ {
					entries[j].Stop()
				}
			}

			b.ReportMetric(float64(n), "crons")
		})
	}
}

// 测试 robfig/cron 性能
func Benchmark_robfig_Cron(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(formatCronCount(n), func(b *testing.B) {
			var executed atomic.Int64

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				executed.Store(0)
				c := cron.New(cron.WithSeconds())

				// 创建 n 个周期任务
				for j := 0; j < n; j++ {
					c.AddFunc("*/1 * * * * *", func() { // 每秒执行
						executed.Add(1)
					})
				}

				c.Start()
				time.Sleep(1100 * time.Millisecond) // 等待执行一轮
				c.Stop()
			}

			b.ReportMetric(float64(n), "crons")
		})
	}
}

// 测试添加周期任务的性能
func Benchmark_whTimer_CronAdd(b *testing.B) {
	timer := NewTimer(func(e *Entry) {
		e.Execute()
	})
	timer.Start()
	defer timer.Stop()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		entry := timer.CronInterval(time.Hour, func() {})
		entry.Stop()
	}
}

func Benchmark_robfig_CronAdd(b *testing.B) {
	c := cron.New(cron.WithSeconds())
	c.Start()
	defer c.Stop()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		id, _ := c.AddFunc("0 0 * * * *", func() {})
		c.Remove(id)
	}
}

// 测试内存占用
func Benchmark_whTimer_CronMemory(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		b.Run(formatCronCount(n), func(b *testing.B) {
			timer := NewTimer(func(e *Entry) {
				e.Execute()
			})
			timer.Start()
			defer timer.Stop()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				entries := make([]*CronEntry, n)
				for j := 0; j < n; j++ {
					entries[j] = timer.CronInterval(time.Hour, func() {})
				}
				for j := 0; j < n; j++ {
					entries[j].Stop()
				}
			}

			b.ReportMetric(float64(n), "crons")
		})
	}
}

func Benchmark_robfig_CronMemory(b *testing.B) {
	for _, n := range []int{1000, 10000} { // robfig/cron 10万会很慢
		b.Run(formatCronCount(n), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				c := cron.New(cron.WithSeconds())
				ids := make([]cron.EntryID, n)
				for j := 0; j < n; j++ {
					ids[j], _ = c.AddFunc("0 0 * * * *", func() {})
				}
				for j := 0; j < n; j++ {
					c.Remove(ids[j])
				}
			}

			b.ReportMetric(float64(n), "crons")
		})
	}
}

func formatCronCount(n int) string {
	switch n {
	case 100:
		return "100"
	case 1000:
		return "1K"
	case 10000:
		return "10K"
	case 100000:
		return "100K"
	default:
		return "N"
	}
}
