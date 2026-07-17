package app

import (
	"net/http"

	v1 "watchtower/internal/app/v1"
)

type App struct {
	V1 *v1.V1App
}

func New(v1App *v1.V1App) *App {
	return &App{
		V1: v1App,
	}
}

func (a *App) RegisterRoutes(mux *http.ServeMux) {
	a.V1.RegisterAll(mux)
}
