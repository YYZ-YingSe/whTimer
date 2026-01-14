# whTimer

高性能分层时间轮定时器，专为海量定时任务场景设计。

## 特性

- **Wait-Free 添加**: 基于 MPSC 队列实现，添加任务无锁等待
- **分层时间轮**: 7 层 64 槽位设计，支持最长约 139 年的定时任务
- **Bitmap 加速**: 使用位图快速定位非空槽位
- **内存高效**: 单任务内存占用约为标准库的 1/8
- **稳定性能**: 大规模任务下性能恒定

## 安装

```bash
go get github.com/your/whTimer
```

## 快速开始

```go
package main

import (
    "fmt"
    "time"
    "whTimer"
)

func main() {
    // 创建定时器
    timer := whTimer.NewTimer(func(e *whTimer.Entry) {
        e.Execute()
    })
    timer.Start()
    defer timer.Stop()

    // 添加定时任务
    timer.AddEntry(100*time.Millisecond, func() {
        fmt.Println("Hello, Timer!")
    })

    time.Sleep(200 * time.Millisecond)
}
```

## 性能测试

### 测试环境

```
cpu: Apple M4 Pro
goos: darwin
goarch: arm64
```

### 全流程测试 (添加 + 执行)

测试场景：添加 N 条延迟任务，等待全部执行完成，计算平均每任务开销。

| 任务数 | 延迟 | whTimer | Stdlib | 性能比 |
|--------|------|---------|--------|--------|
| 100 | 1s | 3,938 ns/task | 11,888 ns/task | **3.0x** |
| 10K | 5s | 257 ns/task | 1,016 ns/task | **4.0x** |
| 1M | 30s | 125 ns/task | 338 ns/task | **2.7x** |
| 100M | 1min | **104 ns/task** | N/A | - |

> **注意**: 低量级任务（如 100、10K）的 ns/task 数值偏高，这是因为存在固定启动开销（Timer 初始化、Goroutine 调度、系统调用等），这些开销被平摊到较少的任务上。随着任务数量增加，固定开销被稀释，**100M 任务的 ~104 ns/task 最接近真实的单任务性能**。
>
> ```
> 平均耗时 = (固定开销 + N × 单任务开销) / N = 固定开销/N + 单任务开销
> ```

### 内存占用

| 任务数 | whTimer | Stdlib | 内存比 |
|--------|---------|--------|--------|
| 10K | 508 KB / 10,929 allocs | 5.4 MB / 26,917 allocs | **10.7x** |
| 1M | 47 MB / 1,066,093 allocs | 388 MB / 2,365,144 allocs | **8.2x** |

### 关键指标

| 指标 | whTimer | Stdlib |
|------|---------|--------|
| 单任务内存 | ~48 B | ~407 B |
| 单任务分配 | 1 alloc | 2+ allocs |
| 大规模性能 | 104 ns/task | 338+ ns/task |
| 1亿任务支持 | ✅ | ❌ (OOM) |

### Cron 周期任务性能 (vs robfig/cron)

#### 单次添加/删除

| | whTimer | robfig/cron | 性能比 |
|--|---------|-------------|--------|
| 耗时 | 172 ns/op | 1,843 ns/op | **10.7x** |
| 内存 | 186 B/op | 1,376 B/op | **7.4x** |
| 分配 | 5 allocs | 35 allocs | **7x** |

#### 批量创建内存占用

| 任务数 | whTimer | robfig/cron | 内存比 |
|--------|---------|-------------|--------|
| 1K | 203 KB | 11.4 MB | **56x** |
| 10K | 1.96 MB | 1.56 GB | **797x** |
| 100K | 19.6 MB | N/A (太慢) | - |

#### 批量创建耗时

| 任务数 | whTimer | robfig/cron | 时间比 |
|--------|---------|-------------|--------|
| 1K | 0.38 ms | 2.7 ms | **7x** |
| 10K | 3.5 ms | 378 ms | **108x** |
| 100K | 29 ms | N/A | - |

## 架构设计

### 分层时间轮

```
Level 6: 64 slots × 68,719,476,736 ms = ~139 年
Level 5: 64 slots × 1,073,741,824 ms  = ~34 年
Level 4: 64 slots × 16,777,216 ms     = ~194 天
Level 3: 64 slots × 262,144 ms        = ~4.7 小时
Level 2: 64 slots × 4,096 ms          = ~4.4 分钟
Level 1: 64 slots × 64 ms             = ~4 秒
Level 0: 64 slots × 1 ms              = 64 ms
```

### Wait-Free MPSC 队列

```
Producer 1 ──┐
Producer 2 ──┼──▶ [MPSC Queue] ──▶ Timer Goroutine
Producer N ──┘
```

- 多生产者可并发添加任务，无锁竞争
- 单消费者（Timer Goroutine）批量处理

## API

### Timer

```go
// 创建定时器
func NewTimer(handler func(*Entry)) *Timer

// 启动/停止
func (t *Timer) Start()
func (t *Timer) Stop()

// 添加任务
func (t *Timer) AddEntry(delay time.Duration, callback func()) *Entry
func (t *Timer) AddEntryAt(expireAt time.Time, callback func()) *Entry

// 待处理任务数
func (t *Timer) Pending() uint64
```

### Entry

```go
// 执行回调
func (e *Entry) Execute()

// 取消任务
func (e *Entry) Cancel()

// 检查是否已取消
func (e *Entry) IsCanceled() bool
```

### 延迟任务 (defer.go)

```go
// 延迟执行，等同于 time.AfterFunc
func (t *Timer) AfterFunc(d time.Duration, f func()) *Entry

// 返回 channel，等同于 time.After
func (t *Timer) After(d time.Duration) <-chan time.Time

// 阻塞等待，等同于 time.Sleep
func (t *Timer) Sleep(d time.Duration)
```

### 周期任务 (cron.go)

```go
// Cron 表达式创建周期任务
// 格式: "秒 分 时 日 月 星期"
func (t *Timer) Cron(expr string, callback func()) (*CronEntry, error)

// 指定时间执行一次
func (t *Timer) CronAt(at time.Time, callback func()) *CronEntry

// 固定间隔执行
func (t *Timer) CronInterval(interval time.Duration, callback func()) *CronEntry

// 停止周期任务
func (c *CronEntry) Stop()
```

## 适用场景

- 游戏服务器大量 NPC/技能定时器
- 网络连接超时管理
- 延迟任务队列
- 定时数据同步
- 任何需要海量定时任务的场景

## 运行测试

```bash
# 单元测试
go test -v

# 性能测试
go test -bench="FullCycle" -benchmem -benchtime=1x

# 竞态检测
go test -race
```

## License

MIT License