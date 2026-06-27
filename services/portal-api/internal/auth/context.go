package auth

import (
	"context"
)

type contextKey int

const userContextKey contextKey = 1

type User struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Active      bool   `json:"active,omitempty"`
}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userContextKey).(User)
	return u, ok
}

func MustUser(ctx context.Context) User {
	u, ok := UserFromContext(ctx)
	if !ok {
		panic("auth: no user in context")
	}
	return u
}
