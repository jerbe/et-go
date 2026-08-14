package actor

import "sync"

var processOuterRegistry = struct {
	sync.RWMutex
	senders map[int]*ProcessOuterSender
}{
	senders: make(map[int]*ProcessOuterSender),
}

// RegisterProcessOuterSender 注册当前 Process 的跨进程发送器。
//
// 一个逻辑 Process 只能有一个外发器；Session 的增删由该外发器统一管理，
// 防止业务 Fiber 各自持有不一致的 Session 表。
func RegisterProcessOuterSender(processID int, sender *ProcessOuterSender) error {
	if processID <= 0 || sender == nil {
		return ErrProcessOuterSenderRequired
	}
	processOuterRegistry.Lock()
	defer processOuterRegistry.Unlock()
	if existing := processOuterRegistry.senders[processID]; existing != nil && existing != sender {
		return ErrProcessOuterSenderDuplicate
	}
	processOuterRegistry.senders[processID] = sender
	sender.mu.Lock()
	sender.processID = processID
	sender.mu.Unlock()
	return nil
}

// UnregisterProcessOuterSender 删除指定实例的进程级外发器。
func UnregisterProcessOuterSender(processID int, sender *ProcessOuterSender) {
	if processID <= 0 || sender == nil {
		return
	}
	processOuterRegistry.Lock()
	if processOuterRegistry.senders[processID] == sender {
		delete(processOuterRegistry.senders, processID)
	}
	processOuterRegistry.Unlock()
}

// ResolveProcessOuterSender 返回指定 Process 的跨进程发送器。
func ResolveProcessOuterSender(processID int) *ProcessOuterSender {
	if processID <= 0 {
		return nil
	}
	processOuterRegistry.RLock()
	sender := processOuterRegistry.senders[processID]
	processOuterRegistry.RUnlock()
	if sender == nil {
		return nil
	}
	sender.mu.RLock()
	closed := sender.closed
	sender.mu.RUnlock()
	if closed {
		return nil
	}
	return sender
}
