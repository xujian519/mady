package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver
)

// openDB 打开或创建一个 WAL 模式 SQLite 数据库，应用通用连接池设置。
// 所有 *Store 类型的构造函数均使用此 helper 消除重复的 DSN 构建、Ping 和连接池配置。
func openDB(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(4)
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s: %w", dbPath, err)
	}
	return db, nil
}
