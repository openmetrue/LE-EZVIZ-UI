package client

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

func (LEZ *LE_EZVIZ_Client) GetSessionIDClaims() (jwt.MapClaims, error) {
	if LEZ.LoginResponse.LoginSession.SessionId == nil {
		log.Error("You need to login first before getting sessionID claims")
		return nil, errors.New("you need to login first")
	}
	return parseNoVerify(*LEZ.LoginResponse.LoginSession.SessionId)
}

func parseNoVerify(tokenString string) (jwt.MapClaims, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid claims type")
}
