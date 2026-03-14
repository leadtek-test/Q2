# Q2 Logging System

這個專案實作麗臺科技面試作業第二題：設計一個可擴展的日誌管理系統。  
目標是用 Go 1.26 建立一個可重用的 logging library，支援可插拔儲存、分級過濾、非同步寫入、處理器擴充，以及一個可直接展示的 CLI demo。

## Features

- `LogManager` 提供統一入口，負責 level 驗證、非同步入隊、handler 分派。
- `LogStore` 介面可切換不同儲存後端，目前提供 `memory`、`file`、`influxdb`，且可同時啟用多個 backend。
- 分級日誌支援 `DEBUG`、`INFO`、`WARN`、`ERROR`，預設最小等級為 `INFO`。
- `Flush()` 與 `Close()` 讓非同步流程在測試與整合時可控。
- `WriteLogWithAttrs()` 支援 structured logging 欄位寫入。
- `SetFormatter()` 可切換 `TextFormatter`（預設）或 `JsonFormatter`。
- `LogHandler` 提供後續擴充點，方便接遠端聚合、分析、告警等需求。
- `FileStore` 使用 JSONL，保留未來擴充 structured logging 的空間。
- `FileStore` 改為 buffered write + `Flush()`，降低 syscall 與 GC 壓力。
- `InfluxDB Store` 使用時序資料庫，適合高吞吐 log 事件儲存與時間區間查詢。

## Project Layout

```text
.
├── cmd/logdemo                # CLI demo
├── pkg/logging                # 公開型別、介面與 LogManager
└── pkg/logging/store
    ├── file                   # JSONL file backend
    ├── influxdb               # InfluxDB time-series backend
    ├── multi                  # multi-store fan-out backend
    └── memory                 # in-memory backend
```

## Core Interfaces

```go
type LogWriter interface {
    Write(entry LogEntry) error
}

type LogReader interface {
    Read(level Level, filter LogFilter) ([]LogEntry, error)
    Clear(before time.Time) error
}

type LogHandler interface {
    Handle(entry LogEntry) error
}

type Formatter interface {
    Format(entry LogEntry) (string, error)
}
```

```go
// structured logging example
_ = manager.WriteLogWithAttrs("INFO", "request finished", map[string]any{
    "trace_id": "abc-123",
    "latency_ms": 42,
})
```

`LogEntry` 包含 `ID`、`Timestamp`、`Level`、`Message`、`Source`、`Attrs`。  
`Attrs` 在 v1 先保留擴充位，讓之後加入 structured logging 時不需要重做資料模型。

## Run Demo

```bash
go run ./cmd/logdemo -backend=memory
go run ./cmd/logdemo -backend=file -file=tmp/logdemo/logs.jsonl
go run ./cmd/logdemo -backend=memory -format=json
go run ./cmd/logdemo -backend=file -file=tmp/logdemo/logs.jsonl -clear=true
go run ./cmd/logdemo -backend=memory,file -file=tmp/logdemo/logs.jsonl
go run ./cmd/logdemo -backend=influx \
  --influx-url=http://localhost:8086 \
  --influx-token=q2-dev-token \
  --influx-org=q2 \
  --influx-bucket=logging
go run ./cmd/logdemo -backend=file,influx \
  --file=tmp/logdemo/logs.jsonl \
  --influx-url=http://localhost:8086 \
  --influx-token=q2-dev-token \
  --influx-org=q2 \
  --influx-bucket=logging
```

## Run InfluxDB (Docker Compose)

```bash
docker compose up -d influxdb
docker compose ps
```

預設初始化參數（定義於 `docker-compose.yml`）：

- URL: `http://localhost:8086`
- Org: `q2`
- Bucket: `logging`
- Token: `q2-dev-token`

CLI demo 會展示：

- 寫入多個 level 的日誌
- `SetFormatter()` 切換輸出格式（text/json）
- `Flush()` 後查詢 persisted logs（可用 `ReadFormattedLogs` 取得格式化後輸出）
- 依 level + time range 讀取
- 預設保留寫入檔案內容，若加上 `-clear=true` 才會呼叫 `ClearLogs()` 清理舊資料
- `-backend` 支援逗號分隔（例如 `file,influx`），可同時寫入多個 store
- 註冊 handler 並輸出處理結果

## Test

```bash
go test ./...
```

測試覆蓋：

- level parsing 與非法輸入
- manager 非同步寫入、queue full、關閉後寫入
- handler 失敗隔離
- 多 goroutine 寫入
- memory/file/influxdb store 讀取、過濾、清理
- CLI demo 整合測試
