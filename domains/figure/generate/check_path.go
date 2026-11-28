package generate

import (
	"math"
	"strconv"
	"strings"
)

type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

func (m matrix) apply(p [2]float64) [2]float64 {
	return [2]float64{m[0]*p[0] + m[2]*p[1] + m[4], m[1]*p[0] + m[3]*p[1] + m[5]}
}

// multiply 返回 m·n（先应用 n，再应用 m）。
func (m matrix) multiply(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

func (m matrix) scaleFactor() float64 {
	return math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2]))
}

// parsePath 解析 path 的 d 属性；曲线按 8 段折线采样，弧线降级为弦并记未量测。
//
//nolint:gocognit // SVG 路径命令的状态机，按命令分派本就分支密集
func parsePath(d string, emit func(from, to [2]float64), note func(reason string)) {
	type command struct {
		letter byte
		args   []float64
	}
	var commands []command
	current := command{}
	for _, field := range tokenizePath(d) {
		if len(field) == 1 && strings.ContainsAny(field, "MmLlHhVvCcSsQqTtAaZz") {
			if current.letter != 0 {
				commands = append(commands, current)
			}
			current = command{letter: field[0]}
			continue
		}
		value, err := strconv.ParseFloat(field, 64)
		if err != nil {
			// 非法数值→视为路径结束，忽略其后指令（仅影响几何核验精度）。
			return
		}
		current.args = append(current.args, value)
	}
	if current.letter != 0 {
		commands = append(commands, current)
	}

	position := [2]float64{}
	start := [2]float64{}
	var lastControl [2]float64
	hasControl := false

	for _, cmd := range commands {
		relative := cmd.letter >= 'a' && cmd.letter <= 'z'
		letter := cmd.letter
		if relative {
			letter = letter - 'a' + 'A'
		}
		args := cmd.args
		step := 2
		switch letter {
		case 'H', 'V':
			step = 1
		case 'C':
			step = 6
		case 'S', 'Q':
			step = 4
		case 'T':
			step = 2
		case 'A':
			step = 7
		}
		if letter == 'Z' {
			if position != start {
				emit(position, start)
				position = start
			}
			hasControl = false
			continue
		}
		if len(args) < step || step == 0 || len(args)%step != 0 {
			return
		}
		for i := 0; i+step <= len(args); i += step {
			point := func(offset int) [2]float64 {
				p := [2]float64{args[i+offset], args[i+offset+1]}
				if relative {
					return [2]float64{position[0] + p[0], position[1] + p[1]}
				}
				return p
			}
			switch letter {
			case 'M':
				target := point(0)
				if i == 0 && letter == 'M' {
					position = target
					start = target
				} else {
					emit(position, target)
					position = target
				}
				hasControl = false
			case 'L':
				target := point(0)
				emit(position, target)
				position = target
				hasControl = false
			case 'H':
				x := args[i]
				if relative {
					x += position[0]
				}
				target := [2]float64{x, position[1]}
				emit(position, target)
				position = target
				hasControl = false
			case 'V':
				y := args[i]
				if relative {
					y += position[1]
				}
				target := [2]float64{position[0], y}
				emit(position, target)
				position = target
				hasControl = false
			case 'C':
				c1, c2, end := point(0), point(2), point(4)
				sampleCubic(position, c1, c2, end, emit)
				lastControl, hasControl = c2, true
				position = end
			case 'S':
				c2, end := point(0), point(2)
				c1 := position
				if hasControl {
					c1 = [2]float64{2*position[0] - lastControl[0], 2*position[1] - lastControl[1]}
				}
				sampleCubic(position, c1, c2, end, emit)
				lastControl, hasControl = c2, true
				position = end
			case 'Q':
				c1, end := point(0), point(2)
				sampleQuadratic(position, c1, end, emit)
				lastControl, hasControl = c1, true
				position = end
			case 'T':
				end := point(0)
				c1 := position
				if hasControl {
					c1 = [2]float64{2*position[0] - lastControl[0], 2*position[1] - lastControl[1]}
				}
				sampleQuadratic(position, c1, end, emit)
				lastControl, hasControl = c1, true
				position = end
			case 'A':
				end := [2]float64{args[i+5], args[i+6]}
				if relative {
					end = [2]float64{position[0] + end[0], position[1] + end[1]}
				}
				note("弧线按端点弦近似量测")
				emit(position, end)
				position = end
				hasControl = false
			}
		}
	}
}

func sampleCubic(from, c1, c2, to [2]float64, emit func(from, to [2]float64)) {
	previous := from
	for i := 1; i <= 8; i++ {
		t := float64(i) / 8
		u := 1 - t
		point := [2]float64{
			u*u*u*from[0] + 3*u*u*t*c1[0] + 3*u*t*t*c2[0] + t*t*t*to[0],
			u*u*u*from[1] + 3*u*u*t*c1[1] + 3*u*t*t*c2[1] + t*t*t*to[1],
		}
		emit(previous, point)
		previous = point
	}
}

func sampleQuadratic(from, c, to [2]float64, emit func(from, to [2]float64)) {
	previous := from
	for i := 1; i <= 8; i++ {
		t := float64(i) / 8
		u := 1 - t
		point := [2]float64{
			u*u*from[0] + 2*u*t*c[0] + t*t*to[0],
			u*u*from[1] + 2*u*t*c[1] + t*t*to[1],
		}
		emit(previous, point)
		previous = point
	}
}

func tokenizePath(d string) []string {
	return strings.FieldsFunc(d, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
	})
}

// ---------- 判定 ----------
