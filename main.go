package main

import (
	"embed"
	"mitm-departament/internal/app"
)

//go:embed frontend/*
var templateFS embed.FS

func main() {
	app.Start(templateFS)
}
