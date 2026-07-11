package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/database"
)

func main() {
	email := flag.String("email", "", "admin email")
	password := flag.String("password", "", "admin password (min 10 chars)")
	display := flag.String("name", "Platform Admin", "display name")
	role := flag.String("role", auth.RoleAdmin, "role")
	force := flag.Bool("force", false, "cập nhật mật khẩu nếu user đã tồn tại")
	dbURL := flag.String("db", os.Getenv("DATABASE_URL"), "postgres url")
	flag.Parse()

	if *email == "" || *password == "" {
		log.Fatal("usage: seed-admin --email x@y.com --password '...' [--role admin|tech_lead|dev]")
	}
	if !auth.ValidRole(*role) {
		log.Fatal("role không hợp lệ")
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, *dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		log.Fatal("migrate:", err)
	}

	store := auth.NewStore(pool)
	emailNorm := strings.ToLower(*email)
	existing, err := store.GetUserByEmail(ctx, emailNorm)
	if err == nil {
		if !*force {
			fmt.Println("user đã tồn tại — bỏ qua (dùng --force để đổi mật khẩu)")
			return
		}
		hash, err := auth.HashPassword(*password)
		if err != nil {
			log.Fatal(err)
		}
		if err := store.UpdatePassword(ctx, existing.ID, hash); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("đã cập nhật mật khẩu user id=%d email=%s\n", existing.ID, *email)
		return
	}

	hash, err := auth.HashPassword(*password)
	if err != nil {
		log.Fatal(err)
	}
	id, err := store.CreateUser(ctx, strings.ToLower(*email), *display, hash, *role)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("đã tạo user id=%d email=%s role=%s\n", id, *email, *role)
}
