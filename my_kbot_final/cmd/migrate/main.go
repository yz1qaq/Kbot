package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	var (
		up          = flag.Bool("up", false, "迁移到最新版本")
		down        = flag.Int("down", 0, "回退 N 步")
		showVer     = flag.Bool("version", false, "打印当前迁移版本")
		path        = flag.String("path", "migrations", "迁移文件目录")
		databaseEnv = flag.String("database-env", "KBOT_DATABASE_URL", "数据库连接串的环境变量名")
	)
	flag.Parse()

	rawURL := os.Getenv(*databaseEnv)
	if rawURL == "" {
		log.Fatalf("缺少环境变量：%s", *databaseEnv)
	}

	dbURL := rawURL
	if strings.HasPrefix(dbURL, "postgres://") {
		dbURL = "pgx5://" + strings.TrimPrefix(dbURL, "postgres://")
	} else if strings.HasPrefix(dbURL, "postgresql://") {
		dbURL = "pgx5://" + strings.TrimPrefix(dbURL, "postgresql://")
	}

	m, err := migrate.New("file://"+*path, dbURL)
	if err != nil {
		log.Fatalf("初始化迁移失败：%v", err)
	}
	defer m.Close()

	switch {
	case *showVer:
		version, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			log.Println("当前版本：无，数据库尚未迁移")
			return
		}
		if err != nil {
			log.Fatalf("读取版本失败：%v", err)
		}
		log.Printf("当前版本：%d(dirty=%v)", version, dirty)

	case *down > 0:
		err := m.Steps(-*down)
		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("回退失败：%v", err)
		}
		log.Printf("回退完成：%d 步", *down)

	case *up:
		err := m.Up()
		if errors.Is(err, migrate.ErrNoChange) {
			log.Println("数据库已是最新版本")
			return
		}
		if err != nil {
			log.Fatalf("执行迁移失败：%v", err)
		}
		log.Println("数据库迁移完成")

	default:
		flag.Usage()
		os.Exit(2)
	}
}
