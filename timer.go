package whTimer

import (
	"sync/atomic"
	"time"
)

// MaxDuration 最大支持的定时时长
var MaxDuration = time.Duration(maxMs[MaxLevel]) * time.Millisecond

// Timer 高性能定时器
type Timer struct {
	wheel      *Wheel
	start      time.Time
	numEntries uint64

	queue *MPSCQueue

	wakeChan   chan struct{}
	stopChan   chan struct{}
	doneChan   chan struct{}
	sleepUntil atomic.Int64

	handler func(*Entry)
	running atomic.Bool
}

// NewTimer 创建新的定时器
func NewTimer(handler func(*Entry)) *Timer {
	return &Timer{
		queue:    NewMPSCQueue(),
		wakeChan: make(chan struct{}, 1),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
		handler:  handler,
	}
}

// Start 启动定时器
func (t *Timer) Start() {
	if t.running.Swap(true) {
		return
	}
	go t.run()
}

// Stop 停止定时器
func (t *Timer) Stop() {
	if !t.running.Swap(false) {
		return
	}
	close(t.stopChan)
	<-t.doneChan
}

// AddEntry 添加定时任务 - Wait-Free
func (t *Timer) AddEntry(delay time.Duration, callback func()) *Entry {
	return t.AddEntryAt(time.Now().Add(delay), callback)
}

// AddEntryAt 在指定时间添加定时任务 - Wait-Free
func (t *Timer) AddEntryAt(expireAt time.Time, callback func()) *Entry {
	entry := NewEntry(expireAt, callback)

	wasEmpty := t.queue.Push(entry)

	sleepUntil := t.sleepUntil.Load()
	if wasEmpty || (sleepUntil > 0 && expireAt.UnixNano() < sleepUntil) {
		select {
		case t.wakeChan <- struct{}{}:
		default:
		}
	}

	return entry
}

func (t *Timer) run() {
	defer close(t.doneChan)

	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	for {
		t.drainQueue()
		t.handleExpired()

		nextWake := t.calculateNextWake()

		if nextWake == nil {
			t.sleepUntil.Store(0)
			select {
			case <-t.stopChan:
				return
			case <-t.wakeChan:
				continue
			}
		}

		t.sleepUntil.Store(nextWake.UnixNano())

		sleepDuration := time.Until(*nextWake)
		if sleepDuration <= 0 {
			continue
		}

		timer.Reset(sleepDuration)

		select {
		case <-t.stopChan:
			timer.Stop()
			return
		case <-timer.C:
		case <-t.wakeChan:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
	}
}

func (t *Timer) drainQueue() {
	t.queue.DrainAll(func(entry *Entry) {
		t.addToWheel(entry)
	})
}

func (t *Timer) addToWheel(entry *Entry) {
	now := time.Now()

	if entry.expireAt.Before(now) || entry.expireAt.Equal(now) {
		t.handler(entry)
		return
	}

	if t.wheel == nil {
		t.start = now
		interval := uint64(entry.expireAt.Sub(now).Milliseconds())
		t.buildWheelAndAdd(entry, interval)
	} else {
		interval := uint64(entry.expireAt.Sub(t.start).Milliseconds())
		t.levelUpAndAdd(entry, interval)
	}
	t.numEntries++
}

func (t *Timer) buildWheelAndAdd(entry *Entry, interval uint64) {
	level := 0
	for level < MaxLevel {
		if interval < maxMs[level] {
			break
		}
		level++
	}
	t.wheel = NewWheel(level)
	t.wheel.AddEntry(entry, interval)
}

func (t *Timer) levelUpAndAdd(entry *Entry, interval uint64) {
	for interval >= t.wheel.MaxMs() && t.wheel.Level() < MaxLevel {
		t.wheel = t.wheel.LevelUp()
	}
	t.wheel.AddEntry(entry, interval)
}

func (t *Timer) handleExpired() {
	if t.wheel == nil || t.numEntries == 0 {
		return
	}

	now := time.Now()
	interval := uint64(now.Sub(t.start).Milliseconds())

	count := t.wheel.HandleExpiredEntries(t.handler, interval)
	t.numEntries -= uint64(count)

	t.maintenance(interval)
}

func (t *Timer) maintenance(interval uint64) {
	if t.wheel == nil {
		return
	}

	if t.wheel.Empty() {
		t.wheel = nil
		t.numEntries = 0
		return
	}

	n := interval / t.wheel.MsPerSlot()
	if n > 0 {
		t.wheel.Rotate(n)
		t.start = t.start.Add(time.Duration(n*t.wheel.MsPerSlot()) * time.Millisecond)
	}

	t.levelDownIfNeeded()
}

func (t *Timer) levelDownIfNeeded() {
	for t.wheel != nil && t.wheel.CanLevelDown() {
		t.wheel = t.wheel.LevelDown()
	}
}

func (t *Timer) calculateNextWake() *time.Time {
	if t.wheel == nil || t.numEntries == 0 {
		return nil
	}

	nextMs := t.wheel.NextExpirationTime()
	now := time.Now()
	interval := uint64(now.Sub(t.start).Milliseconds())

	if nextMs <= interval {
		result := now
		return &result
	}

	result := t.start.Add(time.Duration(nextMs) * time.Millisecond)
	return &result
}

// Pending 返回待处理任务数量
func (t *Timer) Pending() uint64 {
	return t.numEntries
}
