package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
)

const (
	fixedDigit  = "1"
	companyCode = "1111110"
	startSerial = 100_000_000
	endSerial   = startSerial + 10_000 - 1
	outputFile  = "/tmp/sscc.txt"
	bufferSize  = 128 * 1024 * 1024 // 64MB缓冲区
)

func createSsccMain() {
	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Printf("file create faile: %v\n", err)
		return
	}
	defer file.Close()

	writer := bufio.NewWriterSize(file, bufferSize)
	defer writer.Flush()

	// 使用带缓冲的生产者队列
	serialCh := make(chan int, 500_000)
	ssccCh := make(chan string, 100_000)
	errCh := make(chan error, 1)
	stopCh := make(chan struct{}) // 紧急停止信号

	// 启动生产者协程
	go func() {
		defer close(serialCh)
		for serial := startSerial; serial <= endSerial; serial++ {
			select {
			case serialCh <- serial:
			case <-stopCh:
				return
			}
		}
	}()

	// 启动工作协程池
	var wg sync.WaitGroup
	workerCount := runtime.GOMAXPROCS(0) * 3 // 更激进的并发策略
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for serial := range serialCh {
				select {
				case <-stopCh:
					return
				default:
					prefix := fmt.Sprintf("%s%s%09d", fixedDigit, companyCode, serial)
					checkDigit, err := calculateSSCCCheckDigit(prefix)
					if err != nil {
						fmt.Println("error:", err)
						return
					}
					ssccCh <- fmt.Sprintf("%s%d", prefix, checkDigit)
				}
			}
		}()
	}

	// 启动写入协程
	var writeWG sync.WaitGroup
	writeWG.Add(1)
	go func() {
		defer writeWG.Done()
		for sscc := range ssccCh {
			if _, err := writer.WriteString(sscc + "\n"); err != nil {
				select {
				case errCh <- fmt.Errorf("write error: %v", err):
				default: // 防止多次错误写入
				}
				close(stopCh)
				return
			}
		}
	}()

	// 错误监控
	go func() {
		select {
		case err := <-errCh:
			fmt.Printf("执行错误: %v\n", err)
			close(stopCh)
		}
	}()

	// 等待所有工作完成
	wg.Wait()      // 等待所有worker完成
	close(ssccCh)  // 安全关闭SSCC通道
	writeWG.Wait() // 等待写入完成

	// 最终错误检查
	if len(errCh) > 0 {
		os.Exit(1)
	}
}

// 改进的Luhn算法实现
func calculateLuhn(numberStr string) int {
	sum := 0
	isSecond := true
	for i := len(numberStr) - 1; i >= 0; i-- {
		digit := int(numberStr[i] - '0')
		if isSecond {
			if digit *= 2; digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		isSecond = !isSecond
	}
	return (sum * 9) % 10
}

func calculateSSCCCheckDigit(ssccWithoutCheckDigit string) (int, error) {
	// 确保输入长度为17位（不包括校验位）
	if len(ssccWithoutCheckDigit) != 17 {
		return 0, fmt.Errorf("输入的SSCC码必须为17位（不包含校验位）")
	}

	// 将字符串转换为数字数组
	digits := make([]int, 17)
	for i, char := range ssccWithoutCheckDigit {
		digit, err := strconv.Atoi(string(char))
		if err != nil {
			return 0, fmt.Errorf("输入包含非数字字符: %v", err)
		}
		digits[i] = digit
	}

	// 根据GS1校验位规则计算加权和
	// 从右到左，偶数位乘以3，奇数位保持不变
	sum := 0
	for i := 0; i < 17; i++ {
		if i%2 == 0 { // 偶数索引（从0开始）
			sum += digits[i] * 3
		} else { // 奇数索引
			sum += digits[i]
		}
	}

	// 计算校验位
	checkDigit := (10 - (sum % 10)) % 10
	return checkDigit, nil
}
