package whTimer

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// 哨兵值，表示next正在被设置
var settingNext = unsafe.Pointer(new(Entry))

// entryPool 对象池
var entryPool = sync.Pool{
	New: func() any {
		return &Entry{}
	},
}

// Entry 定时任务条目（同时作为队列节点）
type Entry struct {
	// 队列链接（热路径，放前面）
	next unsafe.Pointer // *Entry

	// 定时任务数据
	expireAt time.Time
	callback func()
	removed  atomic.Bool
}

// NewEntry 创建新的定时任务条目
func NewEntry(expireAt time.Time, callback func()) *Entry {
	e := entryPool.Get().(*Entry)
	e.expireAt = expireAt
	e.callback = callback
	e.next = settingNext // 标记正在设置
	e.removed.Store(false)
	return e
}

// Release 释放回对象池
func (e *Entry) Release() {
	e.callback = nil
	e.next = nil
	entryPool.Put(e)
}

// Execute 执行回调
func (e *Entry) Execute() {
	if !e.removed.Load() && e.callback != nil {
		e.callback()
	}
}

// Cancel 取消定时任务
func (e *Entry) Cancel() {
	e.removed.Store(true)
}

// IsCanceled 检查是否已取消
func (e *Entry) IsCanceled() bool {
	return e.removed.Load()
}

// MPSCQueue Wait-Free MPSC队列
type MPSCQueue struct {
	head unsafe.Pointer // *Entry
	_    [56]byte       // padding
}

// NewMPSCQueue 创建队列
func NewMPSCQueue() *MPSCQueue {
	return &MPSCQueue{}
}

// Push 添加元素 - Wait-Free O(1)
func (q *MPSCQueue) Push(entry *Entry) bool {
	oldHead := atomic.SwapPointer(&q.head, unsafe.Pointer(entry))
	atomic.StorePointer(&entry.next, oldHead)
	return oldHead == nil
}

// PopAll 取出所有元素 - Wait-Free
func (q *MPSCQueue) PopAll() *Entry {
	head := (*Entry)(atomic.SwapPointer(&q.head, nil))
	if head == nil {
		return nil
	}

	// 反转链表
	var prev *Entry
	curr := head

	for curr != nil {
		var next unsafe.Pointer
		for {
			next = atomic.LoadPointer(&curr.next)
			if next != settingNext {
				break
			}
		}

		atomic.StorePointer(&curr.next, unsafe.Pointer(prev))
		prev = curr
		curr = (*Entry)(next)
	}

	return prev
}

// DrainAll 取出并处理所有元素
func (q *MPSCQueue) DrainAll(fn func(*Entry)) int {
	head := q.PopAll()
	count := 0
	for head != nil {
		// 必须先保存next，因为fn可能修改head.next（如添加到wheel链表）
		next := (*Entry)(atomic.LoadPointer(&head.next))
		fn(head)
		head = next
		count++
	}
	return count
}

// IsEmpty 检查队列是否为空
func (q *MPSCQueue) IsEmpty() bool {
	return atomic.LoadPointer(&q.head) == nil
}
