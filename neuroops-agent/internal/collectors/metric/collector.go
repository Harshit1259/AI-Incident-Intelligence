/*
 * NeurOps Agent — Metric Collector
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Collects host system metrics (CPU, memory, disk, network, processes,
 * services, system info) using gopsutil and publishes them as structured
 * events to the NeurOps product.
 *
 * Each metric record follows the canonical NeurOps event envelope:
 *   {
 *     "event.type":      "metric",
 *     "object.type":     "Linux" | "Windows" | ...,
 *     "object.ip":       "<host IP>",
 *     "agent.id":        "<uuid>",
 *     "timestamp":       <unix seconds>,
 *     "metrics":         { <metric.name>: <value>, ... }
 *   }
 */

package metric

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/neuroops/agent/internal/config"
	"github.com/neuroops/agent/internal/logger"
)

// Publisher is the minimal interface the metric collector needs to ship data.
type Publisher interface {
	Publish(event map[string]any) bool
}

// Collector orchestrates all host metric collectors.
type Collector struct {
	cfg     *config.MetricAgentConfig
	agentCfg *config.AgentConfig
	pub     Publisher
	log     *logger.Logger
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// Cached previous network counters for delta calculation
	prevNetStats map[string]psnet.IOCountersStat
	prevNetTime  time.Time
	prevNetMu    sync.Mutex
}

// New creates a metric Collector. Call Start() to begin polling.
func New(cfg *config.MetricAgentConfig, agentCfg *config.AgentConfig, pub Publisher, log *logger.Logger) *Collector {
	return &Collector{
		cfg:          cfg,
		agentCfg:     agentCfg,
		pub:          pub,
		log:          log,
		stopCh:       make(chan struct{}),
		prevNetStats: make(map[string]psnet.IOCountersStat),
	}
}

// Start launches all polling goroutines.
func (c *Collector) Start() error {
	c.log.Info("Metric collector starting")

	if c.cfg.CPUMemoryEnabled {
		c.wg.Add(1)
		go c.poll("cpu-memory", time.Duration(c.cfg.CPUMemoryPollSeconds)*time.Second, c.collectCPUMemory)
	}
	if c.cfg.DiskEnabled {
		c.wg.Add(1)
		go c.poll("disk", time.Duration(c.cfg.DiskPollSeconds)*time.Second, c.collectDisk)
	}
	if c.cfg.NetworkEnabled {
		c.wg.Add(1)
		go c.poll("network", time.Duration(c.cfg.NetworkPollSeconds)*time.Second, c.collectNetwork)
	}
	if c.cfg.ProcessEnabled {
		c.wg.Add(1)
		go c.poll("process", time.Duration(c.cfg.ProcessPollSeconds)*time.Second, c.collectProcesses)
	}
	if c.cfg.SystemInfoEnabled {
		c.wg.Add(1)
		go c.poll("system-info", time.Duration(c.cfg.SystemInfoPollSeconds)*time.Second, c.collectSystemInfo)
	}
	if c.cfg.SystemLoadEnabled {
		c.wg.Add(1)
		go c.poll("system-load", time.Duration(c.cfg.SystemLoadPollSeconds)*time.Second, c.collectSystemLoad)
	}

	c.log.Info("Metric collector started")
	return nil
}

// Stop gracefully stops all polling goroutines.
func (c *Collector) Stop() {
	close(c.stopCh)
	c.wg.Wait()
	c.log.Info("Metric collector stopped")
}

// ── Polling loop ──────────────────────────────────────────────────────────────

func (c *Collector) poll(name string, interval time.Duration, fn func()) {
	defer c.wg.Done()
	c.log.Infof("Starting %s poller (interval: %v)", name, interval)

	// Collect immediately on start, then on the tick.
	fn()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			c.log.Infof("%s poller stopped", name)
			return
		case <-ticker.C:
			fn()
		}
	}
}

// ── CPU & Memory ──────────────────────────────────────────────────────────────

func (c *Collector) collectCPUMemory() {
	ctx := context.Background()
	metrics := make(map[string]any)

	// CPU
	percents, err := cpu.PercentWithContext(ctx, 0, false)
	if err == nil && len(percents) > 0 {
		metrics["system.cpu.used.percent"] = round2(percents[0])
		metrics["system.cpu.idle.percent"] = round2(100.0 - percents[0])
	}

	// Per-core CPU
	corePercents, err := cpu.PercentWithContext(ctx, 0, true)
	if err == nil {
		for i, p := range corePercents {
			metrics[fmt.Sprintf("system.cpu.core.%d.used.percent", i)] = round2(p)
		}
	}

	// CPU count
	cpuCount, _ := cpu.CountsWithContext(ctx, true)
	metrics["system.cpu.core.count"] = cpuCount

	// Memory
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err == nil {
		metrics["system.memory.total.bytes"] = vm.Total
		metrics["system.memory.used.bytes"] = vm.Used
		metrics["system.memory.available.bytes"] = vm.Available
		metrics["system.memory.used.percent"] = round2(vm.UsedPercent)
		metrics["system.memory.free.bytes"] = vm.Free
		metrics["system.memory.cached.bytes"] = vm.Cached
		metrics["system.memory.buffers.bytes"] = vm.Buffers
	}

	// Swap
	swap, err := mem.SwapMemoryWithContext(ctx)
	if err == nil {
		metrics["system.swap.total.bytes"] = swap.Total
		metrics["system.swap.used.bytes"] = swap.Used
		metrics["system.swap.free.bytes"] = swap.Free
		metrics["system.swap.used.percent"] = round2(swap.UsedPercent)
	}

	c.publish("cpu_memory", metrics)
}

// ── Disk ──────────────────────────────────────────────────────────────────────

func (c *Collector) collectDisk() {
	ctx := context.Background()

	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		c.log.Errorf("Failed to list disk partitions: %v", err)
		return
	}

	var records []map[string]any

	for _, p := range partitions {
		if !c.shouldCollect(p.Mountpoint, c.cfg.MonitoredDisks) {
			continue
		}

		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			c.log.Warnf("Disk usage error for %s: %v", p.Mountpoint, err)
			continue
		}

		io, _ := disk.IOCountersWithContext(ctx, p.Device)
		ioStat := disk.IOCountersStat{}
		if io != nil {
			if s, ok := io[p.Device]; ok {
				ioStat = s
			}
		}

		rec := map[string]any{
			"disk.mount":           p.Mountpoint,
			"disk.device":          p.Device,
			"disk.fs.type":         p.Fstype,
			"disk.total.bytes":     usage.Total,
			"disk.used.bytes":      usage.Used,
			"disk.free.bytes":      usage.Free,
			"disk.used.percent":    round2(usage.UsedPercent),
			"disk.inode.used":      usage.InodesUsed,
			"disk.inode.free":      usage.InodesFree,
			"disk.inode.total":     usage.InodesTotal,
			"disk.read.bytes":      ioStat.ReadBytes,
			"disk.write.bytes":     ioStat.WriteBytes,
			"disk.read.ops":        ioStat.ReadCount,
			"disk.write.ops":       ioStat.WriteCount,
			"disk.read.time.ms":    ioStat.ReadTime,
			"disk.write.time.ms":   ioStat.WriteTime,
		}
		records = append(records, rec)
	}

	c.publishSlice("disk", records)
}

// ── Network ───────────────────────────────────────────────────────────────────

func (c *Collector) collectNetwork() {
	ctx := context.Background()

	counters, err := psnet.IOCountersWithContext(ctx, true)
	if err != nil {
		c.log.Errorf("Failed to collect network counters: %v", err)
		return
	}

	now := time.Now()

	c.prevNetMu.Lock()
	prev := c.prevNetStats
	prevTime := c.prevNetTime
	c.prevNetMu.Unlock()

	elapsed := now.Sub(prevTime).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}

	var records []map[string]any
	newStats := make(map[string]psnet.IOCountersStat)

	interfaces, _ := psnet.InterfacesWithContext(ctx)
	ifMap := make(map[string]psnet.InterfaceStat)
	for _, i := range interfaces {
		ifMap[i.Name] = i
	}

	for _, c2 := range counters {
		if !c.shouldCollect(c2.Name, c.cfg.MonitoredInterfaces) {
			continue
		}

		newStats[c2.Name] = c2
		rec := map[string]any{
			"interface.name":                   c2.Name,
			"interface.bytes.sent.total":        c2.BytesSent,
			"interface.bytes.recv.total":        c2.BytesRecv,
			"interface.packets.sent.total":      c2.PacketsSent,
			"interface.packets.recv.total":      c2.PacketsRecv,
			"interface.errors.in.total":         c2.Errin,
			"interface.errors.out.total":        c2.Errout,
			"interface.drop.in.total":           c2.Dropin,
			"interface.drop.out.total":          c2.Dropout,
		}

		// Calculate rates from deltas
		if p, ok := prev[c2.Name]; ok {
			rec["interface.bytes.sent.per.sec"] = round2(float64(c2.BytesSent-p.BytesSent) / elapsed)
			rec["interface.bytes.recv.per.sec"] = round2(float64(c2.BytesRecv-p.BytesRecv) / elapsed)
			rec["interface.packets.sent.per.sec"] = round2(float64(c2.PacketsSent-p.PacketsSent) / elapsed)
			rec["interface.packets.recv.per.sec"] = round2(float64(c2.PacketsRecv-p.PacketsRecv) / elapsed)
		}

		// Attach interface attributes
		if iface, ok := ifMap[c2.Name]; ok {
			rec["interface.mtu"] = iface.MTU
			rec["interface.flags"] = iface.Flags
			if len(iface.HardwareAddr) > 0 {
				rec["interface.mac.address"] = iface.HardwareAddr
			}
			for _, addr := range iface.Addrs {
				rec["interface.ip.address"] = addr.Addr
				break
			}
		}

		records = append(records, rec)
	}

	c.prevNetMu.Lock()
	c.prevNetStats = newStats
	c.prevNetTime = now
	c.prevNetMu.Unlock()

	c.publishSlice("network_interface", records)
}

// ── Processes ─────────────────────────────────────────────────────────────────

func (c *Collector) collectProcesses() {
	ctx := context.Background()

	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		c.log.Errorf("Failed to list processes: %v", err)
		return
	}

	var records []map[string]any

	for _, p := range procs {
		name, _ := p.NameWithContext(ctx)

		if !c.shouldCollect(name, c.cfg.MonitoredProcesses) {
			continue
		}

		cpuPct, _ := p.CPUPercentWithContext(ctx)
		memInfo, _ := p.MemoryInfoWithContext(ctx)
		memPct, _ := p.MemoryPercentWithContext(ctx)
		status, _ := p.StatusWithContext(ctx)
		cmdline, _ := p.CmdlineWithContext(ctx)
		username, _ := p.UsernameWithContext(ctx)
		createTime, _ := p.CreateTimeWithContext(ctx)
		numThreads, _ := p.NumThreadsWithContext(ctx)

		rec := map[string]any{
			"system.process.id":                 p.Pid,
			"system.process.name":               name,
			"system.process.status":             statusString(status),
			"system.process.cpu.percent":        round2(cpuPct),
			"system.process.memory.used.percent": round2(float64(memPct)),
			"system.process.threads":            numThreads,
			"system.process.user":               username,
			"system.process.command":            cmdline,
			"system.process.started.time.ms":    createTime,
		}

		if memInfo != nil {
			rec["system.process.memory.rss.bytes"] = memInfo.RSS
			rec["system.process.memory.vms.bytes"] = memInfo.VMS
		}

		// Network connections for this process
		if c.cfg.ProcessConnEnabled {
			conns, err := p.ConnectionsWithContext(ctx)
			if err == nil && len(conns) > 0 {
				rec["system.process.connection.count"] = len(conns)
			}
		}

		records = append(records, rec)
	}

	c.publishSlice("process", records)
}

// ── System Info ───────────────────────────────────────────────────────────────

func (c *Collector) collectSystemInfo() {
	ctx := context.Background()

	info, err := host.InfoWithContext(ctx)
	if err != nil {
		c.log.Errorf("Failed to collect system info: %v", err)
		return
	}

	hostname, _ := os.Hostname()
	localIP := c.localIP()

	metrics := map[string]any{
		"system.hostname":          hostname,
		"system.ip.address":        localIP,
		"system.os":                info.OS,
		"system.platform":          info.Platform,
		"system.platform.version":  info.PlatformVersion,
		"system.kernel.version":    info.KernelVersion,
		"system.architecture":      info.KernelArch,
		"system.boot.time":         info.BootTime,
		"system.uptime.seconds":    info.Uptime,
		"system.cpu.core.count":    info.Procs,
		"system.virtualization":    info.VirtualizationSystem,
		"system.virtualization.role": info.VirtualizationRole,
		"system.go.version":        runtime.Version(),
	}

	c.publish("system_info", metrics)
}

// ── System Load ───────────────────────────────────────────────────────────────

func (c *Collector) collectSystemLoad() {
	avg, err := load.Avg()
	if err != nil {
		c.log.Errorf("Failed to collect load average: %v", err)
		return
	}

	misc, _ := load.Misc()

	metrics := map[string]any{
		"system.load.1":           round2(avg.Load1),
		"system.load.5":           round2(avg.Load5),
		"system.load.15":          round2(avg.Load15),
	}
	if misc != nil {
		metrics["system.process.running"] = misc.ProcsRunning
		metrics["system.process.blocked"] = misc.ProcsBlocked
		metrics["system.process.total"]   = misc.ProcsTotal
	}

	c.publish("system_load", metrics)
}

// ── Event building ────────────────────────────────────────────────────────────

func (c *Collector) publish(metricType string, metrics map[string]any) {
	event := c.envelope(metricType)
	for k, v := range metrics {
		event[k] = v
	}
	if !c.pub.Publish(event) {
		c.log.Warnf("Failed to publish %s metrics", metricType)
	}
}

func (c *Collector) publishSlice(metricType string, records []map[string]any) {
	if len(records) == 0 {
		return
	}
	for _, rec := range records {
		event := c.envelope(metricType)
		for k, v := range rec {
			event[k] = v
		}
		c.pub.Publish(event)
	}
}

func (c *Collector) envelope(metricType string) map[string]any {
	return map[string]any{
		"event.type":   "metric",
		"metric.type":  metricType,
		"object.type":  c.objectType(),
		"object.ip":    c.localIP(),
		"agent.id":     c.agentCfg.AgentID,
		"timestamp":    time.Now().Unix(),
	}
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func (c *Collector) objectType() string {
	switch runtime.GOOS {
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	default:
		return runtime.GOOS
	}
}

func (c *Collector) localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}

// shouldCollect returns true if name is in the whitelist (or whitelist is empty).
func (c *Collector) shouldCollect(name string, whitelist []string) bool {
	if len(whitelist) == 0 {
		return true
	}
	for _, w := range whitelist {
		if w == name {
			return true
		}
	}
	return false
}

func round2(f float64) float64 {
	return float64(int(f*100)) / 100
}

func statusString(s []string) string {
	if len(s) == 0 {
		return "unknown"
	}
	return s[0]
}

// psnet alias to avoid import collision with stdlib net
var _ = psnet.IOCountersStat{}
