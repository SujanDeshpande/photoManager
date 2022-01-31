package services

var staticResponseCode = []int{400, 401, 500}

func Parser(carrier string, trackingNumber string) string {
	return "sujan"
}

func intInSlice(a int, list []int) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
