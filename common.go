package main

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
)

// UserResponse is to get status message and data
type UserResponse struct {
	Status  int        `json:"status"`
	Message string     `json:"message"`
	Data    interface{} `json:"data"`
}

// ValidateToken is to valliadate the user
func ValidateToken() func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		db, ok := c.Context().UserValue("UDB").(*gorm.DB)
		defer db.Close()
		_ = db
		if !ok {
			return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: &fiber.Map{"data": "db not found in context"}})

		}
		reqToken := c.Get("Authorization")
		if reqToken == "" {
			reqToken = c.Get("authorization")
		}
		replacer := strings.NewReplacer("bearer", "Bearer", "BEARER", "Bearer")
		reqToken = replacer.Replace(reqToken)
		splitToken := strings.Split(reqToken, "Bearer ")
		if len(splitToken) == 2 {
			reqToken = splitToken[1]
		} else {
			c.Context().SetUserValue("AuthorizationRequired", 1)
		}

		type Details struct {
			Userid string
		}

		var response Details
		if reqToken != "" {
			db.Debug().Raw("select data->>'UserID' as userid  from oauth2_tokens ot where access = ? and (data->>'ExpiresAt')::timestamp >= now()", reqToken).Scan(&response)
		}

		if response.Userid != "" {
			c.Context().SetUserValue("userid", response.Userid)
			c.Context().SetUserValue("AuthorizationRequired", 0)
			c.Next()
		} else {
			c.Context().SetUserValue("userid", "")
			c.Context().SetUserValue("AuthorizationRequired", 1)
			c.Next()

		}

		return nil
	}
}
