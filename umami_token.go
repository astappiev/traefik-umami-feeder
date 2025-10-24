package traefik_umami_feeder

import "context"

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
}

func getToken(ctx context.Context, umamiHost, umamiUsername, umamiPassword string) (string, error) {
	var result authResponse
	err := sendRequestAndParse(ctx, umamiHost+"/api/auth/login", authRequest{
		Username: umamiUsername,
		Password: umamiPassword,
	}, nil, &result)
	if err != nil {
		return "", err
	}

	return result.Token, nil
}
