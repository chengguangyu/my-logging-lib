package startlogger

import (
	"fmt"
	"unicode/utf8"
)

func lb(log string) string {
	return lb2(log, 100)
}

func lb2(log string, width int) string {

	out := ""

	i := 0

	for i < utf8.RuneCountInString(log) {
		if i > 0 && i%width == 0 {
			out += "\r\n"
		}

		out += string(log[i])
		i++
	}

	j := i % width

	for j < width {
		out += "."
		j++
	}

	return out

}

func CommandDone() {

	fmt.Println("✓___________________________________________")
}

func CommandWarn(log string) {
	fmt.Println("[• WARN ] " + log)
}

func CommandDebug(log string) {
	fmt.Println("[✓ DEBUG] " + (log))
}

func CommandFyi(log string) {
	fmt.Println("[✓ FYI  ] " + (log))
}

func CommandGood(log string) {
	fmt.Println("[✓ OK   ] " + (log))
}

func CommandPanic(log string) {
	fmt.Println("[○ PANIC] " + (log))
}

func CommandFail(log string) {
	fmt.Println("[○ FAIL ] " + log)
}
