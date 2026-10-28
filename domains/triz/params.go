package triz

// 39 个经典工程参数（与随包矛盾矩阵的行列一一对应）。
// 名称只取自本表，不采用模型输出，避免同一编号在不同回答里叫法不一。

var engineeringParams = []Parameter{
	{Number: 1, Name: "运动物体重量"},
	{Number: 2, Name: "静止物体重量"},
	{Number: 3, Name: "运动物体长度"},
	{Number: 4, Name: "静止物体长度"},
	{Number: 5, Name: "运动物体面积"},
	{Number: 6, Name: "静止物体面积"},
	{Number: 7, Name: "运动物体体积"},
	{Number: 8, Name: "静止物体体积"},
	{Number: 9, Name: "速度"},
	{Number: 10, Name: "力"},
	{Number: 11, Name: "应力"},
	{Number: 12, Name: "形状"},
	{Number: 13, Name: "结构稳定性"},
	{Number: 14, Name: "强度"},
	{Number: 15, Name: "运动物体作用时间"},
	{Number: 16, Name: "静止物体作用时间"},
	{Number: 17, Name: "温度"},
	{Number: 18, Name: "光照度"},
	{Number: 19, Name: "运动物体能量"},
	{Number: 20, Name: "静止物体能量"},
	{Number: 21, Name: "功率"},
	{Number: 22, Name: "能量损失"},
	{Number: 23, Name: "物质损失"},
	{Number: 24, Name: "信息损失"},
	{Number: 25, Name: "时间损失"},
	{Number: 26, Name: "物质数量"},
	{Number: 27, Name: "可靠性"},
	{Number: 28, Name: "测量精度"},
	{Number: 29, Name: "制造精度"},
	{Number: 30, Name: "作用于物体的有害因素"},
	{Number: 31, Name: "物体产生的有害因素"},
	{Number: 32, Name: "可制造性"},
	{Number: 33, Name: "可操作性"},
	{Number: 34, Name: "可维修性"},
	{Number: 35, Name: "适应性"},
	{Number: 36, Name: "装置复杂性"},
	{Number: 37, Name: "检测复杂性"},
	{Number: 38, Name: "自动化程度"},
	{Number: 39, Name: "生产率"},
}

// Parameters 返回 39 个工程参数（按编号升序）。
func Parameters() []Parameter {
	out := make([]Parameter, len(engineeringParams))
	copy(out, engineeringParams)
	return out
}

// ParamLabel 返回工程参数的展示名称；未知编号返回编号本身。
func ParamLabel(no int) string {
	if no >= MatrixMin && no <= MatrixMax {
		return engineeringParams[no-1].Name
	}
	return itoa(no)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits [12]byte
	pos := len(digits)
	for value > 0 {
		pos--
		digits[pos] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		pos--
		digits[pos] = '-'
	}
	return string(digits[pos:])
}
