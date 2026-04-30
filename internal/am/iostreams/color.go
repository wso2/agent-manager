package iostreams

const (
	ansiReset    = "\033[0m"
	ansiBold     = "\033[1m"
	ansiFgRed    = "\033[31m"
	ansiFgGreen  = "\033[32m"
	ansiFgCyan   = "\033[36m"
	ansiFgGray   = "\033[90m"
)

type ColorScheme struct {
	Enabled bool
}

func (cs *ColorScheme) apply(code, t string) string {
	if !cs.Enabled {
		return t
	}
	return code + t + ansiReset
}

func (cs *ColorScheme) Bold(t string) string  { return cs.apply(ansiBold, t) }
func (cs *ColorScheme) Red(t string) string   { return cs.apply(ansiFgRed, t) }
func (cs *ColorScheme) Green(t string) string { return cs.apply(ansiFgGreen, t) }
func (cs *ColorScheme) Cyan(t string) string  { return cs.apply(ansiFgCyan, t) }
func (cs *ColorScheme) Gray(t string) string  { return cs.apply(ansiFgGray, t) }

func (cs *ColorScheme) SuccessIcon() string { return cs.Green("✓") }
func (cs *ColorScheme) FailureIcon() string { return cs.Red("X") }

func (cs *ColorScheme) TableHeader(t string) string {
	if !cs.Enabled {
		return t
	}
	return ansiBold + t + ansiReset
}
