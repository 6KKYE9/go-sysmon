# go-sysmon

本地系统巡检小工具，把常用的资源查看收进一个二进制。Windows / Linux / macOS 都能跑。

## 安装

```bash
go build -o go-sysmon.exe
```

## 子命令

### cpu

```bash
go-sysmon cpu       # 核心数 + 占用率（采样 1 秒）
```

### mem

```bash
go-sysmon mem       # 总量/已用/可用，带百分比
```

### disk

```bash
go-sysmon disk      # 各盘符/挂载点使用情况
```

### process

```bash
go-sysmon process              # 进程列表
go-sysmon process -name go     # 按名字过滤
```

### network

```bash
go-sysmon network    # 网络接口与地址
```

### port

```bash
go-sysmon port       # 监听中的 TCP 端口
```

## 说明

零依赖纯 Go。跨平台取数走系统自带接口：CPU/内存/磁盘在 Windows 用 `wmic`、类 Unix 用 `/proc` 和 `df`，端口用 `netstat`/`ss`，进程用 `tasklist`/`/proc`，不引入任何第三方库。
