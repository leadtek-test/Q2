# Q2 Logging System

這個專案實作麗臺科技面試作業第二題：設計一個可擴展的日誌管理系統。  
目標是用 Go 1.26 建立一個可重用的 logging library，支援可插拔儲存、分級過濾、非同步寫入、處理器擴充，以及一個可直接展示的 CLI demo。

## Features

- `LogManager` 提供統一入口，負責 level 驗證、非同步入隊、handler 分派。
- `LogStore` 介面可切換不同儲存後端，目前提供 `memory` 與 `file`。
- 分級日誌支援 `DEBUG`、`INFO`、`WARN`、`ERROR`，預設最小等級為 `INFO`。
- `Flush()` 與 `Close()` 讓非同步流程在測試與整合時可控。
- `SetFormatter()` 可切換 `TextFormatter`（預設）或 `JsonFormatter`。
- `LogHandler` 提供後續擴充點，方便接遠端聚合、分析、告警等需求。
- `FileStore` 使用 JSONL，保留未來擴充 structured logging 的空間。

## Project Layout

```text
.
├── cmd/logdemo                # CLI demo
├── pkg/logging                # 公開型別、介面與 LogManager
└── pkg/logging/store
    ├── file                   # JSONL file backend
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

`LogEntry` 包含 `ID`、`Timestamp`、`Level`、`Message`、`Source`、`Attrs`。  
`Attrs` 在 v1 先保留擴充位，讓之後加入 structured logging 時不需要重做資料模型。

## Run Demo

```bash
go run ./cmd/logdemo -backend=memory
go run ./cmd/logdemo -backend=file -file=tmp/logdemo/logs.jsonl
go run ./cmd/logdemo -backend=memory -format=json
```

CLI demo 會展示：

- 寫入多個 level 的日誌
- `SetFormatter()` 切換輸出格式（text/json）
- `Flush()` 後查詢 persisted logs（可用 `ReadFormattedLogs` 取得格式化後輸出）
- 依 level + time range 讀取
- `ClearLogs()` 清理舊資料
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
- memory/file store 讀取、過濾、清理
- CLI demo 整合測試
