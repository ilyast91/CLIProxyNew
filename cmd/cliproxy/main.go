// Package main — точка входа для бинарника cliproxy.
package main

import (
	"fmt"
	"os"
)

const (
	appName    = "cliproxy"
	appVersion = "dev"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", appName, err)
		os.Exit(1)
	}
}

// run — основная точка входа приложения.
// Здесь позже появится загрузка конфигурации, инициализация upstream relay SDK
// (через go-модуль) и запуск HTTP/gRPC сервера.
func run() error {
	fmt.Printf("%s %s — инициализация ещё не завершена\n", appName, appVersion)
	return nil
}
