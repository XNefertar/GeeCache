package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"geecache"
	"log"
	"time"
	// 注意：实际运行时需要安装 MySQL 驱动
	// _ "github.com/go-sql-driver/mysql"
)

// ==========================================
// 1. 数据模型定义 (Data Model)
// ==========================================

// User 是我们要查询的业务实体
type User struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// ==========================================
// 2. 应用程序上下文 (App Context)
// ==========================================

// App 结构体持有数据库连接和缓存组的引用
// 在真实项目中，这通常是 Service 或 Repository 层
type App struct {
	DB        *sql.DB
	UserCache *geecache.Group
}

// NewApp 初始化应用依赖
func NewApp(dsn string) (*App, error) {
	// 1. 连接 MySQL
	// db, err := sql.Open("mysql", dsn)
	// if err != nil { return nil, err }
	var db *sql.DB // 这里仅演示，未实际连接

	app := &App{DB: db}

	// 2. 初始化缓存组
	// 核心逻辑：定义 "当缓存未命中时，如何去数据库取数据"
	app.UserCache, _ = geecache.NewGroup("users", 100*1024*1024, geecache.GetterFunc(
		func(ctx context.Context, key string) ([]byte, error) {
			// === 缓存回源逻辑 (Cache Miss Logic) ===
			log.Printf("[GeeCache] Key %s missed, fetching from MySQL...", key)

			// 调用内部方法查库
			user, err := app.queryUserFromDB(key)
			if err != nil {
				return nil, err
			}

			// 序列化：将 Go 结构体转为 []byte 存入缓存
			return json.Marshal(user)
		},
	))

	return app, nil
}

// queryUserFromDB 是纯粹的数据库查询逻辑
func (app *App) queryUserFromDB(id string) (*User, error) {
	// 模拟数据库查询代码
	// row := app.DB.QueryRow("SELECT id, username, email FROM users WHERE id = ?", id)
	// var u User
	// if err := row.Scan(&u.ID, &u.Username, &u.Email); err != nil {
	//    return nil, err
	// }

	// 模拟返回
	return &User{ID: 1001, Username: "tom_cat", Email: "tom@example.com"}, nil
}

// ==========================================
// 3. 对外暴露的查询接口 (Public API)
// ==========================================

// GetUser 是业务代码直接调用的方法
// 它对外隐藏了 "是查缓存还是查数据库" 的细节
func (app *App) GetUser(ctx context.Context, userID string) (*User, error) {
	// step 1: 调用 GeeCache
	// 甚至不需要手动检查缓存是否存在，GeeCache 内部会自动处理 (Get -> Miss -> Load -> Set -> Return)
	byteView, err := app.UserCache.Get(ctx, userID)
	if err != nil {
		return nil, err
	}

	// step 2: 反序列化
	// 将缓存返回的通用 []byte 还原为业务结构体
	var u User
	if err := json.Unmarshal(byteView.ByteSlice(), &u); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %v", err)
	}

	return &u, nil
}

// ==========================================
// 4. 调用示例
// ==========================================

func main() {
	// 初始化
	app, _ := NewApp("root:password@tcp(127.0.0.1:3306)/mydb")

	// 模拟业务调用
	ctx := context.Background()

	// 第一次调用：因为缓存为空，会触发 "fetch from MySQL"
	fmt.Println("--- Request 1 ---")
	u1, _ := app.GetUser(ctx, "1001")
	fmt.Printf("Result: %+v\n", u1)

	// 第二次调用：直接命中缓存，不会看到 fetching 日志
	fmt.Println("\n--- Request 2 ---")
	u2, _ := app.GetUser(ctx, "1001")
	fmt.Printf("Result: %+v\n", u2)
}
