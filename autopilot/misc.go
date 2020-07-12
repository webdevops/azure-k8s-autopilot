package autopilot

func stringArrayContains(arr []string, needle string) bool {
	for _, val := range arr {
		if val == needle {
			return true
		}
	}

	return false
}
