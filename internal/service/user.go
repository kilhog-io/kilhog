package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

var (
	ErrUserNotFound              = errors.New("user not found")
	ErrUsernameTaken             = errors.New("username already exists")
	ErrInvalidUsername           = errors.New("invalid username")
	ErrInvalidPassword           = errors.New("invalid password")
	ErrInvalidUserRole           = errors.New("invalid user role")
	ErrInvalidCredentials        = errors.New("invalid credentials")
	ErrInvalidEmail              = errors.New("invalid email")
	ErrBootstrapUnavailable      = errors.New("bootstrap unavailable")
	ErrBootstrapForbidden        = errors.New("bootstrap forbidden")
	ErrLastAdmin                 = errors.New("cannot remove last enabled admin")
	ErrForbidden                 = errors.New("forbidden")
	ErrIdentityPoolNotFound      = errors.New("identity pool not found")
	ErrIdentityPoolNameTaken     = errors.New("identity pool name already exists")
	ErrIdentityPoolSlugTaken     = errors.New("identity pool slug already exists")
	ErrIdentityPoolIssuerTaken   = errors.New("identity pool issuer already exists")
	ErrInvalidIdentityPool       = errors.New("invalid identity pool")
	ErrSessionNotFound           = errors.New("session not found")
	ErrSessionExpired            = errors.New("session expired")
	ErrOIDCLoginStateNotFound    = errors.New("oidc login state not found")
	ErrOIDCLoginStateExpired     = errors.New("oidc login state expired")
	ErrOIDCNotConfigured         = errors.New("oidc not configured")
	ErrAuthNotConfigured         = errors.New("authentication is not configured")
	ErrUnauthenticated           = errors.New("unauthenticated")
)

const (
	minPasswordLength = 8
	maxPasswordLength = 200
	minUsernameLength = 2
	maxUsernameLength = 64
)

type UserService struct {
	users UserRepository
}

func NewUserService(users UserRepository) *UserService {
	return &UserService{users: users}
}

type CreateUserInput struct {
	Username    string
	Password    string
	DisplayName string
	Email       string
	Role        model.UserRole
	Enabled     *bool
}

type UpdateUserInput struct {
	Username    *string
	DisplayName *string
	Email       *string
	Role        *model.UserRole
	Enabled     *bool
	Password    *string
}

type ChangePasswordInput struct {
	CurrentPassword string
	NewPassword     string
}

func (s *UserService) List(ctx context.Context) ([]*model.LocalUser, error) {
	users, err := s.users.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	if users == nil {
		users = []*model.LocalUser{}
	}
	return users, nil
}

func (s *UserService) GetByUUID(ctx context.Context, id uuid.UUID) (*model.LocalUser, error) {
	user, err := s.users.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

func (s *UserService) Create(ctx context.Context, input CreateUserInput) (*model.LocalUser, error) {
	username, err := normalizeUsername(input.Username)
	if err != nil {
		return nil, err
	}
	if err := validatePassword(input.Password); err != nil {
		return nil, err
	}
	role := input.Role
	if role == "" {
		role = model.UserRoleUser
	}
	if err := validateRole(role); err != nil {
		return nil, err
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return nil, err
	}

	if existing, err := s.users.GetByUsername(ctx, username); err == nil && existing != nil {
		return nil, userError(ErrUsernameTaken, `username %q is already used`, username)
	} else if err != nil && !errors.Is(err, ErrUserNotFound) {
		return nil, fmt.Errorf("check username: %w", err)
	}

	hash, err := hashPassword(input.Password)
	if err != nil {
		return nil, err
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	user := &model.LocalUser{
		UUID:         uuid.New(),
		Username:     username,
		PasswordHash: hash,
		DisplayName:  strings.TrimSpace(input.DisplayName),
		Email:        email,
		Role:         role,
		Enabled:      enabled,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (s *UserService) Update(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*model.LocalUser, error) {
	user, err := s.users.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	wasEnabledAdmin := user.Enabled && user.Role == model.UserRoleAdmin

	if input.Username != nil {
		username, err := normalizeUsername(*input.Username)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(username, user.Username) {
			if existing, err := s.users.GetByUsername(ctx, username); err == nil && existing != nil && existing.UUID != user.UUID {
				return nil, userError(ErrUsernameTaken, `username %q is already used`, username)
			} else if err != nil && !errors.Is(err, ErrUserNotFound) {
				return nil, fmt.Errorf("check username: %w", err)
			}
		}
		user.Username = username
	}
	if input.DisplayName != nil {
		user.DisplayName = strings.TrimSpace(*input.DisplayName)
	}
	if input.Email != nil {
		email, err := normalizeEmail(*input.Email)
		if err != nil {
			return nil, err
		}
		user.Email = email
	}
	if input.Role != nil {
		if err := validateRole(*input.Role); err != nil {
			return nil, err
		}
		user.Role = *input.Role
	}
	if input.Enabled != nil {
		user.Enabled = *input.Enabled
	}
	if input.Password != nil {
		if err := validatePassword(*input.Password); err != nil {
			return nil, err
		}
		hash, err := hashPassword(*input.Password)
		if err != nil {
			return nil, err
		}
		user.PasswordHash = hash
	}

	stillEnabledAdmin := user.Enabled && user.Role == model.UserRoleAdmin
	if wasEnabledAdmin && !stillEnabledAdmin {
		if err := s.ensureNotLastAdmin(ctx); err != nil {
			return nil, err
		}
	}

	if err := s.users.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return user, nil
}

func (s *UserService) Delete(ctx context.Context, id uuid.UUID) error {
	user, err := s.users.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrUserNotFound
		}
		return fmt.Errorf("get user: %w", err)
	}
	if user.Enabled && user.Role == model.UserRoleAdmin {
		if err := s.ensureNotLastAdmin(ctx); err != nil {
			return err
		}
	}
	if err := s.users.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (s *UserService) ChangeOwnPassword(ctx context.Context, id uuid.UUID, input ChangePasswordInput) error {
	user, err := s.users.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrUserNotFound
		}
		return fmt.Errorf("get user: %w", err)
	}
	if !checkPassword(user.PasswordHash, input.CurrentPassword) {
		return userError(ErrInvalidCredentials, "current password is incorrect")
	}
	if err := validatePassword(input.NewPassword); err != nil {
		return err
	}
	hash, err := hashPassword(input.NewPassword)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	if err := s.users.Update(ctx, user); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

func (s *UserService) ensureNotLastAdmin(ctx context.Context) error {
	n, err := s.users.CountEnabledAdmins(ctx)
	if err != nil {
		return fmt.Errorf("count enabled admins: %w", err)
	}
	if n <= 1 {
		return userError(ErrLastAdmin, "cannot delete or disable the last enabled admin")
	}
	return nil
}

func normalizeUsername(raw string) (string, error) {
	username := strings.TrimSpace(raw)
	if len(username) < minUsernameLength || len(username) > maxUsernameLength {
		return "", userError(ErrInvalidUsername, "username must be between %d and %d characters", minUsernameLength, maxUsernameLength)
	}
	for _, r := range username {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			continue
		}
		return "", userError(ErrInvalidUsername, "username may only contain letters, digits, '.', '_' and '-'")
	}
	return strings.ToLower(username), nil
}

func validatePassword(password string) error {
	if len(password) < minPasswordLength || len(password) > maxPasswordLength {
		return userError(ErrInvalidPassword, "password must be between %d and %d characters", minPasswordLength, maxPasswordLength)
	}
	return nil
}

func validateRole(role model.UserRole) error {
	switch role {
	case model.UserRoleAdmin, model.UserRoleUser:
		return nil
	default:
		return userError(ErrInvalidUserRole, "role must be %q or %q", model.UserRoleAdmin, model.UserRoleUser)
	}
}

func normalizeEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", nil
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return "", userError(ErrInvalidEmail, "invalid email address")
	}
	return addr.Address, nil
}
