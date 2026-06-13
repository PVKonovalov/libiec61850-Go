package mms

import (
	"fmt"
)

//
//<EnumVal ord="1">25 ns</EnumVal>
//<EnumVal ord="2">100 ns</EnumVal>
//<EnumVal ord="3">250 ns</EnumVal>
//<EnumVal ord="4">1 us</EnumVal>
//<EnumVal ord="5">2.5 us</EnumVal>
//<EnumVal ord="6">10 us</EnumVal>
//<EnumVal ord="7">25 us</EnumVal>
//<EnumVal ord="8">100 us</EnumVal>
//<EnumVal ord="9">250 us</EnumVal>
//<EnumVal ord="10">1 ms</EnumVal>
//<EnumVal ord="11">2.5 ms</EnumVal>
//<EnumVal ord="12">10 ms</EnumVal>
//<EnumVal ord="13">25 ms</EnumVal>
//<EnumVal ord="14">100 ms</EnumVal>
//<EnumVal ord="15">250 ms</EnumVal>
//<EnumVal ord="16">1 s</EnumVal>
//<EnumVal ord="17">10 s</EnumVal>
//<EnumVal ord="18">more than 10 s</EnumVal>

func getTimeAccuracy(n byte) string {
	switch n {
	case 7:
		return "10ms"
	case 10:
		return "1ms"
	case 14:
		return "100μs"
	case 16:
		return "25μs"
	case 18:
		return "4μs"
	case 20:
		return "1μs"
	case 31:
		return "unspecified"
	default:
		divisor := 1 << n
		return fmt.Sprintf("~%.3fms", 1000.0/float64(divisor))
	}
}

func getPerformanceClass(n byte) int {
	switch n {
	case 7:
		return 0
	case 10:
		return 1
	case 14:
		return 2
	case 16:
		return 3
	case 18:
		return 4
	case 20:
		return 5
	default:
		return -1
	}
}
