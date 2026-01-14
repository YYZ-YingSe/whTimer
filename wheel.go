package whTimer

import (
	"math/bits"
	"sync/atomic"
	"unsafe"
)

// 编译期常量，避免运行时计算
const (
	SlotBits = 6             // 2^6 = 64 slots per wheel
	SlotSize = 1 << SlotBits // 64 slots
	SlotMask = SlotSize - 1  // 0x3F
	MaxLevel = 6             // 支持到level 6，约~139年
)

// 编译期计算的常量表
var (
	msPerSlot = [MaxLevel + 1]uint64{
		1, 64, 4096, 262144, 16777216, 1073741824, 68719476736,
	}

	maxMs = [MaxLevel + 1]uint64{
		64, 4096, 262144, 16777216, 1073741824, 68719476736, 4398046511104,
	}

	shift = [MaxLevel + 1]uint{0, 6, 12, 18, 24, 30, 36}

	mask = [MaxLevel + 1]uint64{
		0x3F, 0x3F << 6, 0x3F << 12, 0x3F << 18, 0x3F << 24, 0x3F << 30, 0x3F << 36,
	}
)

// Wheel 时间轮
type Wheel struct {
	level     int
	bitmap    uint64
	entries   [SlotSize]*Entry
	subWheels [SlotSize]*Wheel
}

// NewWheel 创建新的时间轮
func NewWheel(level int) *Wheel {
	return &Wheel{level: level}
}

// NewWheelWithChild 从子轮创建父轮
func NewWheelWithChild(child *Wheel) *Wheel {
	w := &Wheel{level: child.level + 1}
	w.bitmap = 1
	w.subWheels[0] = child
	return w
}

// getNext 获取entry的next指针
func getNext(e *Entry) *Entry {
	return (*Entry)(atomic.LoadPointer(&e.next))
}

// setNext 设置entry的next指针
func setNext(e *Entry, next *Entry) {
	atomic.StorePointer(&e.next, unsafe.Pointer(next))
}

// AddEntry 添加定时任务
func (w *Wheel) AddEntry(entry *Entry, interval uint64) {
	index := w.getIndex(interval)

	if w.level == 0 {
		setNext(entry, w.entries[index])
		w.bitmap |= 1 << index
		w.entries[index] = entry
	} else {
		if w.subWheels[index] == nil {
			w.bitmap |= 1 << index
			w.subWheels[index] = NewWheel(w.level - 1)
		}
		w.subWheels[index].AddEntry(entry, interval)
	}
}

// RemoveEntry 移除定时任务
func (w *Wheel) RemoveEntry(entry *Entry, interval uint64) {
	index := w.getIndex(interval)

	if w.level == 0 {
		head := w.entries[index]
		if head == entry {
			w.entries[index] = getNext(head)
			if w.entries[index] == nil {
				w.bitmap &^= 1 << index
			}
		} else {
			cur := head
			for getNext(cur) != entry {
				cur = getNext(cur)
			}
			setNext(cur, getNext(entry))
		}
	} else {
		child := w.subWheels[index]
		child.RemoveEntry(entry, interval)
		if child.Empty() {
			w.bitmap &^= 1 << index
			w.subWheels[index] = nil
		}
	}
}

// HandleExpiredEntries 处理过期的定时任务
func (w *Wheel) HandleExpiredEntries(handler func(*Entry), remainingMs uint64) int {
	count := 0

	for w.bitmap != 0 {
		index := uint64(bits.TrailingZeros64(w.bitmap))

		if w.level == 0 {
			if index > remainingMs {
				break
			}
			for w.entries[index] != nil {
				entry := w.entries[index]
				w.entries[index] = getNext(entry)
				handler(entry)
				count++
			}
			w.bitmap &^= 1 << index
		} else {
			slotMs := index * msPerSlot[w.level]
			if slotMs > remainingMs {
				break
			}
			child := w.subWheels[index]
			count += child.HandleExpiredEntries(handler, remainingMs-slotMs)
			if child.Empty() {
				w.subWheels[index] = nil
				w.bitmap &^= 1 << index
			} else {
				break
			}
		}
	}

	return count
}

// NextExpirationTime 获取下一个过期时间
func (w *Wheel) NextExpirationTime() uint64 {
	if w.Empty() {
		return ^uint64(0)
	}

	index := uint64(bits.TrailingZeros64(w.bitmap))

	if w.level == 0 {
		return index
	}
	return index*msPerSlot[w.level] + w.subWheels[index].NextExpirationTime()
}

// Rotate 推进时间轮
func (w *Wheel) Rotate(n uint64) {
	if n == 0 || n >= SlotSize {
		return
	}

	if w.level == 0 {
		for i := n; i < SlotSize; i++ {
			w.entries[i-n] = w.entries[i]
			w.entries[i] = nil
		}
	} else {
		for i := n; i < SlotSize; i++ {
			w.subWheels[i-n] = w.subWheels[i]
			w.subWheels[i] = nil
		}
	}
	w.bitmap >>= n
}

// LevelUp 升级到更高层级
func (w *Wheel) LevelUp() *Wheel {
	return NewWheelWithChild(w)
}

// LevelDown 降级到更低层级
func (w *Wheel) LevelDown() *Wheel {
	if w.level == 0 {
		return nil
	}
	return w.subWheels[0]
}

// CanLevelDown 检查是否可以降级
func (w *Wheel) CanLevelDown() bool {
	return w.bitmap == 1 && w.level > 0
}

// Empty 检查是否为空
func (w *Wheel) Empty() bool {
	return w.bitmap == 0
}

// Level 获取当前层级
func (w *Wheel) Level() int {
	return w.level
}

// MsPerSlot 获取每个槽位的毫秒数
func (w *Wheel) MsPerSlot() uint64 {
	return msPerSlot[w.level]
}

// MaxMs 获取最大支持的毫秒数
func (w *Wheel) MaxMs() uint64 {
	return maxMs[w.level]
}

func (w *Wheel) getIndex(interval uint64) uint64 {
	if w.level == 0 {
		return interval & SlotMask
	}
	return (interval & mask[w.level]) >> shift[w.level]
}
