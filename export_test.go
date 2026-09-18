package cnfg

// SetExit swaps what [MustParse] ends the program with, and gives back the function that
// puts the old one back.
func SetExit(fn func(int)) func() {
	old := exit
	exit = fn
	return func() { exit = old }
}
