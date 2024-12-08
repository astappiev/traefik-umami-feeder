package traefik_umami_feeder

import "context"

type Auth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

func getToken(ctx context.Context, umamiHost string, umamiUsername string, umamiPassword string) (string, error) {
	var result AuthResponse
	err := sendRequestAndParse(ctx, umamiHost+"/api/auth/login", Auth{
		Username: umamiUsername,
		Password: umamiPassword,
	}, nil, &result)

	if err != nil {
		return "", err
	}

	return result.Token, nil
}
