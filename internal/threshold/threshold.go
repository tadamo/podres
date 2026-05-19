package threshold

// Level represents a usage severity level.
type Level int

const (
	LevelOK   Level = iota // < warn threshold
	LevelWarn              // >= warn, < crit threshold
	LevelCrit              // >= crit threshold
)

// Config holds warn and critical percentage thresholds.
type Config struct {
	Warn int // default 75
	Crit int // default 95
}

// Classify returns the Level for a given usage percentage.
func (c Config) Classify(pct float64) Level {
	switch {
	case pct >= float64(c.Crit):
		return LevelCrit
	case pct >= float64(c.Warn):
		return LevelWarn
	default:
		return LevelOK
	}
}
