package main

import (
	"fmt"
	"strconv"
)

// CalculateSSCCCheckDigit 计算SSCC码的校验位
func CalculateSSCCCheckDigit(ssccWithoutCheckDigit string) (int, error) {
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

func ssccValMain() {
	// 示例：输入一个17位的SSCC码（不包含校验位）
	ssccWithoutCheckDigit := "13597920999999982"

	// 计算校验位
	checkDigit, err := CalculateSSCCCheckDigit(ssccWithoutCheckDigit)
	if err != nil {
		fmt.Println("错误:", err)
		return
	}

	// 输出完整的SSCC码
	fullSSCC := ssccWithoutCheckDigit + strconv.Itoa(checkDigit)
	fmt.Println("完整的SSCC码:", fullSSCC)
}
