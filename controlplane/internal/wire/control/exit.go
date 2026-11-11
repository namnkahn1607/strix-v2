package control

const (
	ExitOK       = 0
	ExitGeneral  = 1
	ExitUsage    = 2
	ExitNoDaemon = 3
	ExitConflict = 4
	ExitTimeout  = 5
)

func ExitCodeFor(c ErrorCode) int {
	switch {
	case c.IsConflict():
		return ExitConflict
	case c == CodeBadRequest:
		return ExitUsage
	}
	return ExitGeneral
}
