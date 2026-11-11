package process

func validateCommandSize(executable string, args []string) error {
	return validateWindowsCommandSize(executable, args)
}
