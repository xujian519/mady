package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// OpenReadOnly 以只读模式打开 SQLite 数据库，使用与知识库一致的
// busy_timeout 与连接数上限。调用方负责关闭返回的连接。
//
// 法律知识库（knowledge/legal）等旁路读取方共用本函数，避免各自拼接 DSN
// 导致读模式与 pragma 不一致。
func OpenReadOnly(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(2)
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	return db, nil
}
