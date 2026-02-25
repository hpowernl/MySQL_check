package output

import (
	"fmt"
	"os"
	"strings"

	"github.com/hypernode/mysql-health-check/internal/checks"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[97m"
)

type Renderer struct {
	NoColor bool
}

func (r *Renderer) c(color, text string) string {
	if r.NoColor {
		return text
	}
	return color + text + colorReset
}

func (r *Renderer) levelColor(l checks.Level) string {
	switch l {
	case checks.LevelOK:
		return colorGreen
	case checks.LevelWarn:
		return colorYellow
	case checks.LevelCrit:
		return colorRed
	default:
		return colorGray
	}
}

func (r *Renderer) levelTag(l checks.Level) string {
	tag := fmt.Sprintf("[%s]", l.String())
	return r.c(r.levelColor(l), tag)
}

func (r *Renderer) Render(categories []checks.Category, mysqlVersion, hostname, cnfPath string) {
	w := os.Stdout
	lineW := 80

	border := strings.Repeat("=", lineW)
	fmt.Fprintln(w)
	fmt.Fprintln(w, r.c(colorCyan, border))
	fmt.Fprintf(w, "  %s%s", r.c(colorBold, "MySQL Health Checks"),
		r.pad("MySQL "+mysqlVersion, lineW-21))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Host: %s | CNF: %s\n", hostname, cnfPath)
	fmt.Fprintln(w, r.c(colorCyan, border))

	for _, cat := range categories {
		fmt.Fprintln(w)
		worst := cat.WorstLevel()
		catHeader := fmt.Sprintf("  %s", r.c(colorBold, cat.Name))
		catTag := r.levelTag(worst)
		padding := lineW - visibleLen(cat.Name) - 2 - visibleLen(fmt.Sprintf("[%s]", worst.String()))
		if padding < 1 {
			padding = 1
		}
		fmt.Fprintf(w, "%s%s%s\n", catHeader, strings.Repeat(" ", padding), catTag)
		fmt.Fprintln(w, r.c(colorGray, "  "+strings.Repeat("-", lineW-2)))

		for _, ch := range cat.Checks {
			tag := r.levelTag(ch.Level)
			name := ch.Name
			value := ch.Value

			namePad := 32 - len(name)
			if namePad < 1 {
				namePad = 1
			}

			fmt.Fprintf(w, "  %s  %s%s%s\n",
				tag, name, strings.Repeat(" ", namePad), value)

			if ch.Level == checks.LevelWarn || ch.Level == checks.LevelCrit {
				reason := fmt.Sprintf(">> Threshold: %s", ch.Threshold)
				fmt.Fprintf(w, "          %s\n", r.c(r.levelColor(ch.Level), reason))
			}

			descLines := wrapText(ch.Description, lineW-10)
			for _, dl := range descLines {
				fmt.Fprintf(w, "          %s\n", r.c(colorGray, dl))
			}

			detailLines := wrapText(ch.Detail, lineW-10)
			for _, dl := range detailLines {
				fmt.Fprintf(w, "          %s\n", r.c(colorGray, dl))
			}
			fmt.Fprintln(w)
		}
	}

	r.renderSummary(w, categories, lineW)
}

func (r *Renderer) renderSummary(w *os.File, categories []checks.Category, lineW int) {
	border := strings.Repeat("=", lineW)
	thin := strings.Repeat("-", lineW-4)

	var issues []checks.Check
	for _, cat := range categories {
		for _, ch := range cat.Checks {
			if ch.Level == checks.LevelWarn || ch.Level == checks.LevelCrit {
				issues = append(issues, ch)
			}
		}
	}

	overall := checks.OverallLevel(categories)
	fmt.Fprintln(w, r.c(colorCyan, border))

	if len(issues) == 0 {
		fmt.Fprintln(w, r.c(colorGreen, r.c(colorBold, "  Overall: OK - All checks passed")))
		fmt.Fprintln(w, r.c(colorCyan, border))
		fmt.Fprintln(w)
		return
	}

	fmt.Fprintf(w, "  %s  %d issue(s) found\n",
		r.c(r.levelColor(overall), r.c(colorBold, fmt.Sprintf("Overall: %s", overall.String()))),
		len(issues))
	fmt.Fprintln(w, r.c(colorCyan, border))
	fmt.Fprintln(w)

	nameCol := 30
	valCol := 18
	threshCol := 28

	header := fmt.Sprintf("  %-*s  %-*s  %-*s  %s",
		nameCol, "Check", valCol, "Value", threshCol, "Threshold", "Status")
	fmt.Fprintln(w, r.c(colorBold, header))
	fmt.Fprintf(w, "  %s\n", r.c(colorGray, thin))

	for _, ch := range issues {
		name := ch.Name
		if len(name) > nameCol {
			name = name[:nameCol-1] + "."
		}
		val := ch.Value
		if len(val) > valCol {
			val = val[:valCol-1] + "."
		}
		thresh := ch.Threshold
		if len(thresh) > threshCol {
			thresh = thresh[:threshCol-1] + "."
		}
		tag := r.levelTag(ch.Level)
		fmt.Fprintf(w, "  %-*s  %-*s  %-*s  %s\n",
			nameCol, name, valCol, val, threshCol, thresh, tag)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, r.c(colorCyan, border))
	fmt.Fprintln(w)
}

func (r *Renderer) pad(s string, width int) string {
	pad := width - len(s)
	if pad < 1 {
		pad = 1
	}
	return strings.Repeat(" ", pad) + s
}

func visibleLen(s string) int {
	inEscape := false
	count := 0
	for _, ch := range s {
		if ch == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				inEscape = false
			}
			continue
		}
		count++
	}
	return count
}

func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 70
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len(current)+1+len(w) > maxWidth {
			lines = append(lines, current)
			current = w
		} else {
			current += " " + w
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
