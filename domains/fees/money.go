package fees

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 金额一律以「分」为整数单位参与运算，避免浮点误差；费率以百分比整数表示，
// 乘算后四舍五入到分（这是本包的算术约定，只在此处声明一次）。

// yuanPattern 匹配十进制人民币金额：非负、至多两位小数、无指数。
var yuanPattern = regexp.MustCompile(`^(?:0|[1-9][0-9]*)(?:\.[0-9]{1,2})?$`)

// ParseYuan 把十进制人民币金额解析为分。
//
// 输入必须是资产里的写法（如 "900"、"94.5"），非法文本返回错误而不是猜测值。
func ParseYuan(value string) (int64, error) {
	if !yuanPattern.MatchString(value) {
		return 0, fmt.Errorf("fees: %q 不是合法的十进制人民币金额", value)
	}
	whole, fraction, _ := strings.Cut(value, ".")
	if fraction == "" && strings.Contains(value, ".") {
		return 0, fmt.Errorf("fees: %q 的小数部分为空", value)
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	yuan, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("fees: 解析金额 %q: %w", value, err)
	}
	fen, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("fees: 解析金额 %q: %w", value, err)
	}
	return yuan*100 + fen, nil
}

// FormatYuan 把分格式化为两位小数的元。
func FormatYuan(fen int64) string {
	sign := ""
	if fen < 0 {
		sign = "-"
		fen = -fen
	}
	return fmt.Sprintf("%s%d.%02d", sign, fen/100, fen%100)
}

// ApplyPercent 按百分比缩放分金额，四舍五入到分。
func ApplyPercent(fen int64, percent int) int64 {
	// (fen*percent + 50) / 100 即四舍五入；fen 与 percent 均为非负。
	return (fen*int64(percent) + 50) / 100
}
