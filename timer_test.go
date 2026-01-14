package whTimer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWheelBasic(t *testing.T) {
	w := NewWheel(0)

	if !w.Empty() {
		t.Error("new wheel should be empty")
	}

	// 添加entry
	entry := NewEntry(time.Now().Add(10*time.Millisecond), func() {})
	w.AddEntry(entry, 10)

	if w.Empty() {
		t.Error("wheel should not be empty after adding entry")
	}

	// 检查下次过期时间
	next := w.NextExpirationTime()
	if next != 10 {
		t.Errorf("expected next expiration time 10, got %d", next)
	}

	// 移除entry
	w.RemoveEntry(entry, 10)
	if !w.Empty() {
		t.Error("wheel should be empty after removing entry")
	}
}

func TestWheelMultiLevel(t *testing.T) {
	w := NewWheel(1) // level 1: 64ms per slot

	// 添加一个100ms后过期的任务
	entry := NewEntry(time.Now().Add(100*time.Millisecond), func() {})
	w.AddEntry(entry, 100)

	if w.Empty() {
		t.Error("wheel should not be empty")
	}

	// 100ms / 64ms = 1, 所以应该在slot 1
	next := w.NextExpirationTime()
	// level 1的slot 1 = 64ms, 加上level 0的偏移
	if next < 64 || next > 128 {
		t.Errorf("unexpected next expiration time: %d", next)
	}
}

func TestWheelLevelUp(t *testing.T) {
	w := NewWheel(0)

	// 添加一个任务
	entry := NewEntry(time.Now().Add(10*time.Millisecond), func() {})
	w.AddEntry(entry, 10)

	// 升级
	w2 := w.LevelUp()
	if w2.Level() != 1 {
		t.Errorf("expected level 1, got %d", w2.Level())
	}
}

func TestWheelHandleExpired(t *testing.T) {
	w := NewWheel(0)

	var executed int
	handler := func(e *Entry) {
		executed++
	}

	// 添加多个任务
	for i := 0; i < 5; i++ {
		entry := NewEntry(time.Now().Add(time.Duration(i+1)*time.Millisecond), func() {})
		w.AddEntry(entry, uint64(i+1))
	}

	// 处理前3ms内过期的任务
	count := w.HandleExpiredEntries(handler, 3)
	if count != 3 {
		t.Errorf("expected 3 expired entries, got %d", count)
	}
	if executed != 3 {
		t.Errorf("expected 3 executions, got %d", executed)
	}
}

func TestTimerBasic(t *testing.T) {
	var executed atomic.Int32
	handler := func(e *Entry) {
		e.Execute()
		executed.Add(1)
	}

	timer := NewTimer(handler)
	timer.Start()
	defer timer.Stop()

	// 添加一个50ms后执行的任务
	timer.AddEntry(50*time.Millisecond, func() {})

	// 等待执行
	time.Sleep(100 * time.Millisecond)

	if executed.Load() != 1 {
		t.Errorf("expected 1 execution, got %d", executed.Load())
	}
}

func TestTimerMultiple(t *testing.T) {
	var executed atomic.Int32
	handler := func(e *Entry) {
		e.Execute()
		executed.Add(1)
	}

	timer := NewTimer(handler)
	timer.Start()
	defer timer.Stop()

	// 添加多个任务
	for i := 0; i < 10; i++ {
		timer.AddEntry(time.Duration(10+i*5)*time.Millisecond, func() {})
	}

	// 等待所有任务执行
	time.Sleep(200 * time.Millisecond)

	if executed.Load() != 10 {
		t.Errorf("expected 10 executions, got %d", executed.Load())
	}
}

func TestTimerWakeUp(t *testing.T) {
	var order []int
	var mu sync.Mutex
	handler := func(e *Entry) {
		e.Execute()
	}

	timer := NewTimer(handler)
	timer.Start()
	defer timer.Stop()

	// 先添加一个100ms后的任务
	timer.AddEntry(100*time.Millisecond, func() {
		mu.Lock()
		order = append(order, 2)
		mu.Unlock()
	})

	// 再添加一个20ms后的任务，应该唤醒timer
	time.Sleep(10 * time.Millisecond)
	timer.AddEntry(20*time.Millisecond, func() {
		mu.Lock()
		order = append(order, 1)
		mu.Unlock()
	})

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 2 {
		t.Errorf("expected 2 executions, got %d", len(order))
		return
	}

	if order[0] != 1 || order[1] != 2 {
		t.Errorf("expected order [1, 2], got %v", order)
	}
}

func TestTimerCancel(t *testing.T) {
	var executed atomic.Int32
	handler := func(e *Entry) {
		e.Execute()
	}

	timer := NewTimer(handler)
	timer.Start()
	defer timer.Stop()

	// 添加一个任务并取消
	entry := timer.AddEntry(50*time.Millisecond, func() {
		executed.Add(1)
	})
	entry.Cancel()

	time.Sleep(100 * time.Millisecond)

	// 回调不应该执行因为entry被cancel了
	if executed.Load() != 0 {
		t.Errorf("expected 0 execution (canceled), got %d", executed.Load())
	}
}

func TestTimerConcurrentAdd(t *testing.T) {
	var executed atomic.Int64
	handler := func(e *Entry) {
		e.Execute()
		executed.Add(1)
	}

	timer := NewTimer(handler)
	timer.Start()
	defer timer.Stop()

	var wg sync.WaitGroup
	numGoroutines := 10
	tasksPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				timer.AddEntry(time.Duration(10+j)*time.Millisecond, func() {})
			}
		}()
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	expected := int64(numGoroutines * tasksPerGoroutine)
	if executed.Load() != expected {
		t.Errorf("expected %d executions, got %d", expected, executed.Load())
	}
}

// Benchmark tests

func BenchmarkWheelAddEntry(b *testing.B) {
	w := NewWheel(0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := NewEntry(time.Now().Add(time.Duration(i%64)*time.Millisecond), func() {})
		w.AddEntry(entry, uint64(i%64))
	}
}

func BenchmarkWheelMultiLevelAdd(b *testing.B) {
	w := NewWheel(2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := NewEntry(time.Now().Add(time.Duration(i%4096)*time.Millisecond), func() {})
		w.AddEntry(entry, uint64(i%4096))
	}
}

func BenchmarkTimerAdd(b *testing.B) {
	handler := func(e *Entry) {}

	timer := NewTimer(handler)
	timer.Start()
	defer timer.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer.AddEntry(time.Duration(100+i%1000)*time.Millisecond, func() {})
	}
}

func BenchmarkTimerAddParallel(b *testing.B) {
	handler := func(e *Entry) {}

	timer := NewTimer(handler)
	timer.Start()
	defer timer.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			timer.AddEntry(time.Duration(100+i%1000)*time.Millisecond, func() {})
			i++
		}
	})
}

func BenchmarkNextExpirationTime(b *testing.B) {
	w := NewWheel(2)

	// 添加一些任务
	for i := 0; i < 1000; i++ {
		entry := NewEntry(time.Now().Add(time.Duration(i)*time.Millisecond), func() {})
		w.AddEntry(entry, uint64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.NextExpirationTime()
	}
}

func BenchmarkHandleExpired(b *testing.B) {
	handler := func(e *Entry) {}

	// 预先创建entries
	entries := make([]*Entry, 64)
	for j := 0; j < 64; j++ {
		entries[j] = NewEntry(time.Now().Add(time.Duration(j)*time.Millisecond), func() {})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := NewWheel(0)
		for j := 0; j < 64; j++ {
			w.AddEntry(entries[j], uint64(j))
		}
		w.HandleExpiredEntries(handler, 64)
	}
}
