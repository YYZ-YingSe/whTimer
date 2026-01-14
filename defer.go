package whTimer

import (
	"time"
)

// AfterFunc 在 d 时间后执行 f，返回可取消的 Entry
func (t *Timer) AfterFunc(d time.Duration, f func()) *Entry {
	return t.AddEntry(d, f)
}

// AfterFuncAt 在指定时间执行 f，返回可取消的 Entry
func (t *Timer) AfterFuncAt(at time.Time, f func()) *Entry {
	return t.AddEntryAt(at, f)
}

// After 返回一个 channel，在 d 时间后发送当前时间
func (t *Timer) After(d time.Duration) <-chan time.Time {
	c := make(chan time.Time, 1)
	t.AddEntry(d, func() {
		c <- time.Now()
	})
	return c
}

// Sleep 阻塞当前 goroutine 指定时间
func (t *Timer) Sleep(d time.Duration) {
	<-t.After(d)
}
