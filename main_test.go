package main

import "testing"

func TestHumanBytes(t *testing.T) {
	if humanBytes(512) != "512 B" {
		t.Fatalf("512 应为 512 B，实际 %s", humanBytes(512))
	}
	if humanBytes(1024) != "1.0 KiB" {
		t.Fatalf("1024 应为 1.0 KiB，实际 %s", humanBytes(1024))
	}
	if humanBytes(1024*1024*1024) != "1.0 GiB" {
		t.Fatalf("1GiB 解析异常: %s", humanBytes(1024*1024*1024))
	}
}

func TestPct(t *testing.T) {
	if pct(50, 100) != 50 {
		t.Fatalf("50/100 应为 50%%，实际 %v", pct(50, 100))
	}
	if pct(1, 0) != 0 {
		t.Fatal("除零应返回 0")
	}
}

func TestListProcessesDoesNotError(t *testing.T) {
	// 至少能列到当前进程，不报错即可
	procs, err := listProcesses()
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) == 0 {
		t.Fatal("应当至少列出一个进程")
	}
}
