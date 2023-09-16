package auth

import (
	"fmt"
	"github.com/golang-jwt/jwt"
	"os"
)

type JwtPayload struct {
	Username       string `json:"username"`
	Admin          bool   `json:"admin"`
	UserId         string `json:"userId"`
	AvatarObjectId string `json:"avatarObjectId"`
}

type JwtRefreshPayload struct {
	UserId string `json:"userId"`
}

//func AddAuthCookies(ctx *gin.Context, jwtToken string, refreshToken string) {
//	jwtCookie := http.Cookie{
//		Name:     "token",
//		Value:    jwtToken,
//		Path:     "/",
//		Domain:   "",
//		MaxAge:   60,
//		SameSite: http.SameSiteStrictMode,
//		Secure:   true,
//		HttpOnly: true,
//	}
//	http.SetCookie(ctx.Writer, &jwtCookie)
//
//	refreshCookie := http.Cookie{
//		Name:     "refreshToken",
//		Value:    refreshToken,
//		Path:     "/",
//		Domain:   "",
//		MaxAge:   60 * 60 * 3,
//		SameSite: http.SameSiteStrictMode,
//		Secure:   true,
//		HttpOnly: true,
//	}
//	http.SetCookie(ctx.Writer, &refreshCookie)
//}

func VerifyRefreshToken(token string) (payload *JwtRefreshPayload, err error) {
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))

	return verifyRefreshToken(token, jwtSecret)
}

func VerifyJwtToken(token string) (payload *JwtPayload, err error) {
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))

	return verifyJwtToken(token, jwtSecret)
}

func verifyJwtToken(token string, jwtSecret []byte) (payload *JwtPayload, err error) {
	jwtToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		fmt.Println("Token parsing error:", err)
		return nil, err
	}

	if _, ok := jwtToken.Claims.(jwt.MapClaims); ok && jwtToken.Valid {
		payload = &JwtPayload{
			Username:       jwtToken.Claims.(jwt.MapClaims)["username"].(string),
			Admin:          jwtToken.Claims.(jwt.MapClaims)["admin"].(bool),
			UserId:         jwtToken.Claims.(jwt.MapClaims)["userId"].(string),
			AvatarObjectId: jwtToken.Claims.(jwt.MapClaims)["avatarObjectId"].(string),
		}
		return payload, nil
	}

	return nil, fmt.Errorf("invalid token")
}

func verifyRefreshToken(token string, jwtSecret []byte) (payload *JwtRefreshPayload, err error) {
	jwtToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		fmt.Println("Token parsing error:", err)
		return nil, err
	}

	if _, ok := jwtToken.Claims.(jwt.MapClaims); ok && jwtToken.Valid {
		payload = &JwtRefreshPayload{
			UserId: jwtToken.Claims.(jwt.MapClaims)["userId"].(string),
		}
		return payload, nil
	}

	return nil, fmt.Errorf("invalid token")
}

func GenerateTokens(payload *JwtPayload) (token string, refreshToken string, err error) {
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))

	token, err = generateJwtToken(payload, jwtSecret)
	if err != nil {
		return "", "", err
	}

	refreshToken, err = generateRefreshToken(JwtRefreshPayload{UserId: payload.UserId}, jwtSecret)
	if err != nil {
		return "", "", err
	}

	return token, refreshToken, nil
}

func generateRefreshToken(payload JwtRefreshPayload, jwtSecret []byte) (string, error) {
	claims := jwt.MapClaims{}
	claims["userId"] = payload.UserId

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(jwtSecret)
	return tokenString, err
}

func generateJwtToken(payload *JwtPayload, jwtSecret []byte) (string, error) {
	claims := jwt.MapClaims{}
	claims["username"] = payload.Username
	claims["admin"] = payload.Admin
	claims["userId"] = payload.UserId
	claims["avatarObjectId"] = payload.AvatarObjectId

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(jwtSecret)
	return tokenString, err
}
