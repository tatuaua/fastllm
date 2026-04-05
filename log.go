package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

var logs []string

func logf(format string, args ...any) {
	logs = append(logs, fmt.Sprintf(format, args...))
}

func WriteLogs(path string) {
	os.WriteFile(path, []byte(strings.Join(logs, "\n")+"\n"), 0644)
}

func loadEnv() {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("error loading .env file: %v", err)
	}
}
