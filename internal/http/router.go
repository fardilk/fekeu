package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"be03/internal/http/handlers"
)

// NewRouter builds and returns the chi router with all routes configured
func NewRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)

	// Init handlers
	authHandler := handlers.NewAuthHandler(db)
	userHandler := handlers.NewUserHandler(db)
	jwtAuth := handlers.NewJWTAuthMiddleware(db)

	// Public routes
	r.Get("/health", healthHandler)

	// User authentication routes (public)
	r.Post("/register", userHandler.Register)
	r.Post("/login", userHandler.Login)
	r.Post("/refresh", userHandler.Refresh)
	r.Post("/revoke", userHandler.Revoke)

	// Auth routes (password reset flow)
	r.Route("/auth", func(r chi.Router) {
		r.Post("/forgot-password", authHandler.ForgotPassword)
		r.Post("/forgot-password/verify", authHandler.VerifyOTP)
		r.Post("/reset-password", authHandler.ResetPassword)
		r.With(jwtAuth).Put("/change-password", authHandler.ChangePassword)
	})

	// Protected routes (require JWT)
	r.Group(func(r chi.Router) {
		r.Use(jwtAuth)
		r.Get("/me", userHandler.Me)
		// TODO: Add profile, catatan, upload routes
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write([]byte(`{"status":"ok"}`))
}
