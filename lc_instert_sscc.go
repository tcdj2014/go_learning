package main

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"os"
	"sync"
	"time"
)

const (
	batchSize    = 200   // 每个事务插入量
	maxWorkers   = 16    // 并行工作协程数
	retryMax     = 3     // 失败重试次数
	mysqlTimeout = "30s" // 数据库超时时间
)

var pool = sync.Pool{
	New: func() interface{} {
		return make([]string, 0, batchSize)
	},
}

func instertSsccMain() {
	// 1. 初始化数据库连接池
	db := initDB("username:passwd@tcp(db_host:db_port)/db_name?parseTime=true&timeout=" + mysqlTimeout)
	defer db.Close()

	// 2. 计算文件哈希（数据完整性校验）
	filePath := "/tmp/sscc.txt"
	originalHash, err := calculateFileHash(filePath)
	if err != nil {
		log.Fatalf("文件哈希计算失败: %v", err)
	}

	// 3. 创建并行处理管道
	dataCh := make(chan []string, maxWorkers)
	var wg sync.WaitGroup

	// 4. 启动工作协程池
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go batchProcessor(db, dataCh, &wg)
	}

	// 5. 分批次读取文件
	if err := readBatches(filePath, dataCh); err != nil {
		log.Fatalf("文件读取失败: %v", err)
	}
	close(dataCh)
	wg.Wait()

	// 6. 一致性验证
	if validateConsistency(db, filePath, originalHash) {
		fmt.Println("✅ 数据一致性验证通过")
	} else {
		fmt.Println("❌ 数据不一致，请检查错误日志")
	}
}

// 初始化数据库连接池
func initDB(dsn string) *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	// 连接池配置
	db.SetMaxOpenConns(maxWorkers * 2) // 根据并发量调整
	db.SetMaxIdleConns(maxWorkers)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err = db.Ping(); err != nil {
		log.Fatalf("数据库连接测试失败: %v", err)
	}
	return db
}

// 分批次读取文件
func readBatches(path string, ch chan<- []string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	batch := pool.Get().([]string)

	for scanner.Scan() {
		batch = append(batch, scanner.Text())
		if len(batch) >= batchSize {
			ch <- batch
			pool.Put(make([]string, 0, batchSize)) // 归还空切片
			batch = pool.Get().([]string)
		}
	}

	if len(batch) > 0 {
		ch <- batch
		pool.Put(make([]string, 0, batchSize))
	}
	return scanner.Err()
}

// 批量处理器（带重试机制）
func batchProcessor(db *sql.DB, ch <-chan []string, wg *sync.WaitGroup) {
	defer wg.Done()
	for batch := range ch {
		var lastErr error
		for i := 0; i < retryMax; i++ {
			if err := insertBatch(db, batch); err == nil {
				break // 成功则退出重试
			} else {
				lastErr = err
				time.Sleep(time.Duration(i+1) * 100 * time.Millisecond) // 线性增长延迟
			}
		}
		if lastErr != nil {
			log.Printf("最终插入失败: %v", lastErr)
			saveFailedBatch(batch)
		}
	}
}

// 单批次插入事务
func insertBatch(db *sql.DB, batch []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("事务启动失败: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			log.Printf("事务异常回滚: %v", p)
			tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare("INSERT INTO lc_sscc (code,status,createdBy,lastUpdatedBy) VALUES (?,100,'txm','txm')")
	if err != nil {
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	for _, code := range batch {
		if _, err := stmt.Exec(code); err != nil {
			tx.Rollback()
			return fmt.Errorf("插入失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

// 文件哈希计算（SHA256）
func calculateFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := bufio.NewReader(file).WriteTo(hasher); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// 一致性验证
func validateConsistency(db *sql.DB, path string, originalHash string) bool {
	// 文件完整性验证
	currentHash, err := calculateFileHash(path)
	if err != nil {
		log.Printf("文件哈希计算失败: %v", err)
		return false
	}
	if currentHash != originalHash {
		log.Printf("文件篡改检测: 原始哈希 %s ≠ 当前哈希 %s", originalHash, currentHash)
		return false
	}

	// 行数比对
	fileLines, err := countFileLines(path)
	if err != nil {
		log.Printf("文件行数统计失败: %v", err)
		return false
	}
	var dbCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM lc_sscc").Scan(&dbCount); err != nil {
		log.Printf("数据库计数失败: %v", err)
		return false
	}

	if fileLines != dbCount {
		log.Printf("数量不一致: 文件行数 %d ≠ 数据库记录 %d", fileLines, dbCount)
		return false
	}
	return true
}

// 辅助函数：统计文件行数
func countFileLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

// 保存失败批次到日志文件
func saveFailedBatch(batch []string) {
	f, err := os.OpenFile("failed.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("日志文件打开失败: %v", err)
		return
	}
	defer f.Close()

	for _, code := range batch {
		if _, err := f.WriteString(code + "\n"); err != nil {
			log.Printf("日志写入失败: %v", err)
		}
	}
}
