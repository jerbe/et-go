package actor

import "errors"

var (
	// ErrHandlerNotFound 消息处理器未找到
	ErrHandlerNotFound = errors.New("actor: message handler not found")
	// ErrActorNotFound Actor 未找到
	ErrActorNotFound = errors.New("actor: actor not found")
	// ErrTimeout 消息超时
	ErrTimeout = errors.New("actor: message timeout")
	// ErrMailboxFull Actor 邮箱已满
	ErrMailboxFull = errors.New("actor: mailbox is full")
	// ErrFiberManagerMissing 表示跨进程接收端没有注入本地 Fiber Manager。
	ErrFiberManagerMissing = errors.New("actor: fiber manager missing")
	// ErrRpcManagerMissing 表示跨进程发送器没有注入 RPC Manager。
	ErrRpcManagerMissing = errors.New("actor: rpc manager missing")
	// ErrInvalidPacket 表示收到无法处理的网络包。
	ErrInvalidPacket = errors.New("actor: invalid packet")
	// ErrProcessSessionClosed 表示远端 Process Session 已关闭。
	ErrProcessSessionClosed = errors.New("actor: process session closed")
	// ErrProcessOuterSenderDuplicate 表示同一 Process 重复注册外发器。
	ErrProcessOuterSenderDuplicate = errors.New("actor: process outer sender already registered")
	// ErrProcessOuterSenderRequired 表示注册外发器缺少必要参数。
	ErrProcessOuterSenderRequired = errors.New("actor: process outer sender required")
)
