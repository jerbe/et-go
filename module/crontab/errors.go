package crontab

import "errors"

var (
	// ErrInvalidCronExpression 表示 cron 表达式格式错误。
	ErrInvalidCronExpression = errors.New("crontab: invalid cron expression")
	// ErrTaskAlreadyExists 表示尝试注册重复任务名。
	ErrTaskAlreadyExists = errors.New("crontab: task already exists")
	// ErrTaskNotFound 表示请求的任务不存在。
	ErrTaskNotFound = errors.New("crontab: task not found")
	// ErrHandlerNotRegistered 表示未注册对应的处理器。
	ErrHandlerNotRegistered = errors.New("crontab: handler not registered")
	// ErrComponentClosed 表示 CrontabComponent 已销毁。
	ErrComponentClosed = errors.New("crontab: component closed")
)
