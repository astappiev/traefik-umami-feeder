package traefik_umami_feeder

type Auth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

func getToken(umamiHost string, umamiUsername string, umamiPassword string) (string, error) {
	var result AuthResponse
	err := sendRequestAndParse(umamiHost+"/api/auth/login", Auth{
		Username: umamiUsername,
		Password: umamiPassword,
	}, nil, &result)

	if err != nil {
		return "", err
	}

	return result.Token, nil
}
