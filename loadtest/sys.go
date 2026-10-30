package main

import (
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Process and host measurements read from /proc, so no extra tools are needed. Linux only.

type procSample struct {
	At      time.Time
	CPUSecs float64 // user+system seconds used so far
	RSSMB   float64
	FDs     int
	Threads int
}

func readProc(pid string) (procSample, error) {
	var s procSample
	b, err := os.ReadFile("/proc/" + pid + "/stat")
	if err != nil {
		return s, err
	}
	str := string(b)
	i := strings.LastIndex(str, ")")
	if i < 0 {
		return s, fmt.Errorf("odd stat")
	}
	f := strings.Fields(str[i+2:])
	if len(f) < 20 {
		return s, fmt.Errorf("short stat")
	}
	ut, _ := strconv.ParseFloat(f[11], 64)
	st, _ := strconv.ParseFloat(f[12], 64)
	th, _ := strconv.Atoi(f[17])
	s.CPUSecs = (ut + st) / 100 // CLK_TCK is 100 on Linux
	s.Threads = th
	if sb, err := os.ReadFile("/proc/" + pid + "/status"); err == nil {
		for _, ln := range strings.Split(string(sb), "\n") {
			if strings.HasPrefix(ln, "VmRSS:") {
				kb, _ := strconv.ParseFloat(strings.Fields(ln)[1], 64)
				s.RSSMB = kb / 1024
			}
		}
	}
	if ents, err := os.ReadDir("/proc/" + pid + "/fd"); err == nil {
		s.FDs = len(ents)
	}
	s.At = time.Now()
	return s, nil
}

type envInfo struct {
	Go        string `json:"go"`
	OSArch    string `json:"os_arch"`
	CPUs      int    `json:"cpus"`
	GoMaxProc int    `json:"gomaxprocs"`
	CgroupCPU string `json:"cgroup_cpu_max"`
	CgroupMem string `json:"cgroup_memory_max"`
	MemTotal  string `json:"mem_total"`
	NoFile    uint64 `json:"client_nofile_limit"`
	DBVersion string `json:"postgres_version,omitempty"`
	DBMaxConn string `json:"postgres_max_connections,omitempty"`
	DBShared  string `json:"postgres_shared_buffers,omitempty"`
	Server    string `json:"server_process,omitempty"`
}

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return "n/a"
	}
	return strings.TrimSpace(string(b))
}

func collectEnv(db *sql.DB, serverPID string) envInfo {
	e := envInfo{Go: runtime.Version(), OSArch: runtime.GOOS + "/" + runtime.GOARCH, CPUs: runtime.NumCPU(), GoMaxProc: runtime.GOMAXPROCS(0),
		CgroupCPU: readTrim("/sys/fs/cgroup/cpu.max"), CgroupMem: readTrim("/sys/fs/cgroup/memory.max")}
	for _, ln := range strings.Split(readTrim("/proc/meminfo"), "\n") {
		if strings.HasPrefix(ln, "MemTotal:") {
			e.MemTotal = strings.TrimSpace(strings.TrimPrefix(ln, "MemTotal:"))
		}
	}
	var rl syscall.Rlimit
	if syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl) == nil {
		e.NoFile = rl.Cur
	}
	if db != nil {
		_ = db.QueryRow(`SELECT version()`).Scan(&e.DBVersion)
		_ = db.QueryRow(`SHOW max_connections`).Scan(&e.DBMaxConn)
		_ = db.QueryRow(`SHOW shared_buffers`).Scan(&e.DBShared)
	}
	if serverPID != "" {
		e.Server = "pid " + serverPID + " (" + readTrim("/proc/"+serverPID+"/comm") + ")"
	}
	return e
}

// raiseNoFile lifts the soft file limit to the hard limit: every connection is one file descriptor.
func raiseNoFile() uint64 {
	var rl syscall.Rlimit
	if syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl) != nil {
		return 0
	}
	rl.Cur = rl.Max
	_ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rl)
	_ = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl)
	return rl.Cur
}
