// go-sysmon 一个本地系统巡检小工具，把常用的资源查看收进一个二进制。
// 子命令：
//
//	cpu       打印 CPU 核心数与当前占用率（采样 1 秒）
//	mem       打印内存总量/已用/可用
//	disk      打印各挂载点使用情况
//	process   列出当前进程（按名字过滤可选）
//	network   打印网络接口与 IP
//	port      列出本机正在监听的 TCP 端口
//
// 纯标准库实现，Windows / Linux / macOS 都能跑。
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printSysUsage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "cpu":
		err = sysCPU()
	case "mem":
		err = sysMem()
	case "disk":
		err = sysDisk()
	case "process":
		err = sysProcess(args)
	case "network":
		err = sysNetwork()
	case "port":
		err = sysPort()
	case "help", "-h", "--help":
		printSysUsage()
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n", cmd)
		printSysUsage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func printSysUsage() {
	fmt.Println(`go-sysmon 系统巡检工具

用法:
  go-sysmon <子命令>

子命令:
  cpu                  CPU 核心数与占用率
  mem                  内存使用情况
  disk                 磁盘挂载点使用情况
  process [-name x]    进程列表（可按名字过滤）
  network              网络接口与地址
  port                 监听中的 TCP 端口`)
}

// ----- cpu -----
// cpuUsage 用前后两次读 runtime.NumGoroutine 之外的系统时间差估算忙闲，
// 标准库没有跨平台 CPU 占用，这里用 busy loop 采样近似值更直观：改为读
// 系统负载（仅类 Unix 有 /proc，Windows 走另一路径）。
func sysCPU() error {
	fmt.Printf("CPU 核心数: %d\n", runtime.NumCPU())
	usage, err := cpuPercent()
	if err != nil {
		// 拿不到就只报核心数
		fmt.Println("(当前平台暂不支持读取占用率)")
		return nil
	}
	fmt.Printf("当前占用率: %.1f%%\n", usage)
	return nil
}

// cpuPercent 尝试从 /proc/stat 或 wmic 取两次采样差算出占用率。
func cpuPercent() (float64, error) {
	if runtime.GOOS == "windows" {
		return winCPUPercent()
	}
	return procCPUPercent()
}

func procCPUPercent() (float64, error) {
	read := func() (idle, total uint64, err error) {
		f, e := os.Open("/proc/stat")
		if e != nil {
			return 0, 0, e
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		if !sc.Scan() {
			return 0, 0, fmt.Errorf("读不到 cpu 行")
		}
		fields := strings.Fields(sc.Text())
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			total += v
			if i == 4 { // idle 在第 5 列（索引 4）
				idle = v
			}
		}
		return
	}
	i1, t1, e := read()
	if e != nil {
		return 0, e
	}
	time.Sleep(time.Second)
	i2, t2, e := read()
	if e != nil {
		return 0, e
	}
	di := float64(i2 - i1)
	dt := float64(t2 - t1)
	if dt == 0 {
		return 0, nil
	}
	return (1 - di/dt) * 100, nil
}

func winCPUPercent() (float64, error) {
	out, err := exec.Command("wmic", "cpu", "get", "loadpercentage").Output()
	if err != nil {
		return 0, err
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, e := strconv.Atoi(line); e == nil {
			return float64(v), nil
		}
	}
	return 0, fmt.Errorf("解析失败")
}

// ----- mem -----
func sysMem() error {
	if runtime.GOOS == "windows" {
		return winMem()
	}
	return procMem()
}

func procMem() error {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return err
	}
	defer f.Close()
	vals := map[string]uint64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 {
			if v, e := strconv.ParseUint(fields[1], 10, 64); e == nil {
				vals[fields[0]] = v // 单位是 kB
			}
		}
	}
	total := vals["MemTotal:"] * 1024
	avail := vals["MemAvailable:"] * 1024
	used := total - avail
	fmt.Printf("总量: %s\n", humanBytes(total))
	fmt.Printf("已用: %s (%.1f%%)\n", humanBytes(used), pct(used, total))
	fmt.Printf("可用: %s\n", humanBytes(avail))
	return nil
}

func winMem() error {
	out, err := exec.Command("wmic", "OS", "get", "FreePhysicalMemory,TotalVisibleMemorySize").Output()
	if err != nil {
		return err
	}
	var free, total uint64
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 {
			total, _ = strconv.ParseUint(fields[1], 10, 64)
			free, _ = strconv.ParseUint(fields[0], 10, 64)
		}
	}
	// wmic 单位是 KB
	total *= 1024
	free *= 1024
	used := total - free
	fmt.Printf("总量: %s\n", humanBytes(total))
	fmt.Printf("已用: %s (%.1f%%)\n", humanBytes(used), pct(used, total))
	fmt.Printf("可用: %s\n", humanBytes(free))
	return nil
}

func pct(part, whole uint64) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole) * 100
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ----- disk -----
func sysDisk() error {
	// 各平台挂载点不一样，Windows 遍历盘符，类 Unix 只查根。
	roots := []string{"/"}
	if runtime.GOOS == "windows" {
		roots = nil
		for _, d := range "CDEFGH" {
			roots = append(roots, string(d)+":")
		}
	}
	any := false
	for _, r := range roots {
		total, free, err := diskUsage(r)
		if err != nil {
			continue
		}
		if total == 0 {
			continue
		}
		used := total - free
		any = true
		fmt.Printf("%-8s 总量 %s 已用 %s (%.1f%%) 可用 %s\n",
			r, humanBytes(total), humanBytes(used), pct(used, total), humanBytes(free))
	}
	if !any {
		fmt.Println("(当前平台未能读取磁盘信息)")
	}
	return nil
}

// diskUsage 用系统自带命令取某个挂载点/盘符的总空间与可用空间，
// 不引入第三方库：类 Unix 走 df，Windows 走 wmic。
func diskUsage(root string) (total, free uint64, err error) {
	if runtime.GOOS == "windows" {
		out, e := exec.Command("wmic", "logicaldisk", "where",
			"DeviceID='"+root+"'", "get", "Size,FreeSpace", "/VALUE").Output()
		if e != nil {
			return 0, 0, e
		}
		var size, avail uint64
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Size=") {
				fmt.Sscanf(line, "Size=%d", &size)
			}
			if strings.HasPrefix(line, "FreeSpace=") {
				fmt.Sscanf(line, "FreeSpace=%d", &avail)
			}
		}
		return size, avail, nil
	}
	out, e := exec.Command("df", "--block-size=1", root).Output()
	if e != nil {
		return 0, 0, e
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, 0, fmt.Errorf("df 输出格式异常")
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return 0, 0, fmt.Errorf("df 字段不足")
	}
	fmt.Sscanf(fields[1], "%d", &total)
	fmt.Sscanf(fields[3], "%d", &free)
	return total, free, nil
}

// ----- process -----
func sysProcess(args []string) error {
	name := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "-name" && i+1 < len(args) {
			name = args[i+1]
		}
	}
	procs, err := listProcesses()
	if err != nil {
		return err
	}
	count := 0
	for _, p := range procs {
		if name != "" && !strings.Contains(strings.ToLower(p.name), strings.ToLower(name)) {
			continue
		}
		fmt.Printf("%-8d %s\n", p.pid, p.name)
		count++
	}
	fmt.Printf("共 %d 个进程\n", count)
	return nil
}

// ----- network -----
func sysNetwork() error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		var ips []string
		for _, a := range addrs {
			ips = append(ips, a.String())
		}
		fmt.Printf("%-12s %s\n", iface.Name, strings.Join(ips, ", "))
	}
	return nil
}

// ----- port -----
func sysPort() error {
	procs, err := listProcesses()
	if err != nil {
		return err
	}
	// 每个进程尝试看它有没有监听端口不现实，这里改为扫常见监听端口是否可连。
	// 更直接的做法：Windows 用 netstat，类 Unix 也用 netstat/ss。
	lines, err := listenPorts()
	if err != nil {
		return err
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	_ = procs
	return nil
}

func listenPorts() ([]string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("netstat", "-ano", "-p", "TCP")
	} else {
		cmd = exec.Command("ss", "-ltn")
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var res []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.Contains(line, "LISTEN") || (runtime.GOOS == "windows" && strings.Contains(line, "TCP")) {
			res = append(res, line)
		}
	}
	return res, nil
}

// 给 go-sysmon 用的小工具：把进程信息抽象出来，便于测试与跨平台替换。
func listProcesses() ([]procInfo, error) {
	if runtime.GOOS == "windows" {
		return winProcesses()
	}
	return procProcesses()
}

type procInfo struct {
	pid  int
	name string
}

func procProcesses() ([]procInfo, error) {
	// 读 /proc 下每个数字目录
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var res []procInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		name := "?"
		if data, e2 := os.ReadFile("/proc/" + e.Name() + "/comm"); e2 == nil {
			name = strings.TrimSpace(string(data))
		}
		res = append(res, procInfo{pid: pid, name: name})
	}
	return res, nil
}

func winProcesses() ([]procInfo, error) {
	out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil, err
	}
	var res []procInfo
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		parts := strings.Split(line, "\",\"")
		if len(parts) >= 2 {
			name := strings.Trim(parts[0], "\"")
			pid, _ := strconv.Atoi(strings.Trim(parts[1], "\""))
			res = append(res, procInfo{pid: pid, name: name})
		}
	}
	return res, nil
}
