package auth

func CheckLogin(username, password string) (bool, error) {
	// Dữ liệu hardcoded
	accounts := map[string]string{
		"alice": "123",
		"bob":   "456",
	}

	if pass, ok := accounts[username]; ok && pass == password {
		return true, nil
	}
	return false, nil
}
