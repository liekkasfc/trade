package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"quantsaas/internal/saas/config"
	"quantsaas/internal/saas/store"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already exists")
)

type Claims struct {
	UserID uint   `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type Service struct {
	db     *gorm.DB
	issuer string
	secret []byte
	ttl    time.Duration
}

func NewService(db *gorm.DB, cfg config.JWTConfig) (*Service, error) {
	if strings.TrimSpace(cfg.Secret) == "" {
		return nil, errors.New("jwt secret is required")
	}
	if strings.TrimSpace(cfg.Issuer) == "" {
		return nil, errors.New("jwt issuer is required")
	}

	return &Service{
		db:     db,
		issuer: cfg.Issuer,
		secret: []byte(cfg.Secret),
		ttl:    time.Duration(cfg.TTLHours) * time.Hour,
	}, nil
}

func (s *Service) Register(ctx context.Context, email, password string) (*store.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, errors.New("email is required")
	}
	if len(password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}

	var existing store.User
	err := s.db.WithContext(ctx).Where("email = ?", email).Take(&existing).Error
	if err == nil {
		return nil, ErrEmailTaken
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup existing user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &store.User{
		Email:        email,
		PasswordHash: string(hash),
		Role:         store.UserRoleUser,
		Plan:         "core",
	}
	if err := s.db.WithContext(ctx).Create(user).Error; err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (s *Service) Authenticate(ctx context.Context, email, password string) (*store.User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var user store.User
	if err := s.db.WithContext(ctx).Where("email = ?", email).Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", fmt.Errorf("find user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	token, err := s.SignToken(user.ID, user.Role)
	if err != nil {
		return nil, "", err
	}

	return &user, token, nil
}

func (s *Service) SignToken(userID uint, role string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   fmt.Sprintf("%d", userID),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *Service) ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidCredentials
	}

	return claims, nil
}

func (s *Service) LoadUser(ctx context.Context, userID uint) (*store.User, error) {
	var user store.User
	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
