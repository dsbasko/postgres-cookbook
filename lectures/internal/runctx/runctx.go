// Package runctx — context, отменяемый по SIGINT и SIGTERM.
//
// Используется как корневой ctx у каждого cmd/main.go в лекциях. Когда автор
// нажимает Ctrl+C, пул соединений и запущенные запросы получают cancellation и
// корректно завершаются — без зависших транзакций и подвисших соединений.
package runctx

import (
	"context"
	"os/signal"
	"syscall"
)

// New возвращает ctx, отменяемый по SIGINT/SIGTERM, и cancel-функцию.
// Caller обязан вызвать cancel() в defer, чтобы освободить signal handler
// (иначе он живёт до конца процесса — для лекций это ок, но привычка
// полезная).
func New() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}
