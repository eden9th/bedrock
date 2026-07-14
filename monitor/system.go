package monitor

import (
	"runtime"
	"syscall"
)

// ─── RuntimeStats（纯 stdlib）────────────────────────────────────────────────

// RuntimeStats 提供 Go 运行时指标采集：内存、GC、Goroutine 数量。
// 基于 runtime 包，零外部依赖。
//
// 使用方式：
//
//	rs := monitor.NewRuntimeStats()
//	rs.Register(r)
//	// 定时采集
//	go func() { for range time.NewTicker(10*time.Second).C { rs.Collect() } }()
type RuntimeStats struct {
	gaugeHeapAlloc  *Gauge
	gaugeHeapSys    *Gauge
	gaugeHeapIdle   *Gauge
	gaugeHeapInuse  *Gauge
	gaugeStackInuse *Gauge
	counterGCCount  *Counter
	gaugeGCPauseNs  *Gauge
	gaugeGoroutines *Gauge
	gaugeNumCPU     *Gauge

	// lastNumGC 记录上次采集的 NumGC 值，用于计算增量。
	// runtime.ReadMemStats.NumGC 是累计绝对值，Counter.Add 需要增量。
	lastNumGC uint32
}

// NewRuntimeStats 创建运行时统计采集器。
func NewRuntimeStats() *RuntimeStats {
	return &RuntimeStats{}
}

// Register 将运行时指标注册到指定 Registry。
func (rs *RuntimeStats) Register(r *Registry) {
	rs.gaugeHeapAlloc = NewGauge(
		"go_mem_heap_alloc_bytes",
		"Bytes of allocated heap objects",
		nil,
	)
	rs.gaugeHeapSys = NewGauge(
		"go_mem_heap_sys_bytes",
		"Bytes of heap memory obtained from the OS",
		nil,
	)
	rs.gaugeHeapIdle = NewGauge(
		"go_mem_heap_idle_bytes",
		"Bytes in idle (unused) spans",
		nil,
	)
	rs.gaugeHeapInuse = NewGauge(
		"go_mem_heap_inuse_bytes",
		"Bytes in in-use spans",
		nil,
	)
	rs.gaugeStackInuse = NewGauge(
		"go_mem_stack_inuse_bytes",
		"Bytes in stack spans",
		nil,
	)
	rs.counterGCCount = NewCounter(
		"go_gc_count_total",
		"Total number of completed GC cycles",
		nil,
	)
	rs.gaugeGCPauseNs = NewGauge(
		"go_gc_pause_ns",
		"Most recent GC STW pause duration in nanoseconds",
		nil,
	)
	rs.gaugeGoroutines = NewGauge(
		"go_goroutine_count",
		"Current number of goroutines",
		nil,
	)
	rs.gaugeNumCPU = NewGauge(
		"go_num_cpu",
		"Number of logical CPUs available to the process",
		nil,
	)

	r.MustRegister(
		rs.gaugeHeapAlloc, rs.gaugeHeapSys, rs.gaugeHeapIdle, rs.gaugeHeapInuse,
		rs.gaugeStackInuse, rs.counterGCCount, rs.gaugeGCPauseNs,
		rs.gaugeGoroutines, rs.gaugeNumCPU,
	)
}

// Collect 从 runtime.ReadMemStats 采集 Go 运行时指标。
// 调用方负责按周期调用。
func (rs *RuntimeStats) Collect() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if rs.gaugeHeapAlloc != nil {
		rs.gaugeHeapAlloc.Set(float64(m.HeapAlloc))
	}
	if rs.gaugeHeapSys != nil {
		rs.gaugeHeapSys.Set(float64(m.HeapSys))
	}
	if rs.gaugeHeapIdle != nil {
		rs.gaugeHeapIdle.Set(float64(m.HeapIdle))
	}
	if rs.gaugeHeapInuse != nil {
		rs.gaugeHeapInuse.Set(float64(m.HeapInuse))
	}
	if rs.gaugeStackInuse != nil {
		rs.gaugeStackInuse.Set(float64(m.StackInuse))
	}
	if rs.counterGCCount != nil {
		// NumGC 是累计值，计算相对上次采集的增量
		if rs.lastNumGC == 0 {
			// 首次采集：记录基准值，不累加（避免把历史 GC 全部计入）
			rs.lastNumGC = m.NumGC
		} else if m.NumGC > rs.lastNumGC {
			delta := float64(m.NumGC - rs.lastNumGC)
			rs.counterGCCount.Add(delta)
			rs.lastNumGC = m.NumGC
		}
		// m.NumGC < lastNumGC: 进程重启或溢出回绕，重置基准
		if m.NumGC < rs.lastNumGC {
			rs.lastNumGC = m.NumGC
		}
	}
	if rs.gaugeGCPauseNs != nil {
		// 取最近一次 GC 暂停时间（环形缓冲，索引为 (NumGC+255)%256）
		rs.gaugeGCPauseNs.Set(float64(m.PauseNs[(m.NumGC+255)%256]))
	}
	if rs.gaugeGoroutines != nil {
		rs.gaugeGoroutines.Set(float64(runtime.NumGoroutine()))
	}
	if rs.gaugeNumCPU != nil {
		// NumCPU 是静态值，但仍作为 gauge 暴露以便 Dashboard 引用
		rs.gaugeNumCPU.Set(float64(runtime.NumCPU()))
	}
}

// ─── DiskStats（纯 stdlib，unix only）────────────────────────────────────────

// DiskStats 提供磁盘使用量采集，基于 syscall.Statfs。
// 仅在 Unix 系统上可用（Linux / macOS）。
type DiskStats struct {
	path            string
	gaugeDiskTotal  *Gauge
	gaugeDiskFree   *Gauge
	gaugeDiskUsed   *Gauge
	gaugeDiskUsePct *Gauge
}

// NewDiskStats 创建磁盘统计采集器。
// path 是要监控的路径（如 "/" 或 "/data"），空字符串默认 "/"。
func NewDiskStats(path string) *DiskStats {
	if path == "" {
		path = "/"
	}
	return &DiskStats{path: path}
}

// Register 将磁盘指标注册到指定 Registry。
func (ds *DiskStats) Register(r *Registry) {
	ds.gaugeDiskTotal = NewGauge(
		"system_disk_total_bytes",
		"Total disk space in bytes",
		[]string{"path"},
	)
	ds.gaugeDiskFree = NewGauge(
		"system_disk_free_bytes",
		"Free disk space in bytes",
		[]string{"path"},
	)
	ds.gaugeDiskUsed = NewGauge(
		"system_disk_used_bytes",
		"Used disk space in bytes",
		[]string{"path"},
	)
	ds.gaugeDiskUsePct = NewGauge(
		"system_disk_usage_percent",
		"Disk usage percent (0-100)",
		[]string{"path"},
	)

	r.MustRegister(ds.gaugeDiskTotal, ds.gaugeDiskFree, ds.gaugeDiskUsed, ds.gaugeDiskUsePct)
}

// Collect 执行一次磁盘使用量采集。
// 基于 syscall.Statfs，在 Unix 上可用。
func (ds *DiskStats) Collect() {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(ds.path, &stat); err != nil {
		return
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free
	var usePct float64
	if total > 0 {
		usePct = float64(used) / float64(total) * 100
	}

	if ds.gaugeDiskTotal != nil {
		ds.gaugeDiskTotal.Set(float64(total), ds.path)
	}
	if ds.gaugeDiskFree != nil {
		ds.gaugeDiskFree.Set(float64(free), ds.path)
	}
	if ds.gaugeDiskUsed != nil {
		ds.gaugeDiskUsed.Set(float64(used), ds.path)
	}
	if ds.gaugeDiskUsePct != nil {
		ds.gaugeDiskUsePct.Set(usePct, ds.path)
	}
}

// ─── 注意事项 ────────────────────────────────────────────────────────────────

// OS 级 CPU 使用率和物理内存使用率不在本包的 stdlib 实现范围内。
// 如需这些指标，推荐引入 github.com/shirou/gopsutil/v3，在消费方代码中实现：
//
//	import "github.com/shirou/gopsutil/v3/cpu"
//	cpuPercent, _ := cpu.Percent(0, false)
//	cpuGauge.Set(cpuPercent[0])
//
// 或者在 Linux 上直接读取 /proc/stat，macOS 上使用 sysctl。
