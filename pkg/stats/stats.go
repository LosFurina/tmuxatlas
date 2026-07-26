package stats

import (
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/LosFurina/tmuxatlas/pkg/tmux"
)

// SystemStats reads system info from /proc (Linux) or falls back to runtime info
func SystemStats() map[string]interface{} {
	stats := map[string]interface{}{
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"cpus":       runtime.NumCPU(),
		"goroutines": runtime.NumGoroutine(),
	}

	// Memory stats from Go runtime
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	stats["tmuxatlas_mem_mb"] = float64(m.Alloc) / 1024 / 1024

	// Load average from /proc/loadavg
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			load1, _ := strconv.ParseFloat(fields[0], 64)
			load5, _ := strconv.ParseFloat(fields[1], 64)
			load15, _ := strconv.ParseFloat(fields[2], 64)
			stats["load"] = map[string]float64{
				"1m": load1, "5m": load5, "15m": load15,
			}
		}
	}

	// Uptime from /proc/uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			uptime, _ := strconv.ParseFloat(fields[0], 64)
			stats["uptime_seconds"] = uptime
		}
	}

	// Memory info from /proc/meminfo
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		memInfo := make(map[string]int64)
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			valStr := strings.TrimSpace(parts[1])
			valStr = strings.TrimSuffix(valStr, " kB")
			val, _ := strconv.ParseInt(strings.TrimSpace(valStr), 10, 64)
			memInfo[key] = val
		}
		totalKB := memInfo["MemTotal"]
		availKB := memInfo["MemAvailable"]
		if totalKB > 0 {
			totalMB := float64(totalKB) / 1024
			availMB := float64(availKB) / 1024
			usedMB := totalMB - availMB
			stats["memory"] = map[string]interface{}{
				"total_mb":     int(totalMB),
				"used_mb":      int(usedMB),
				"available_mb": int(availMB),
				"percent":      int((usedMB / totalMB) * 100),
			}
		}
	}

	// CPU usage from /proc/stat (instantaneous snapshot — user+system percentage)
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				if len(fields) >= 8 {
					user, _ := strconv.ParseFloat(fields[1], 64)
					nice, _ := strconv.ParseFloat(fields[2], 64)
					system, _ := strconv.ParseFloat(fields[3], 64)
					idle, _ := strconv.ParseFloat(fields[4], 64)
					iowait, _ := strconv.ParseFloat(fields[5], 64)
					irq, _ := strconv.ParseFloat(fields[6], 64)
					softirq, _ := strconv.ParseFloat(fields[7], 64)
					total := user + nice + system + idle + iowait + irq + softirq
					if total > 0 {
						stats["cpu_percent"] = int(((total - idle - iowait) / total) * 100)
					}
				}
				break
			}
		}
	}

	return stats
}

// ProcessEntry represents a process name and its count
type ProcessEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ProcessCountsFromSessions counts pane commands from session data, sorted by count descending
func ProcessCountsFromSessions(sessions []*tmux.Session) []ProcessEntry {
	counts := make(map[string]int)
	for _, s := range sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.CurrentCommand != "" {
					counts[p.CurrentCommand]++
				}
			}
		}
	}

	entries := make([]ProcessEntry, 0, len(counts))
	for name, count := range counts {
		entries = append(entries, ProcessEntry{Name: name, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})
	return entries
}
