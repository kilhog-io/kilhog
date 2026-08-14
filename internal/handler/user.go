package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/service"
)

func registerUserAdminRoutes(mux *http.ServeMux, users *service.UserService) {
	if users == nil {
		return
	}
	mux.HandleFunc("GET /users", requireAdmin(listUsersHandler(users)))
	mux.HandleFunc("POST /users", requireAdmin(createUserHandler(users)))
	mux.HandleFunc("GET /users/{uuid}", requireAdmin(getUserHandler(users)))
	mux.HandleFunc("PUT /users/{uuid}", requireAdmin(updateUserHandler(users)))
	mux.HandleFunc("DELETE /users/{uuid}", requireAdmin(deleteUserHandler(users)))
}

type createUserRequest struct {
	Username    string          `json:"username"`
	Password    string          `json:"password"`
	DisplayName string          `json:"display_name"`
	Email       string          `json:"email"`
	Role        model.UserRole  `json:"role"`
	Enabled     *bool           `json:"enabled"`
}

type updateUserRequest struct {
	Username    *string         `json:"username"`
	Password    *string         `json:"password"`
	DisplayName *string         `json:"display_name"`
	Email       *string         `json:"email"`
	Role        *model.UserRole `json:"role"`
	Enabled     *bool           `json:"enabled"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func listUsersHandler(users *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := users.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list users")
			return
		}
		writeSuccess(w, http.StatusOK, list)
	}
}

func createUserHandler(users *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		user, err := users.Create(r.Context(), service.CreateUserInput{
			Username:    req.Username,
			Password:    req.Password,
			DisplayName: req.DisplayName,
			Email:       req.Email,
			Role:        req.Role,
			Enabled:     req.Enabled,
		})
		if err != nil {
			writeUserError(w, err)
			return
		}
		writeSuccess(w, http.StatusCreated, user)
	}
}

func getUserHandler(users *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user uuid")
			return
		}
		user, err := users.GetByUUID(r.Context(), id)
		if err != nil {
			writeUserError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, user)
	}
}

func updateUserHandler(users *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user uuid")
			return
		}
		var req updateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		user, err := users.Update(r.Context(), id, service.UpdateUserInput{
			Username:    req.Username,
			Password:    req.Password,
			DisplayName: req.DisplayName,
			Email:       req.Email,
			Role:        req.Role,
			Enabled:     req.Enabled,
		})
		if err != nil {
			writeUserError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, user)
	}
}

func deleteUserHandler(users *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user uuid")
			return
		}
		if err := users.Delete(r.Context(), id); err != nil {
			writeUserError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, nil)
	}
}

func changeOwnPasswordHandler(users *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal := principalFromContext(r.Context())
		if principal == nil || principal.LocalUser == nil {
			writeError(w, http.StatusForbidden, "only local users can change their password")
			return
		}

		var req changePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := users.ChangeOwnPassword(r.Context(), principal.LocalUser.UUID, service.ChangePasswordInput{
			CurrentPassword: req.CurrentPassword,
			NewPassword:     req.NewPassword,
		}); err != nil {
			writeUserError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, nil)
	}
}

func writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		writeError(w, http.StatusNotFound, errorMessage(err, "user not found"))
	case errors.Is(err, service.ErrUsernameTaken):
		writeError(w, http.StatusConflict, errorMessage(err, "username already exists"))
	case errors.Is(err, service.ErrLastAdmin):
		writeError(w, http.StatusConflict, errorMessage(err, "cannot remove last enabled admin"))
	case errors.Is(err, service.ErrInvalidUsername),
		errors.Is(err, service.ErrInvalidPassword),
		errors.Is(err, service.ErrInvalidUserRole),
		errors.Is(err, service.ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, errorMessage(err, "invalid request"))
	case errors.Is(err, service.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, errorMessage(err, "invalid credentials"))
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
