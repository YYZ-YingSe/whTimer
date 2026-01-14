package whTimer

import (
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
)

// cron 表达式解析器 (支持秒级)
var cronParser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// CronEntry 周期任务条目
type CronEntry struct {
	timer    *Timer
	schedule cron.Schedule
	callback func()
	entry    atomic.Pointer[Entry]
	stopped  atomic.Bool
}

// Cron 使用 Cron 表达式创建周期任务
// 格式: "秒 分 时 日 月 星期"
// 示例: "0 30 9 * * 1-5" 每周一到周五 9:30:00 执行
func (t *Timer) Cron(expr string, callback func()) (*CronEntry, error) {
	schedule, err := cronParser.Parse(expr)
	if err != nil {
		return nil, err
	}

	c := &CronEntry{
		timer:    t,
		schedule: schedule,
		callback: callback,
	}
	c.scheduleNext()
	return c, nil
}

// CronAt 在指定时间执行一次
func (t *Timer) CronAt(at time.Time, callback func()) *CronEntry {
	c := &CronEntry{
		timer:    t,
		callback: callback,
	}
	entry := t.AddEntryAt(at, func() {
		if !c.stopped.Load() {
			callback()
		}
	})
	c.entry.Store(entry)
	return c
}

// CronInterval 按固定间隔执行
func (t *Timer) CronInterval(interval time.Duration, callback func()) *CronEntry {
	c := &CronEntry{
		timer:    t,
		callback: callback,
	}

	var scheduleNext func()
	scheduleNext = func() {
		if c.stopped.Load() {
			return
		}
		entry := t.AddEntry(interval, func() {
			if !c.stopped.Load() {
				callback()
				scheduleNext()
			}
		})
		c.entry.Store(entry)
	}
	scheduleNext()
	return c
}

func (c *CronEntry) scheduleNext() {
	if c.stopped.Load() || c.schedule == nil {
		return
	}

	next := c.schedule.Next(time.Now())
	entry := c.timer.AddEntryAt(next, func() {
		if !c.stopped.Load() {
			c.callback()
			c.scheduleNext()
		}
	})
	c.entry.Store(entry)
}

// Stop 停止周期任务
func (c *CronEntry) Stop() {
	c.stopped.Store(true)
	if entry := c.entry.Load(); entry != nil {
		entry.Cancel()
	}
}

// IsStopped 检查是否已停止
func (c *CronEntry) IsStopped() bool {
	return c.stopped.Load()
}
