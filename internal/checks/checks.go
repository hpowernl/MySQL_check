package checks

import "fmt"

type Level int

const (
	LevelOK Level = iota
	LevelWarn
	LevelCrit
	LevelSkip
)

func (l Level) String() string {
	switch l {
	case LevelOK:
		return "OK"
	case LevelWarn:
		return "WARN"
	case LevelCrit:
		return "CRIT"
	case LevelSkip:
		return "SKIP"
	default:
		return "UNKNOWN"
	}
}

type Check struct {
	Name        string
	Value       string
	Level       Level
	Threshold   string
	Description string
	Detail      string
}

type Category struct {
	Name   string
	Checks []Check
}

func (c *Category) WorstLevel() Level {
	worst := LevelOK
	for _, ch := range c.Checks {
		if ch.Level == LevelSkip {
			continue
		}
		if ch.Level > worst {
			worst = ch.Level
		}
	}
	return worst
}

func OverallLevel(cats []Category) Level {
	worst := LevelOK
	for _, c := range cats {
		l := c.WorstLevel()
		if l > worst {
			worst = l
		}
	}
	return worst
}

func pct(numerator, denominator float64) (float64, bool) {
	if denominator == 0 {
		return 0, false
	}
	return (numerator * 100.0) / denominator, true
}

func fmtPct(v float64) string {
	return fmt.Sprintf("%.2f%%", v)
}

func fmtMin(v float64) string {
	return fmt.Sprintf("%.0fmin", v)
}
