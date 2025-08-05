package main

import (
	"github.com/labstack/echo/v4"
	"github.com/sse-evn/eomstart/backend/handlers"
)

func main() {
	e := echo.New()

	// Маршруты
	e.GET("/api/example", handlers.GetExample)

	e.Logger.Fatal(e.Start(":8080"))
}